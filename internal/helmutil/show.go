package helmutil

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/json"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
	"strings"
)

type Env struct {
	ConfigHome string
	CacheHome  string
	DataHome   string
}

func EnvForRepo(repoRoot string) Env {
	base := filepath.Join(repoRoot, ".helmdex", "helm")
	return Env{
		ConfigHome: filepath.Join(base, "config"),
		CacheHome:  filepath.Join(base, "cache"),
		DataHome:   filepath.Join(base, "data"),
	}
}

// EnvForRepoURL scopes helm state per repository URL so `helm repo update` only
// touches the repo(s) for the current session/URL.
func EnvForRepoURL(repoRoot, repoURL string) Env {
	base := filepath.Join(repoRoot, ".helmdex", "helm", RepoNameForURL(repoURL))
	return Env{
		ConfigHome: filepath.Join(base, "config"),
		CacheHome:  filepath.Join(base, "cache"),
		DataHome:   filepath.Join(base, "data"),
	}
}

// RepoUpdateIfStale runs `helm repo update` only if the last update marker is
// older than maxAge, or missing.
func RepoUpdateIfStale(ctx context.Context, env Env, maxAge time.Duration) error {
	if err := env.EnsureDirs(); err != nil {
		return err
	}
	marker := filepath.Join(env.CacheHome, "helmdex-repo-update.stamp")
	if st, err := os.Stat(marker); err == nil {
		if time.Since(st.ModTime()) < maxAge {
			return nil
		}
	}
	if err := RepoUpdate(ctx, env); err != nil {
		return err
	}
	_ = os.WriteFile(marker, []byte(time.Now().UTC().Format(time.RFC3339)), 0o644)
	return nil
}

func (e Env) EnsureDirs() error {
	for _, p := range []string{e.ConfigHome, e.CacheHome, e.DataHome} {
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
			return nil
		}
		// If we couldn't list repos (older helm / unexpected output), returning the
		// add error is the best we can do.
		return err2
	}
	return nil
}

func RepoUpdate(ctx context.Context, env Env) error {
	if err := env.EnsureDirs(); err != nil {
		return err
	}
	_, err := run(ctx, env, "helm", "repo", "update")
	return err
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
	if err2 := RepoUpdateIfStale(ctx, env, repoUpdateMaxAge); err2 != nil {
		if !IsRepoUpdateWorthRetrying(err2) {
			return "", err
		}
	}
	return ShowValues(ctx, env, ref, version)
}

func run(ctx context.Context, env Env, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(),
		"HELM_CONFIG_HOME="+env.ConfigHome,
		"HELM_CACHE_HOME="+env.CacheHome,
		"HELM_DATA_HOME="+env.DataHome,
	)
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
