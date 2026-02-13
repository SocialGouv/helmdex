package helmutil

import (
	"context"
	"fmt"
	"strings"
)

type RegistryLoginOptions struct {
	Username      string
	Password      string
	PasswordStdin bool
}

// RegistryLogin runs `helm registry login` inside the helmdex isolated env.
// Credentials are stored under env.RegistryConfig (which helmdex points at
// .helmdex/helm/registry/config.json for repo-root sharing).
func RegistryLogin(ctx context.Context, env Env, registry string, opt RegistryLoginOptions) error {
	if err := env.EnsureDirs(); err != nil {
		return err
	}
	registry = strings.TrimSpace(registry)
	if registry == "" {
		return fmt.Errorf("registry is required")
	}
	args := []string{"registry", "login", registry}
	if strings.TrimSpace(opt.Username) != "" {
		args = append(args, "--username", opt.Username)
	}
	if opt.PasswordStdin {
		args = append(args, "--password-stdin")
	}
	if strings.TrimSpace(opt.Password) != "" {
		args = append(args, "--password", opt.Password)
	}
	// Interactive: helm may prompt, and password-stdin reads from stdin.
	return runInteractive(ctx, env, "", "helm", args...)
}
