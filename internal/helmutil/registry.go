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
	// Interactive: helm may prompt, and password-stdin reads from stdin.
	return runInteractive(ctx, env, "", "helm", registryLoginArgs(registry, opt.Username, opt.Password, opt.PasswordStdin)...)
}

// RegistryLoginStdin runs `helm registry login` non-interactively, piping the
// secret through stdin. Helm validates the credentials against the registry
// and, on success, persists them into env.RegistryConfig.
func RegistryLoginStdin(ctx context.Context, env Env, registry, username, secret string) error {
	if err := env.EnsureDirs(); err != nil {
		return err
	}
	registry = strings.TrimSpace(registry)
	if registry == "" {
		return fmt.Errorf("registry is required")
	}
	if strings.TrimSpace(secret) == "" {
		return fmt.Errorf("secret is required")
	}
	args := registryLoginArgs(registry, username, "", true)
	_, err := runWith(ctx, env, strings.NewReader(secret), "helm", args...)
	return err
}

func registryLoginArgs(registry, username, password string, passwordStdin bool) []string {
	args := []string{"registry", "login", registry}
	if strings.TrimSpace(username) != "" {
		args = append(args, "--username", username)
	}
	if passwordStdin {
		args = append(args, "--password-stdin")
	}
	if strings.TrimSpace(password) != "" {
		args = append(args, "--password", password)
	}
	return args
}
