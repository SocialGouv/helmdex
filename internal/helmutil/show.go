package helmutil

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Env struct {
	// RepoRoot is the helmdex repo root used to locate shared state (e.g. registry creds).
	RepoRoot   string
	ConfigHome string
	CacheHome  string
	DataHome   string

	// Home is an isolated HOME directory for helm subprocesses.
	Home string

	// DockerConfig is an isolated Docker config dir, preventing Helm/ORAS from
	// reading user-global ~/.docker/config.json.
	DockerConfig string

	// RegistryConfig is the shared Helm registry config file path.
	// We deliberately store this once per repoRoot (not per instance) so OCI login
	// is shared across instances.
	RegistryConfig string
}

func EnvForRepo(repoRoot string) Env {
	base := filepath.Join(repoRoot, ".helmdex", "helm")
	return envForBase(repoRoot, base)
}

// EnvForRepoURL scopes helm state per repository URL so `helm repo update` only
// touches the repo(s) for the current session/URL.
func EnvForRepoURL(repoRoot, repoURL string) Env {
	base := filepath.Join(repoRoot, ".helmdex", "helm", RepoNameForURL(repoURL))
	return envForBase(repoRoot, base)
}

// EnvForInstancePath scopes Helm state for dependency operations to a single
// instance directory. This prevents repos and caches from accumulating across a
// monorepo when relocking different instances.
func EnvForInstancePath(repoRoot, instancePath string) Env {
	// Stable name: hash the instance path relative to repoRoot.
	rel, err := filepath.Rel(repoRoot, instancePath)
	if err != nil {
		rel = instancePath
	}
	h := sha1.Sum([]byte(filepath.Clean(rel)))
	name := "helmdex-" + hex.EncodeToString(h[:8])
	base := filepath.Join(repoRoot, ".helmdex", "helm", "instances", name)
	return envForBase(repoRoot, base)
}

func envForBase(repoRoot, base string) Env {
	return Env{
		RepoRoot:       repoRoot,
		ConfigHome:     filepath.Join(base, "config"),
		CacheHome:      filepath.Join(base, "cache"),
		DataHome:       filepath.Join(base, "data"),
		Home:           filepath.Join(base, "home"),
		DockerConfig:   filepath.Join(base, "docker"),
		RegistryConfig: filepath.Join(repoRoot, ".helmdex", "helm", "registry", "config.json"),
	}
}

// RepoUpdateIfStale runs `helm repo update` only if the last update marker is
// older than maxAge, or missing.
func RepoUpdateIfStale(ctx context.Context, env Env, maxAge time.Duration) error {
	if err := env.EnsureDirs(); err != nil {
		return err
	}
	marker := repoUpdateMarkerPath(env)
	if st, err := os.Stat(marker); err == nil {
		if time.Since(st.ModTime()) < maxAge {
			return nil
		}
	}
	// Prefer targeted updates to avoid touching unrelated repos.
	// This fallback updates all repos *in this env*, but still passes explicit names.
	if err := RepoUpdate(ctx, env); err != nil {
		return err
	}
	_ = os.WriteFile(marker, []byte(time.Now().UTC().Format(time.RFC3339)), 0o644)
	return nil
}

func repoUpdateMarkerPath(env Env) string {
	return filepath.Join(env.CacheHome, "helmdex-repo-update.stamp")
}

func (e Env) EnsureDirs() error {
	for _, p := range []string{e.ConfigHome, e.CacheHome, e.DataHome, e.Home, e.DockerConfig, filepath.Dir(e.RegistryConfig)} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func RepoNameForURL(u string) string {
	// Stable, filesystem/helm-safe repo name.
	h := sha1.Sum([]byte(u))
	return "helmdex-" + hex.EncodeToString(h[:8])
}

func RepoAdd(ctx context.Context, env Env, name, url string) error {
	if err := env.EnsureDirs(); err != nil {
		return err
	}

	// Avoid `helm repo add --force-update` because it refreshes index data every
	// time and can be slow/heavy in constrained environments.
	//
	// Instead:
	// - if the repo already exists with the same URL: noop
	// - if it exists but URL differs: update it (rare; RepoNameForURL is URL-hashed)
	// - if it doesn't exist: add it
	repos, err := repoList(ctx, env)
	if err == nil {
		if existingURL, ok := repos[name]; ok {
			if strings.TrimSpace(existingURL) == strings.TrimSpace(url) {
				return nil
			}
			_, err = run(ctx, env, "helm", "repo", "add", name, url, "--force-update")
			return err
		}
	}

	// Fallback: try to add without forcing an update.
	_, err2 := run(ctx, env, "helm", "repo", "add", name, url)
	if err2 != nil {
		// If helm says the repo exists already, treat as success.
		s := err2.Error()
		if strings.Contains(s, "already exists") {
			// Ensure a fresh marker exists so stale logic doesn't think we never updated.
			_ = os.WriteFile(repoUpdateMarkerPath(env), []byte(time.Now().UTC().Format(time.RFC3339)), 0o644)
			return nil
		}
		// If we couldn't list repos (older helm / unexpected output), returning the
		// add error is the best we can do.
		return err2
	}
	// Adding a repo typically populates the index; write a marker so stale-update
	// logic has a baseline even if we never explicitly run `helm repo update`.
	_ = os.WriteFile(repoUpdateMarkerPath(env), []byte(time.Now().UTC().Format(time.RFC3339)), 0o644)
	return nil
}

func RepoUpdate(ctx context.Context, env Env) error {
	if err := env.EnsureDirs(); err != nil {
		return err
	}
	repos, err := repoList(ctx, env)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(repos))
	for n := range repos {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		names = append(names, n)
	}
	sort.Strings(names)
	return RepoUpdateNames(ctx, env, names...)
}

// RepoUpdateNames runs `helm repo update <name...>`.
// Passing explicit names avoids updating unrelated repos that may exist in the env.
func RepoUpdateNames(ctx context.Context, env Env, names ...string) error {
	if err := env.EnsureDirs(); err != nil {
		return err
	}
	uniq := make([]string, 0, len(names))
	seen := map[string]struct{}{}
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		uniq = append(uniq, n)
	}
	if len(uniq) == 0 {
		return nil
	}
	args := append([]string{"repo", "update"}, uniq...)
	_, err := run(ctx, env, "helm", args...)
	return err
}

// RepoUpdateIfStaleNames is like RepoUpdateIfStale but updates only the given repo names.
func RepoUpdateIfStaleNames(ctx context.Context, env Env, maxAge time.Duration, names ...string) error {
	if err := env.EnsureDirs(); err != nil {
		return err
	}
	marker := repoUpdateMarkerPath(env)
	if st, err := os.Stat(marker); err == nil {
		if time.Since(st.ModTime()) < maxAge {
			return nil
		}
	}
	if err := RepoUpdateNames(ctx, env, names...); err != nil {
		return err
	}
	_ = os.WriteFile(marker, []byte(time.Now().UTC().Format(time.RFC3339)), 0o644)
	return nil
}

func IsRepoUpdateWorthRetrying(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	// If the process was killed (OOM or user kill), retrying immediately is
	// unlikely to help.
	if strings.Contains(s, "signal: killed") {
		return false
	}
	return true
}

func ShowReadme(ctx context.Context, env Env, ref, version string) (string, error) {
	args := []string{"show", "readme", ref}
	if version != "" {
		args = append(args, "--version", version)
	}
	out, err := run(ctx, env, "helm", args...)
	return out, err
}

func ShowChart(ctx context.Context, env Env, ref, version string) (string, error) {
	args := []string{"show", "chart", ref}
	if version != "" {
		args = append(args, "--version", version)
	}
	out, err := run(ctx, env, "helm", args...)
	return out, err
}

func ShowValues(ctx context.Context, env Env, ref, version string) (string, error) {
	args := []string{"show", "values", ref}
	if version != "" {
		args = append(args, "--version", version)
	}
	out, err := run(ctx, env, "helm", args...)
	return out, err
}

// ShowReadmeBestEffort tries `helm show readme` with minimal side effects:
// - attempt directly (no repo update)
// - if it fails, try a stale-aware update and retry once
func ShowReadmeBestEffort(ctx context.Context, env Env, ref, version string, repoUpdateMaxAge time.Duration) (string, error) {
	out, err := ShowReadme(ctx, env, ref, version)
	if err == nil {
		return out, nil
	}
	// If ref is `repoName/chart`, update only that repoName.
	name := ""
	if i := strings.Index(ref, "/"); i > 0 {
		name = strings.TrimSpace(ref[:i])
	}
	if name != "" {
		if err2 := RepoUpdateIfStaleNames(ctx, env, repoUpdateMaxAge, name); err2 != nil {
			if !IsRepoUpdateWorthRetrying(err2) {
				return "", err
			}
			// fall through and retry show even if update failed; sometimes repos aren't needed.
		}
		return ShowReadme(ctx, env, ref, version)
	}
	if err2 := RepoUpdateIfStale(ctx, env, repoUpdateMaxAge); err2 != nil {
		if !IsRepoUpdateWorthRetrying(err2) {
			return "", err
		}
		// fall through and retry show even if update failed; sometimes repos aren't needed.
	}
	return ShowReadme(ctx, env, ref, version)
}

// ShowValuesBestEffort is like ShowReadmeBestEffort, for default values.
func ShowValuesBestEffort(ctx context.Context, env Env, ref, version string, repoUpdateMaxAge time.Duration) (string, error) {
	out, err := ShowValues(ctx, env, ref, version)
	if err == nil {
		return out, nil
	}
	name := ""
	if i := strings.Index(ref, "/"); i > 0 {
		name = strings.TrimSpace(ref[:i])
	}
	if name != "" {
		if err2 := RepoUpdateIfStaleNames(ctx, env, repoUpdateMaxAge, name); err2 != nil {
			if !IsRepoUpdateWorthRetrying(err2) {
				return "", err
			}
		}
		return ShowValues(ctx, env, ref, version)
	}
	if err2 := RepoUpdateIfStale(ctx, env, repoUpdateMaxAge); err2 != nil {
		if !IsRepoUpdateWorthRetrying(err2) {
			return "", err
		}
	}
	return ShowValues(ctx, env, ref, version)
}

func run(ctx context.Context, env Env, name string, args ...string) (string, error) {
	if name == "helm" || name == "helm.exe" {
		p, err := helmCommandPath(ctx)
		if err != nil {
			return "", err
		}
		name = p
	}
	cmd := exec.CommandContext(ctx, name, args...)
	// IMPORTANT: set all Helm home vars *and* repo/registry vars so an ambient
	// user env (HELM_REPOSITORY_CONFIG, HELM_REPOSITORY_CACHE, HELM_PLUGINS, ...)
	// cannot leak global state into helmdex operations.
	cmd.Env = isolatedProcessEnv(os.Environ(), env)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// If the context is done, prefer surfacing ctx.Err() over an OS-level
		// "signal: killed" or other opaque exec error.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", fmt.Errorf("helm %s failed: %v", strings.Join(args, " "), ctxErr)
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("helm %s failed: %s", strings.Join(args, " "), msg)
	}
	return stdout.String(), nil
}

func runInteractive(ctx context.Context, env Env, dir string, name string, args ...string) error {
	if name == "helm" || name == "helm.exe" {
		p, err := helmCommandPath(ctx)
		if err != nil {
			return err
		}
		name = p
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = isolatedProcessEnv(os.Environ(), env)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func helmEnvVars(env Env) []string {
	// Helm derives repo paths from these, but explicit overrides ensure we don't
	// accidentally use any repo config/cache from the parent environment.
	repoCfg := filepath.Join(env.ConfigHome, "repositories.yaml")
	repoCache := filepath.Join(env.CacheHome, "repository")
	plugs := filepath.Join(env.DataHome, "plugins")
	regCfg := strings.TrimSpace(env.RegistryConfig)
	if regCfg == "" {
		regCfg = filepath.Join(env.ConfigHome, "registry", "config.json")
	}
	return []string{
		"HOME=" + env.Home,
		"XDG_CONFIG_HOME=" + env.ConfigHome,
		"XDG_CACHE_HOME=" + env.CacheHome,
		"XDG_DATA_HOME=" + env.DataHome,
		"DOCKER_CONFIG=" + env.DockerConfig,
		"HELM_CONFIG_HOME=" + env.ConfigHome,
		"HELM_CACHE_HOME=" + env.CacheHome,
		"HELM_DATA_HOME=" + env.DataHome,
		"HELM_REPOSITORY_CONFIG=" + repoCfg,
		"HELM_REPOSITORY_CACHE=" + repoCache,
		"HELM_REGISTRY_CONFIG=" + regCfg,
		"HELM_PLUGINS=" + plugs,
	}
}

func isolatedProcessEnv(parent []string, env Env) []string {
	// Remove variables that could cause Helm (or its OCI stack) to read user-global state.
	out := stripEnvPrefixes(parent, []string{"HELM_", "XDG_", "DOCKER_"})
	out = stripEnvKeys(out, []string{"HOME"})
	return append(out, helmEnvVars(env)...)
}

func stripEnvPrefixes(env []string, prefixes []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		keep := true
		for _, p := range prefixes {
			if strings.HasPrefix(kv, p) {
				keep = false
				break
			}
		}
		if keep {
			out = append(out, kv)
		}
	}
	return out
}

func stripEnvKeys(env []string, keys []string) []string {
	if len(keys) == 0 {
		return env
	}
	out := make([]string, 0, len(env))
	for _, kv := range env {
		keep := true
		for _, k := range keys {
			if strings.HasPrefix(kv, k+"=") {
				keep = false
				break
			}
		}
		if keep {
			out = append(out, kv)
		}
	}
	return out
}

// RepoRemove removes a repo by name from the env.
func RepoRemove(ctx context.Context, env Env, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	_, err := run(ctx, env, "helm", "repo", "remove", name)
	return err
}

// EnsureReposOnly prunes repos not in allowed, then adds any missing allowed repos.
// Keys of allowed are repo names, values are URLs.
func EnsureReposOnly(ctx context.Context, env Env, allowed map[string]string) error {
	if err := env.EnsureDirs(); err != nil {
		return err
	}

	repos, err := repoList(ctx, env)
	if err == nil {
		for name := range repos {
			if _, ok := allowed[name]; ok {
				continue
			}
			// Best-effort prune; if remove fails, bubble up because leftover repos
			// would reintroduce global update/perf issues.
			if err := RepoRemove(ctx, env, name); err != nil {
				return err
			}
		}
	}

	for name, url := range allowed {
		if err := RepoAdd(ctx, env, name, url); err != nil {
			return err
		}
	}
	return nil
}

type helmRepoListItem struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

func repoList(ctx context.Context, env Env) (map[string]string, error) {
	if err := env.EnsureDirs(); err != nil {
		return nil, err
	}
	out, err := run(ctx, env, "helm", "repo", "list", "-o", "json")
	if err != nil {
		return nil, err
	}
	var items []helmRepoListItem
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		return nil, fmt.Errorf("parse helm repo list json: %w", err)
	}
	m := make(map[string]string, len(items))
	for _, it := range items {
		if strings.TrimSpace(it.Name) == "" {
			continue
		}
		m[it.Name] = it.URL
	}
	return m, nil
}
