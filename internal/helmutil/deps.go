package helmutil

import (
	"context"
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

func DependencyUpdate(ctx context.Context, env Env, chartDir string) error {
	_, err := runQuiet(ctx, env, chartDir, "helm", "dependency", "update")
	return err
}

func DependencyBuild(ctx context.Context, env Env, chartDir string) error {
	_, err := runQuiet(ctx, env, chartDir, "helm", "dependency", "build")
	return err
}


func runQuiet(ctx context.Context, env Env, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(cmd.Environ(),
		"HELM_CONFIG_HOME="+env.ConfigHome,
		"HELM_CACHE_HOME="+env.CacheHome,
		"HELM_DATA_HOME="+env.DataHome,
	)
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
