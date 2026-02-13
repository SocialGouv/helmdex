package helmutil

import (
	"strings"
	"testing"
)

func TestIsolatedProcessEnv_StripsAndOverrides(t *testing.T) {
	env := EnvForRepo("/tmp/helmdex-repo")
	parent := []string{
		"HOME=/home/user",
		"XDG_CONFIG_HOME=/home/user/.config",
		"DOCKER_CONFIG=/home/user/.docker",
		"HELM_CONFIG_HOME=/home/user/.config/helm",
		"PATH=/usr/bin",
	}

	out := isolatedProcessEnv(parent, env)

	// Helper: ensure no key appears from parent.
	hasPrefix := func(p string) bool {
		for _, kv := range out {
			if strings.HasPrefix(kv, p) {
				return true
			}
		}
		return false
	}

	if hasPrefix("HELM_CONFIG_HOME=/home/user") {
		t.Fatalf("expected HELM_* from parent to be stripped")
	}
	if hasPrefix("XDG_CONFIG_HOME=/home/user") {
		t.Fatalf("expected XDG_* from parent to be stripped")
	}
	if hasPrefix("DOCKER_CONFIG=/home/user") {
		t.Fatalf("expected DOCKER_* from parent to be stripped")
	}
	if hasPrefix("HOME=/home/user") {
		t.Fatalf("expected HOME from parent to be stripped")
	}

	// And required overrides should exist.
	if !hasPrefix("HOME="+env.Home) {
		t.Fatalf("expected HOME to be overridden")
	}
	if !hasPrefix("DOCKER_CONFIG="+env.DockerConfig) {
		t.Fatalf("expected DOCKER_CONFIG to be overridden")
	}
	if !hasPrefix("XDG_CONFIG_HOME="+env.ConfigHome) {
		t.Fatalf("expected XDG_CONFIG_HOME to be overridden")
	}
	if !hasPrefix("HELM_REGISTRY_CONFIG="+env.RegistryConfig) {
		t.Fatalf("expected HELM_REGISTRY_CONFIG to be set to repo-root shared registry config")
	}
}

