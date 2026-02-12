package gitutil

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type CloneOrUpdateParams struct {
	URL    string
	Ref    string
	Commit string

	DestDir string
}

type CloneOrUpdateResult struct {
	ResolvedCommit string
}

func CloneOrUpdate(ctx context.Context, p CloneOrUpdateParams) (CloneOrUpdateResult, error) {
	if p.URL == "" {
		return CloneOrUpdateResult{}, fmt.Errorf("git url is required")
	}
	if p.DestDir == "" {
		return CloneOrUpdateResult{}, fmt.Errorf("dest dir is required")
	}

	if _, err := os.Stat(filepath.Join(p.DestDir, ".git")); err != nil {
		if err := run(ctx, "", "git", "clone", "--", p.URL, p.DestDir); err != nil {
			return CloneOrUpdateResult{}, err
		}
	} else {
		if err := run(ctx, p.DestDir, "git", "fetch", "--all", "--tags", "--prune"); err != nil {
			return CloneOrUpdateResult{}, err
		}
	}

	// Prefer fixed commit pin if provided.
	if p.Commit != "" {
		if err := run(ctx, p.DestDir, "git", "checkout", "--detach", p.Commit); err != nil {
			return CloneOrUpdateResult{}, err
		}
		return CloneOrUpdateResult{ResolvedCommit: p.Commit}, nil
	}

	if p.Ref == "" {
		p.Ref = "HEAD"
	}
	if err := run(ctx, p.DestDir, "git", "checkout", "--detach", p.Ref); err != nil {
		return CloneOrUpdateResult{}, err
	}

	sha, err := output(ctx, p.DestDir, "git", "rev-parse", "HEAD")
	if err != nil {
		return CloneOrUpdateResult{}, err
	}
	return CloneOrUpdateResult{ResolvedCommit: strings.TrimSpace(sha)}, nil
}

func run(ctx context.Context, cwd string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = cwd
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func output(ctx context.Context, cwd string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = cwd
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s %s failed: %w\n%s", name, strings.Join(args, " "), err, stderr.String())
	}
	return stdout.String(), nil
}

