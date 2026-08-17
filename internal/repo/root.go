package repo

import (
	"fmt"
	"os"
	"path/filepath"
)

// ResolveRoot resolves the repo root directory.
// If explicitRoot is non-empty, it is returned.
// Otherwise, it searches from cwd upwards until it finds helmdex.yaml.
func ResolveRoot(explicitRoot string) (string, error) {
	if explicitRoot != "" {
		abs, err := filepath.Abs(explicitRoot)
		if err != nil {
			return "", err
		}
		return abs, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	// Make sure any returns are absolute and stable across cmd.Dir changes.
	cwdAbs, err := filepath.Abs(cwd)
	if err != nil {
		return "", err
	}

	dir := cwdAbs
	for {
		candidate := filepath.Join(dir, "helmdex.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// No helmdex.yaml: agnostic repo. Anchor on the nearest git root so
	// instance discovery sees the whole repo, not just the cwd.
	dir = cwdAbs
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// Fallback to cwd if neither exists yet (useful for `init`).
	return cwdAbs, nil
}

func RequireConfig(repoRoot string) (string, error) {
	path := filepath.Join(repoRoot, "helmdex.yaml")
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("missing helmdex.yaml at %s", path)
	}
	return path, nil
}
