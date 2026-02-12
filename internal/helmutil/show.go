package helmutil

import (
	"bytes"
	"context"
	"crypto/sha1"
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
	// --force-update avoids errors if the repo exists.
	_, err := run(ctx, env, "helm", "repo", "add", name, url, "--force-update")
	return err
}

func RepoUpdate(ctx context.Context, env Env) error {
	if err := env.EnsureDirs(); err != nil {
		return err
	}
	_, err := run(ctx, env, "helm", "repo", "update")
	return err
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
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("helm %s failed: %s", strings.Join(args, " "), msg)
	}
	return stdout.String(), nil
}
