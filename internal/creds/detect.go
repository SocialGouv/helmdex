package creds

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Candidate is a credential found in the user's local configuration that
// could authenticate against a host. It carries no secret: the secret is
// re-resolved server-side at login time via ResolveCandidate.
type Candidate struct {
	// Source identifies the local origin: docker-config, podman,
	// helm-registry, git-credential, gh-cli, glab-cli.
	Source string `json:"source"`
	// Host is the host the local credential is registered for (may be a
	// related host, e.g. the GitLab host for a registry host).
	Host string `json:"host"`
	// Username as reported by the source ("" when unknown).
	Username string `json:"username,omitempty"`
	// Label is a human-readable description for the UI.
	Label string `json:"label"`
}

const (
	SourceManual        = "manual"
	SourceDockerConfig  = "docker-config"
	SourcePodman        = "podman"
	SourceHelmRegistry  = "helm-registry"
	SourceGitCredential = "git-credential"
	SourceGhCLI         = "gh-cli"
	SourceGlabCLI       = "glab-cli"
)

const probeTimeout = 5 * time.Second

// Detect probes the user's local configuration for credentials usable
// against host (and closely related hosts). It returns candidates without
// secrets, ordered by relevance (exact host first, then related hosts).
func Detect(ctx context.Context, host string, kind Kind) []Candidate {
	host = strings.ToLower(strings.TrimSpace(host))
	hosts := append([]string{host}, RelatedHosts(host)...)

	var out []Candidate
	seen := map[string]struct{}{}
	add := func(c Candidate, err error) {
		if err != nil || c.Source == "" {
			return
		}
		key := c.Source + "|" + c.Host
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, c)
	}

	for _, h := range hosts {
		// Container-registry style sources are most relevant for OCI but a
		// docker login against a GitLab registry also proves a usable
		// account, so probe them for every kind.
		add(probeDockerLike(SourceDockerConfig, dockerConfigPath(), h))
		add(probePodman(h))
		add(probeDockerLike(SourceHelmRegistry, helmRegistryConfigPath(), h))
		add(probeGitCredential(ctx, h))
		add(probeGhCLI(ctx, h))
		add(probeGlabCLI(h))
	}
	_ = kind
	return out
}

// ResolveCandidate re-resolves the secret for a previously detected
// candidate. The secret never transits through the browser: the UI sends
// back (source, host) and the server resolves it locally.
func ResolveCandidate(ctx context.Context, source, host string) (username, secret string, err error) {
	host = strings.ToLower(strings.TrimSpace(host))
	switch source {
	case SourceDockerConfig:
		return resolveDockerLike(dockerConfigPath(), host)
	case SourcePodman:
		p, perr := podmanAuthPath()
		if perr != nil {
			return "", "", perr
		}
		return resolveDockerLike(p, host)
	case SourceHelmRegistry:
		return resolveDockerLike(helmRegistryConfigPath(), host)
	case SourceGitCredential:
		return gitCredentialFill(ctx, host)
	case SourceGhCLI:
		return resolveGhCLI(ctx, host)
	case SourceGlabCLI:
		return resolveGlabCLI(host)
	default:
		return "", "", fmt.Errorf("unknown credential source %q", source)
	}
}

// --- docker-style config.json (docker, podman, helm registry) ---

type dockerAuthEntry struct {
	Auth          string `json:"auth,omitempty"`
	Username      string `json:"username,omitempty"`
	Password      string `json:"password,omitempty"`
	IdentityToken string `json:"identitytoken,omitempty"`
}

type dockerConfigFile struct {
	Auths       map[string]dockerAuthEntry `json:"auths"`
	CredsStore  string                     `json:"credsStore,omitempty"`
	CredHelpers map[string]string          `json:"credHelpers,omitempty"`
}

func dockerConfigPath() string {
	if v := strings.TrimSpace(os.Getenv("DOCKER_CONFIG")); v != "" {
		return filepath.Join(v, "config.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".docker", "config.json")
}

func helmRegistryConfigPath() string {
	base, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "helm", "registry", "config.json")
}

func podmanAuthPath() (string, error) {
	if v := strings.TrimSpace(os.Getenv("REGISTRY_AUTH_FILE")); v != "" {
		return v, nil
	}
	if rt := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR")); rt != "" {
		p := filepath.Join(rt, "containers", "auth.json")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "containers", "auth.json"), nil
}

func readDockerConfig(path string) (dockerConfigFile, error) {
	var cfg dockerConfigFile
	if path == "" {
		return cfg, os.ErrNotExist
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}

// dockerAuthKeys lists the auths-map keys under which a host may be stored.
func dockerAuthKeys(host string) []string {
	keys := []string{host, "https://" + host, "http://" + host, host + "/v2/", "https://" + host + "/v2/"}
	if host == "docker.io" || host == "registry-1.docker.io" || host == "index.docker.io" {
		keys = append(keys, "https://index.docker.io/v1/")
	}
	return keys
}

func probeDockerLike(source, path, host string) (Candidate, error) {
	cfg, err := readDockerConfig(path)
	if err != nil {
		return Candidate{}, err
	}
	label := map[string]string{
		SourceDockerConfig: "Docker login",
		SourceHelmRegistry: "helm registry login",
	}[source]

	// Credential helper for this specific host?
	if helper, ok := cfg.CredHelpers[host]; ok {
		if u, _, err := credHelperGet(helper, host); err == nil {
			return Candidate{Source: source, Host: host, Username: u, Label: label + " (" + helper + " helper)"}, nil
		}
	}
	for _, k := range dockerAuthKeys(host) {
		if e, ok := cfg.Auths[k]; ok {
			u, _, err := decodeDockerAuth(e)
			if err != nil {
				continue
			}
			return Candidate{Source: source, Host: host, Username: u, Label: label}, nil
		}
	}
	// Global credsStore may hold the host even without an auths entry.
	if cfg.CredsStore != "" {
		if u, _, err := credHelperGet(cfg.CredsStore, host); err == nil {
			return Candidate{Source: source, Host: host, Username: u, Label: label + " (" + cfg.CredsStore + " store)"}, nil
		}
	}
	return Candidate{}, os.ErrNotExist
}

func probePodman(host string) (Candidate, error) {
	p, err := podmanAuthPath()
	if err != nil {
		return Candidate{}, err
	}
	c, err := probeDockerLike(SourcePodman, p, host)
	if err != nil {
		return Candidate{}, err
	}
	c.Label = "Podman login"
	return c, nil
}

func resolveDockerLike(path, host string) (string, string, error) {
	cfg, err := readDockerConfig(path)
	if err != nil {
		return "", "", err
	}
	if helper, ok := cfg.CredHelpers[host]; ok {
		if u, s, err := credHelperGet(helper, host); err == nil {
			return u, s, nil
		}
	}
	for _, k := range dockerAuthKeys(host) {
		if e, ok := cfg.Auths[k]; ok {
			u, s, err := decodeDockerAuth(e)
			if err == nil && s != "" {
				return u, s, nil
			}
		}
	}
	if cfg.CredsStore != "" {
		if u, s, err := credHelperGet(cfg.CredsStore, host); err == nil {
			return u, s, nil
		}
	}
	return "", "", fmt.Errorf("no credential for %s in %s", host, path)
}

func decodeDockerAuth(e dockerAuthEntry) (string, string, error) {
	if e.Username != "" && e.Password != "" {
		return e.Username, e.Password, nil
	}
	if e.Auth != "" {
		raw, err := base64.StdEncoding.DecodeString(e.Auth)
		if err != nil {
			return "", "", err
		}
		user, pass, ok := strings.Cut(string(raw), ":")
		if !ok {
			return "", "", fmt.Errorf("malformed auth entry")
		}
		return user, pass, nil
	}
	if e.IdentityToken != "" {
		return e.Username, e.IdentityToken, nil
	}
	return "", "", fmt.Errorf("empty auth entry")
}

// credHelperGet invokes `docker-credential-<helper> get` for host.
func credHelperGet(helper, host string) (string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker-credential-"+helper, "get")
	cmd.Stdin = strings.NewReader("https://" + host)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return "", "", fmt.Errorf("docker-credential-%s get %s failed: %w", helper, host, err)
	}
	var res struct {
		Username string `json:"Username"`
		Secret   string `json:"Secret"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		return "", "", err
	}
	if res.Secret == "" {
		return "", "", fmt.Errorf("docker-credential-%s returned no secret for %s", helper, host)
	}
	return res.Username, res.Secret, nil
}

// --- git credential helpers ---

// gitCredentialFill asks the user's configured git credential helpers for
// https creds, with every interactive prompt disabled so it can never hang
// or pop a dialog.
func gitCredentialFill(ctx context.Context, host string) (string, string, error) {
	cctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, "git", "credential", "fill")
	cmd.Stdin = strings.NewReader("protocol=https\nhost=" + host + "\n\n")
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		// Neutralize askpass programs: an empty value disables them.
		"GIT_ASKPASS=",
		"SSH_ASKPASS=",
		// Git Credential Manager would otherwise open a GUI prompt.
		"GCM_INTERACTIVE=never",
	)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return "", "", fmt.Errorf("git credential fill found nothing for %s: %w", host, err)
	}
	var user, pass string
	for _, line := range strings.Split(stdout.String(), "\n") {
		if v, ok := strings.CutPrefix(line, "username="); ok {
			user = v
		}
		if v, ok := strings.CutPrefix(line, "password="); ok {
			pass = v
		}
	}
	if pass == "" {
		return "", "", fmt.Errorf("git credential fill returned no password for %s", host)
	}
	return user, pass, nil
}

func probeGitCredential(ctx context.Context, host string) (Candidate, error) {
	u, _, err := gitCredentialFill(ctx, host)
	if err != nil {
		return Candidate{}, err
	}
	return Candidate{Source: SourceGitCredential, Host: host, Username: u, Label: "git credential helper"}, nil
}

// --- gh CLI (GitHub) ---

func resolveGhCLI(ctx context.Context, host string) (string, string, error) {
	cctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, "gh", "auth", "token", "--hostname", host)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return "", "", fmt.Errorf("gh auth token failed for %s: %w", host, err)
	}
	token := strings.TrimSpace(stdout.String())
	if token == "" {
		return "", "", fmt.Errorf("gh returned an empty token for %s", host)
	}
	username := ghUsername(ctx, host)
	if username == "" {
		// GHCR and git-over-https accept a PAT with any non-empty username.
		username = "oauth2"
	}
	return username, token, nil
}

func ghUsername(ctx context.Context, host string) string {
	cctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, "gh", "api", "user", "-q", ".login", "--hostname", host)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return ""
	}
	return strings.TrimSpace(stdout.String())
}

func probeGhCLI(ctx context.Context, host string) (Candidate, error) {
	// gh only ever holds GitHub credentials; skip the exec for other hosts.
	if host != "github.com" && !strings.HasSuffix(host, ".ghe.com") && !strings.Contains(host, "github") {
		return Candidate{}, os.ErrNotExist
	}
	cctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, "gh", "auth", "token", "--hostname", host)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return Candidate{}, err
	}
	return Candidate{Source: SourceGhCLI, Host: host, Username: ghUsername(ctx, host), Label: "GitHub CLI (gh)"}, nil
}

// --- glab CLI (GitLab) ---

type glabHostEntry struct {
	Token    string `yaml:"token"`
	User     string `yaml:"user"`
	Username string `yaml:"username"`
}

type glabConfigFile struct {
	Hosts map[string]glabHostEntry `yaml:"hosts"`
}

func glabConfigPath() string {
	if v := strings.TrimSpace(os.Getenv("GLAB_CONFIG_DIR")); v != "" {
		return filepath.Join(v, "config.yml")
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "glab-cli", "config.yml")
}

func readGlabHost(host string) (glabHostEntry, error) {
	path := glabConfigPath()
	if path == "" {
		return glabHostEntry{}, os.ErrNotExist
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return glabHostEntry{}, err
	}
	var cfg glabConfigFile
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return glabHostEntry{}, fmt.Errorf("parse %s: %w", path, err)
	}
	e, ok := cfg.Hosts[host]
	if !ok || strings.TrimSpace(e.Token) == "" {
		return glabHostEntry{}, os.ErrNotExist
	}
	return e, nil
}

func glabUsername(e glabHostEntry) string {
	if e.User != "" {
		return e.User
	}
	return e.Username
}

func probeGlabCLI(host string) (Candidate, error) {
	e, err := readGlabHost(host)
	if err != nil {
		return Candidate{}, err
	}
	return Candidate{Source: SourceGlabCLI, Host: host, Username: glabUsername(e), Label: "GitLab CLI (glab)"}, nil
}

func resolveGlabCLI(host string) (string, string, error) {
	e, err := readGlabHost(host)
	if err != nil {
		return "", "", fmt.Errorf("no glab token for %s: %w", host, err)
	}
	username := glabUsername(e)
	if username == "" {
		// GitLab accepts any non-empty username alongside a PAT.
		username = "oauth2"
	}
	return username, e.Token, nil
}
