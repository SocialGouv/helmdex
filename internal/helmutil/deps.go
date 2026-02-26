package helmutil

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func DependencyUpdate(ctx context.Context, env Env, chartDir string) error {
	// Defensive: Helm loads the chart directory and enforces a per-file size cap.
	// If a stray `.helmdex/` directory exists inside the chart (e.g. from running
	// helmdex with a wrong `--repo`), `helm dependency *` can fail on large cache
	// files unless `.helmignore` excludes it.
	_ = ensureHelmignoreIgnoresHelmdex(chartDir)
	_, err := runQuiet(ctx, env, chartDir, "helm", "dependency", "update")
	return err
}

func DependencyBuild(ctx context.Context, env Env, chartDir string) error {
	_ = ensureHelmignoreIgnoresHelmdex(chartDir)
	_, err := runQuiet(ctx, env, chartDir, "helm", "dependency", "build")
	return err
}

func runQuiet(ctx context.Context, env Env, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = isolatedProcessEnv(cmd.Environ(), env)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	tcmdStderr := &stderr
	cmd.Stderr = tcmdStderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s %v failed: %s", name, args, msg)
	}
	return stdout.String(), nil
}

func ensureHelmignoreIgnoresHelmdex(chartDir string) error {
	// Only do anything when a `.helmdex/` directory actually exists in the chart.
	if _, err := os.Stat(filepath.Join(chartDir, ".helmdex")); err != nil {
		return nil
	}
	ignorePath := filepath.Join(chartDir, ".helmignore")
	const needle = ".helmdex/"

	b, err := os.ReadFile(ignorePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Create a minimal ignore file.
			return os.WriteFile(ignorePath, []byte(needle+"\n"), 0o644)
		}
		return err
	}
	if strings.Contains(string(b), needle) {
		return nil
	}
	// Append.
	if len(b) > 0 && b[len(b)-1] != '\n' {
		b = append(b, '\n')
	}
	b = append(b, []byte(needle+"\n")...)
	return os.WriteFile(ignorePath, b, 0o644)
}
