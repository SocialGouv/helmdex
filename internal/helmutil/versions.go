package helmutil

import (
	"context"
	"encoding/json"
	"fmt"
	semver "github.com/Masterminds/semver/v3"
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

	ref := repoName + "/" + chartName

	// Search-first: try listing versions immediately. In normal operation the
	// repo index already exists, and this avoids doing a potentially expensive
	// `helm repo update` on every UI interaction.
	vs, err := searchRepoVersions(ctx, env, ref)
	if err == nil && len(vs) > 0 {
		return vs, nil
	}

	// If empty or failed, retry after a repo update.
	//
	// Important: if the search returned *empty*, we force a repo update even if
	// the stale marker is fresh. This handles cases where `helm repo add` did not
	// actually populate the index cache, or the cache was cleared.
	updateMaxAge := repoUpdateMaxAge
	if err == nil && len(vs) == 0 {
		updateMaxAge = 0
	}
	updateErr := RepoUpdateIfStale(ctx, env, updateMaxAge)
	vs2, err2 := searchRepoVersions(ctx, env, ref)
	if err2 == nil && len(vs2) > 0 {
		return vs2, nil
	}

	// Fallback: include pre-releases. Keep the best error context.
	vs3, err3 := searchRepoVersionsDevel(ctx, env, ref)
	if err3 == nil {
		return vs3, nil
	}
	// Prefer returning the original search error, but include update context.
	if err != nil {
		if updateErr != nil {
			return nil, fmt.Errorf("%w (repo update error: %v)", err, updateErr)
		}
		return nil, err
	}
	// Original search returned empty; return the most informative error.
	if err2 != nil {
		if updateErr != nil {
			return nil, fmt.Errorf("%w (repo update error: %v)", err2, updateErr)
		}
		return nil, err2
	}
	if updateErr != nil {
		return nil, fmt.Errorf("no versions found for %s (repo update error: %v)", ref, updateErr)
	}
	return nil, fmt.Errorf("no versions found for %s", ref)
}

type helmSearchRepoItem struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func searchRepoVersions(ctx context.Context, env Env, ref string) ([]string, error) {
	// Helm search uses substring matching by default, so searching for
	// `repo/postgresql` can also return `repo/postgresql-ha`.
	//
	// We avoid `--regexp` here because Helm's regexp matching semantics vary
	// across versions (sometimes it matches just the chart name, sometimes it
	// includes the repo prefix). Instead we always filter the JSON results by an
	// exact chart match.
	out, err := run(ctx, env, "helm", "search", "repo", ref, "--versions", "-o", "json")
	if err != nil {
		return nil, err
	}
	return parseSearchRepoVersionsForRef(out, ref)
}

func searchRepoVersionsDevel(ctx context.Context, env Env, ref string) ([]string, error) {
	out, err := run(ctx, env, "helm", "search", "repo", ref, "--versions", "--devel", "-o", "json")
	if err != nil {
		return nil, err
	}
	return parseSearchRepoVersionsForRef(out, ref)
}

func parseSearchRepoVersionsForRef(raw, ref string) ([]string, error) {
	var items []helmSearchRepoItem
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, fmt.Errorf("parse helm search repo json: %w", err)
	}
	// Helm typically returns names like `repo/chart`, but for safety accept
	// `chart` too.
	chartOnly := ref
	if i := strings.LastIndex(ref, "/"); i >= 0 {
		chartOnly = ref[i+1:]
	}
	seen := map[string]struct{}{}
	vs := make([]string, 0, len(items))
	for _, it := range items {
		name := strings.TrimSpace(it.Name)
		if name != strings.TrimSpace(ref) && name != strings.TrimSpace(chartOnly) {
			continue
		}
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
	// Stable ordering for UI:
	// 1) SemVer descending when parseable
	// 2) otherwise lexical descending as a fallback
	sort.Slice(vs, func(i, j int) bool {
		vi := strings.TrimSpace(vs[i])
		vj := strings.TrimSpace(vs[j])
		ai, ei := semver.NewVersion(vi)
		aj, ej := semver.NewVersion(vj)
		if ei == nil && ej == nil {
			return ai.GreaterThan(aj)
		}
		if ei == nil && ej != nil {
			return true
		}
		if ei != nil && ej == nil {
			return false
		}
		// Neither is SemVer: lexical desc.
		return vi > vj
	})
	return vs, nil
}
