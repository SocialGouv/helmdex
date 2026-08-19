// Package authsvc orchestrates sign-in against private chart sources: it
// resolves a secret (typed in, or re-resolved from a detected local source),
// verifies it against the remote, and persists it in the credential store.
// It is shared by the HTTP API (web/desktop UI) and the CLI.
package authsvc

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"helmdex/internal/creds"
	"helmdex/internal/gitutil"
	"helmdex/internal/helmutil"
)

// Method selects how the secret is obtained.
const (
	MethodManual   = "manual"
	MethodDetected = "detected"
)

// LoginRequest describes one sign-in attempt.
type LoginRequest struct {
	Host string     `json:"host"`
	Kind creds.Kind `json:"kind"`
	// Method is "manual" (username/secret/sshKeyPath provided) or
	// "detected" (source/sourceHost from a previous detect call; the secret
	// is re-resolved locally and never transits through the client).
	Method     string `json:"method"`
	Source     string `json:"source,omitempty"`
	SourceHost string `json:"sourceHost,omitempty"`
	Username   string `json:"username,omitempty"`
	Secret     string `json:"secret,omitempty"`
	SSHKeyPath string `json:"sshKeyPath,omitempty"`
	// URL is the failing repository/remote URL when known; it enables
	// verification against the actual remote.
	URL string `json:"url,omitempty"`
}

// LoginResult reports a stored credential (never the secret).
type LoginResult struct {
	Host     string     `json:"host"`
	Kind     creds.Kind `json:"kind"`
	Username string     `json:"username,omitempty"`
	Source   string     `json:"source"`
	// Verified is false when no URL was available to test against (the
	// credential is stored anyway).
	Verified bool   `json:"verified"`
	Message  string `json:"message,omitempty"`
}

// Login resolves, verifies and stores a credential. repoRoot scopes the Helm
// env used for OCI verification (the shared registry config of that repo).
func Login(ctx context.Context, repoRoot string, req LoginRequest) (LoginResult, error) {
	host := strings.ToLower(strings.TrimSpace(req.Host))
	if host == "" {
		return LoginResult{}, fmt.Errorf("host is required")
	}
	if !creds.ValidKind(req.Kind) {
		return LoginResult{}, fmt.Errorf("kind must be one of oci, git, helm-repo (got %q)", req.Kind)
	}

	cred := creds.Credential{Host: host, Kind: req.Kind, Source: req.Source}
	switch req.Method {
	case MethodDetected:
		if req.Source == "" {
			return LoginResult{}, fmt.Errorf("source is required for method=detected")
		}
		srcHost := strings.ToLower(strings.TrimSpace(req.SourceHost))
		if srcHost == "" {
			srcHost = host
		}
		username, secret, err := creds.ResolveCandidate(ctx, req.Source, srcHost)
		if err != nil {
			return LoginResult{}, fmt.Errorf("resolve %s credential for %s: %w", req.Source, srcHost, err)
		}
		cred.Username, cred.Secret = username, secret
	case MethodManual, "":
		cred.Source = creds.SourceManual
		cred.Username = strings.TrimSpace(req.Username)
		cred.Secret = req.Secret
		cred.SSHKeyPath = strings.TrimSpace(req.SSHKeyPath)
		if cred.Secret == "" && cred.SSHKeyPath == "" {
			return LoginResult{}, fmt.Errorf("a token/password or an SSH key path is required")
		}
		if cred.SSHKeyPath != "" {
			if req.Kind != creds.KindGit {
				return LoginResult{}, fmt.Errorf("an SSH key only applies to git remotes")
			}
			st, err := os.Stat(cred.SSHKeyPath)
			if err != nil {
				return LoginResult{}, fmt.Errorf("ssh key %s: %w", cred.SSHKeyPath, err)
			}
			if st.IsDir() {
				return LoginResult{}, fmt.Errorf("ssh key %s is a directory", cred.SSHKeyPath)
			}
		}
	default:
		return LoginResult{}, fmt.Errorf("unknown method %q", req.Method)
	}
	if cred.Username == "" && cred.Secret != "" {
		// Forges accept a PAT with any non-empty username.
		cred.Username = "oauth2"
	}

	verified, msg, err := verify(ctx, repoRoot, cred, strings.TrimSpace(req.URL))
	if err != nil {
		return LoginResult{}, err
	}

	if err := creds.Upsert(cred); err != nil {
		return LoginResult{}, err
	}
	return LoginResult{
		Host:     cred.Host,
		Kind:     cred.Kind,
		Username: cred.Username,
		Source:   cred.Source,
		Verified: verified,
		Message:  msg,
	}, nil
}

// verify tests the credential against the remote before it is stored. A
// missing URL (or SSH without URL) yields verified=false rather than an
// error: storing an unverified credential is still useful.
func verify(ctx context.Context, repoRoot string, cred creds.Credential, url string) (bool, string, error) {
	switch cred.Kind {
	case creds.KindOCI:
		if repoRoot == "" {
			return false, "stored without verification (no repository open)", nil
		}
		env := helmutil.EnvForRepo(repoRoot)
		if err := helmutil.RegistryLoginStdin(ctx, env, cred.Host, cred.Username, cred.Secret); err != nil {
			return false, "", fmt.Errorf("registry login to %s failed: %w", cred.Host, err)
		}
		return true, "", nil

	case creds.KindHelmRepo:
		if url == "" {
			return false, "stored without verification (no repository URL)", nil
		}
		if err := verifyHelmRepoIndex(ctx, url, cred.Username, cred.Secret); err != nil {
			return false, "", err
		}
		return true, "", nil

	case creds.KindGit:
		if url == "" {
			return false, "stored without verification (no remote URL)", nil
		}
		if err := gitutil.VerifyAccessWithCredential(ctx, url, cred); err != nil {
			return false, "", fmt.Errorf("git access to %s failed: %w", url, err)
		}
		return true, "", nil
	}
	return false, "", fmt.Errorf("invalid kind %q", cred.Kind)
}

func verifyHelmRepoIndex(ctx context.Context, repoURL, username, secret string) error {
	indexURL := strings.TrimRight(repoURL, "/") + "/index.yaml"
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, indexURL, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(username, secret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", indexURL, err)
	}
	defer resp.Body.Close() //nolint:errcheck // response body close
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("credentials rejected by %s (%s)", indexURL, resp.Status)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("fetch %s: unexpected status %s", indexURL, resp.Status)
	}
	return nil
}

// Logout removes the stored credential and its materialized registry entry
// for the currently open repo.
func Logout(repoRoot, host string, kind creds.Kind) (bool, error) {
	removed, err := creds.Remove(host, kind)
	if err != nil {
		return false, err
	}
	if removed && (kind == "" || kind == creds.KindOCI) && repoRoot != "" {
		env := helmutil.EnvForRepo(repoRoot)
		if err := creds.RemoveRegistryAuth(env.RegistryConfig, host); err != nil {
			return removed, fmt.Errorf("credential removed, but cleaning the registry config failed: %w", err)
		}
	}
	return removed, nil
}

// OpenBrowser opens url with the platform opener. Errors are surfaced (the
// caller can still display the URL for manual opening).
func OpenBrowser(url string) error {
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		return fmt.Errorf("refusing to open non-http URL")
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}
	// Detach: the opener's lifetime is not ours to manage.
	go func() { _ = cmd.Wait() }()
	return nil
}
