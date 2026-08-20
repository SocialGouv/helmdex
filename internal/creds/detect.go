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
	"sync"
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
//
// Sources are probed for every credential kind on purpose: forge PATs are
// cross-usable (a GitLab token found in a git helper also authenticates the
// GitLab registry, and vice versa).
func Detect(ctx context.Context, host string) []Candidate {
	host = strings.ToLower(strings.TrimSpace(host))
	hosts := append([]string{host}, RelatedHosts(host)...)

	type probeFn func(ctx context.Context, host string) (Candidate, error)
	type spec struct {
		host  string
		probe probeFn
	}
	var specs []spec
	for _, h := range hosts {
		for _, p := range []probeFn{probeDockerConfig, probePodman, probeHelmRegistry, probeGitCredential, probeGhCLI, probeGlabCLI} {
			specs = append(specs, spec{host: h, probe: p})
		}
	}

	// Several probes fork subprocesses (git, credential helpers) that can
	// each take seconds; run everything concurrently and keep declaration
	// order, which ranks exact-host results before related-host ones.
	results := make([]Candidate, len(specs))
	var wg sync.WaitGroup
	for i, s := range specs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if c, err := s.probe(ctx, s.host); err == nil {
				results[i] = c
			}
		}()
	}
	wg.Wait()

	var out []Candidate
	seen := map[string]struct{}{}
	for _, c := range results {
		if c.Source == "" {
			continue
		}
		key := c.Source + "|" + c.Host
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, c)
	}
	return out
}

// ResolveCandidate re-resolves the secret for a previously detected
// candidate. The secret never transits through the browser: the UI sends
// back (source, host) and the server resolves it locally.
func ResolveCandidate(ctx context.Context, source, host string) (username, secret string, err error) {
	host = strings.ToLower(strings.TrimSpace(host))
	switch source {
	case SourceDockerConfig:
		u, s, _, err := lookupDockerLike(dockerConfigPath(), host)
		return u, s, err
	case SourcePodman:
		p, perr := podmanAuthPath()
		if perr != nil {
			return "", "", perr
		}
		u, s, _, err := lookupDockerLike(p, host)
		return u, s, err
	case SourceHelmRegistry:
		u, s, _, err := lookupDockerLike(helmRegistryConfigPath(), host)
		return u, s, err
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

// lookupDockerLike resolves a credential from a docker-style config file:
// per-host credential helper first, then inline auths entries, then the
// global credsStore. labelSuffix names the helper involved, for UI labels.
func lookupDockerLike(path, host string) (username, secret, labelSuffix string, err error) {
	cfg, err := readDockerConfig(path)
	if err != nil {
		return "", "", "", err
	}
	if helper, ok := cfg.CredHelpers[host]; ok {
		if u, s, err := credHelperGet(helper, host); err == nil {
			return u, s, " (" + helper + " helper)", nil
		}
	}
	for _, k := range dockerAuthKeys(host) {
		if e, ok := cfg.Auths[k]; ok {
			if u, s, err := decodeDockerAuth(e); err == nil && s != "" {
				return u, s, "", nil
			}
		}
	}
	if cfg.CredsStore != "" {
		if u, s, err := credHelperGet(cfg.CredsStore, host); err == nil {
			return u, s, " (" + cfg.CredsStore + " store)", nil
		}
	}
	return "", "", "", fmt.Errorf("no credential for %s in %s", host, path)
}

// DockerAuthFor resolves a username/secret for host from a docker-style
// config file (credential helper, inline auths entry, then global credsStore).
//
// helmdex points HELM_REGISTRY_CONFIG at its own config and materializes the
// stored OCI credentials into it, so that one file also carries anything a
// plain `helm registry login` wrote.
func DockerAuthFor(configPath, host string) (username, secret string, ok bool) {
	u, s, _, err := lookupDockerLike(configPath, host)
	if err != nil || s == "" {
		return "", "", false
	}
	return u, s, true
}

func probeDockerConfig(_ context.Context, host string) (Candidate, error) {
	u, _, suffix, err := lookupDockerLike(dockerConfigPath(), host)
	if err != nil {
		return Candidate{}, err
	}
	return Candidate{Source: SourceDockerConfig, Host: host, Username: u, Label: "Docker login" + suffix}, nil
}

func probePodman(_ context.Context, host string) (Candidate, error) {
	p, err := podmanAuthPath()
	if err != nil {
		return Candidate{}, err
	}
	u, _, suffix, err := lookupDockerLike(p, host)
	if err != nil {
		return Candidate{}, err
	}
	return Candidate{Source: SourcePodman, Host: host, Username: u, Label: "Podman login" + suffix}, nil
}

func probeHelmRegistry(_ context.Context, host string) (Candidate, error) {
	u, _, suffix, err := lookupDockerLike(helmRegistryConfigPath(), host)
	if err != nil {
		return Candidate{}, err
	}
	return Candidate{Source: SourceHelmRegistry, Host: host, Username: u, Label: "helm registry login" + suffix}, nil
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

type ghHostEntry struct {
	User string `yaml:"user"`
}

func ghConfigPath() string {
	if v := strings.TrimSpace(os.Getenv("GH_CONFIG_DIR")); v != "" {
		return filepath.Join(v, "hosts.yml")
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "gh", "hosts.yml")
}

func readGhHost(host string) (ghHostEntry, error) {
	path := ghConfigPath()
	if path == "" {
		return ghHostEntry{}, os.ErrNotExist
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ghHostEntry{}, err
	}
	var hosts map[string]ghHostEntry
	if err := yaml.Unmarshal(b, &hosts); err != nil {
		return ghHostEntry{}, fmt.Errorf("parse %s: %w", path, err)
	}
	e, ok := hosts[host]
	if !ok {
		return ghHostEntry{}, os.ErrNotExist
	}
	return e, nil
}

// probeGhCLI reports a gh CLI login by reading gh's hosts.yml — no exec.
// The token itself often lives in the system keyring, so the host entry is
// the availability signal and `gh auth token` resolves the secret later.
func probeGhCLI(_ context.Context, host string) (Candidate, error) {
	// gh only ever holds GitHub credentials; skip other hosts.
	if !IsGitHubHost(host) {
		return Candidate{}, os.ErrNotExist
	}
	e, err := readGhHost(host)
	if err != nil {
		return Candidate{}, err
	}
	return Candidate{Source: SourceGhCLI, Host: host, Username: e.User, Label: "GitHub CLI (gh)"}, nil
}

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
	e, _ := readGhHost(host)
	return e.User, token, nil
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

func probeGlabCLI(_ context.Context, host string) (Candidate, error) {
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
	return glabUsername(e), e.Token, nil
}
