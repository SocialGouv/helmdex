package helmutil

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"helmdex/internal/yamlchart"
)

// AllowedRepoSetForChart reads Chart.yaml and returns the set of classic (non-OCI)
// helm repo URLs referenced by dependencies.
//
// Notes:
// - file:// deps are ignored (no repo needed)
// - oci:// deps are ignored (resolved via registry creds, not helm repo add)
func AllowedRepoSetForChart(chartPath string) (map[string]string, error) {
	c, err := yamlchart.ReadChart(chartPath)
	if err != nil {
		return nil, err
	}
	allowed := map[string]string{}
	for _, d := range c.Dependencies {
		u := strings.TrimSpace(d.Repository)
		if u == "" {
			continue
		}
		if strings.HasPrefix(u, "oci://") {
			continue
		}
		if strings.HasPrefix(u, "file://") {
			continue
		}
		// Treat everything else as a classic helm repo URL.
		name := RepoNameForURL(u)
		allowed[name] = u
	}
	return allowed, nil
}

// PrepareDependencyEnv ensures the env for dependency operations contains only
// the repos referenced by the given chart (Chart.yaml).
func PrepareDependencyEnv(ctx context.Context, env Env, chartPath string) (map[string]string, error) {
	allowed, err := AllowedRepoSetForChart(chartPath)
	if err != nil {
		return nil, err
	}
	if err := EnsureReposOnly(ctx, env, allowed); err != nil {
		return nil, err
	}
	return allowed, nil
}

// ChartPathForInstance is a small helper used by callers.
func ChartPathForInstance(instancePath string) string {
	return filepath.Join(instancePath, "Chart.yaml")
}

func EnsureAllowedRepoUpdateStale(ctx context.Context, env Env, maxAge time.Duration, allowed map[string]string) error {
	if len(allowed) == 0 {
		return nil
	}
	names := make([]string, 0, len(allowed))
	for n := range allowed {
		names = append(names, n)
	}
	return RepoUpdateIfStaleNames(ctx, env, maxAge, names...)
}
