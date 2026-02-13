package helmutil

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// RepoChartVersions returns available chart versions for a chart in a classic
// (non-OCI) Helm repository.
//
// It uses an isolated per-repoURL helm env (see EnvForRepoURL), ensures the repo
// is added, runs a stale-aware repo update, then queries versions via:
//   helm search repo <repo>/<chart> --versions -o json
func RepoChartVersions(ctx context.Context, repoRoot, repoURL, chartName string, repoUpdateMaxAge time.Duration) ([]string, error) {
	if strings.HasPrefix(repoURL, "oci://") {
		return nil, fmt.Errorf("helm search repo does not support OCI refs; cannot list versions for %s", repoURL)
	}
	if strings.TrimSpace(chartName) == "" {
		return nil, fmt.Errorf("chartName is required")
	}

	env := EnvForRepoURL(repoRoot, repoURL)
	repoName := RepoNameForURL(repoURL)
	if err := RepoAdd(ctx, env, repoName, repoURL); err != nil {
		return nil, err
	}

	// Best-effort update: if it fails, still try the search (index may already
	// exist). If the search fails too, include update context.
	updateErr := RepoUpdateIfStale(ctx, env, repoUpdateMaxAge)

	ref := repoName + "/" + chartName
	vs, err := searchRepoVersions(ctx, env, ref)
	if err != nil {
		if updateErr != nil {
			return nil, fmt.Errorf("%w (repo update error: %v)", err, updateErr)
		}
		return nil, err
	}
	if len(vs) > 0 {
		return vs, nil
	}

	// Fallback: include pre-releases.
	vs, err = searchRepoVersionsDevel(ctx, env, ref)
	if err != nil {
		return nil, err
	}
	return vs, nil
}

type helmSearchRepoItem struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func searchRepoVersions(ctx context.Context, env Env, ref string) ([]string, error) {
	out, err := run(ctx, env, "helm", "search", "repo", ref, "--versions", "-o", "json")
	if err != nil {
		return nil, err
	}
	return parseSearchRepoVersions(out)
}

func searchRepoVersionsDevel(ctx context.Context, env Env, ref string) ([]string, error) {
	out, err := run(ctx, env, "helm", "search", "repo", ref, "--versions", "--devel", "-o", "json")
	if err != nil {
		return nil, err
	}
	return parseSearchRepoVersions(out)
}

func parseSearchRepoVersions(raw string) ([]string, error) {
	var items []helmSearchRepoItem
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, fmt.Errorf("parse helm search repo json: %w", err)
	}
	seen := map[string]struct{}{}
	vs := make([]string, 0, len(items))
	for _, it := range items {
		v := strings.TrimSpace(it.Version)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		vs = append(vs, v)
	}
	// Stable ordering: descending for UI.
	sort.Strings(vs)
	for i, j := 0, len(vs)-1; i < j; i, j = i+1, j-1 {
		vs[i], vs[j] = vs[j], vs[i]
	}
	return vs, nil
}

