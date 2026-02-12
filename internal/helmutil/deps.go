package helmutil

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

func DependencyUpdate(ctx context.Context, chartDir string) error {
	return run(ctx, chartDir, "helm", "dependency", "update")
}

func DependencyBuild(ctx context.Context, chartDir string) error {
	return run(ctx, chartDir, "helm", "dependency", "build")
}

func run(ctx context.Context, dir, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %v failed: %w", name, args, err)
	}
	return nil
}

