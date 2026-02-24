package helmutil

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ReadChartArchiveFiles reads README and values.yaml from a Helm chart archive
// (a .tgz produced by `helm pull`). It returns empty strings when the files
// aren't present.
func ReadChartArchiveFiles(tgzPath string) (readme string, values string, err error) {
	readme, values, _, err = ReadChartArchiveFilesWithSchema(tgzPath)
	return readme, values, err
}

// ReadChartArchiveFilesWithSchema reads README, values.yaml, and values.schema.json
// from a Helm chart archive (a .tgz produced by `helm pull`).
//
// It prefers top-level chart files (<chartname>/...) and will not accidentally
// return a subchart's files from <chartname>/charts/<subchart>/...
//
// It returns empty strings when files aren't present.
func ReadChartArchiveFilesWithSchema(tgzPath string) (readme string, values string, schema string, err error) {
	f, err := os.Open(tgzPath)
	if err != nil {
		return "", "", "", err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", "", "", fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	var readmeFallback string
	var valuesFallback string
	var schemaFallback string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", "", "", fmt.Errorf("tar read: %w", err)
		}
		name := filepath.ToSlash(strings.TrimLeft(h.Name, "/"))
		if strings.HasSuffix(name, "/") {
			continue
		}
		base := filepath.Base(name)
		parts := strings.Split(name, "/")
		depth := len(parts)
		// Prefer top-level chart files: <chartname>/values.yaml and <chartname>/README.md.
		// Many charts embed subcharts under <chartname>/charts/<subchart>/values.yaml
		// (e.g. bitnami/common). Those must NOT override the parent chart's values.
		isReadme := strings.EqualFold(base, "README.md") || strings.EqualFold(base, "readme.md")
		isValues := strings.EqualFold(base, "values.yaml")
		isSchema := strings.EqualFold(base, "values.schema.json")
		if !isReadme && !isValues && !isSchema {
			continue
		}
		b, _ := io.ReadAll(tr)
		content := string(b)

		// depth==2 corresponds to <chartname>/<file>.
		if depth == 2 {
			if isReadme {
				readme = content
			} else if isValues {
				values = content
			} else {
				schema = content
			}
		} else {
			// Keep as a fallback only.
			if isReadme && readmeFallback == "" {
				readmeFallback = content
			}
			if isValues && valuesFallback == "" {
				valuesFallback = content
			}
			if isSchema && schemaFallback == "" {
				schemaFallback = content
			}
		}

		if readme != "" && values != "" && schema != "" {
			break
		}
	}
	if readme == "" {
		readme = readmeFallback
	}
	if values == "" {
		values = valuesFallback
	}
	if schema == "" {
		schema = schemaFallback
	}
	return readme, values, schema, nil
}

// FindCachedChartArchive tries to locate a previously-downloaded chart archive
// in Helm's repository cache directories.
//
// It checks:
// - the provided isolated env cache (preferred)
// - the shared repo-level helm env cache
func FindCachedChartArchive(repoRoot, repoURL, chartName, version string) (string, bool) {
	if strings.TrimSpace(chartName) == "" || strings.TrimSpace(version) == "" {
		return "", false
	}
	tgz := fmt.Sprintf("%s-%s.tgz", chartName, version)

	// 1) per-repoURL isolated env
	env := EnvForRepoURL(repoRoot, repoURL)
	p1 := filepath.Join(env.CacheHome, "repository", tgz)
	if st, err := os.Stat(p1); err == nil && !st.IsDir() {
		return p1, true
	}
	// 2) repo-level shared env
	shared := EnvForRepo(repoRoot)
	p2 := filepath.Join(shared.CacheHome, "repository", tgz)
	if st, err := os.Stat(p2); err == nil && !st.IsDir() {
		return p2, true
	}
	return "", false
}

// PullChartArchive pulls the chart archive into the isolated env cache and
// returns the path to the downloaded .tgz.
func PullChartArchive(ctx context.Context, env Env, repoURL, chartName, version string) (string, error) {
	if strings.TrimSpace(chartName) == "" {
		return "", fmt.Errorf("chartName is required")
	}
	if strings.TrimSpace(version) == "" {
		return "", fmt.Errorf("version is required")
	}
	if strings.HasPrefix(repoURL, "oci://") {
		ref, err := OCIChartRef(repoURL, chartName)
		if err != nil {
			return "", err
		}
		// For OCI, helm pull stores <chartName>-<version>.tgz in dest.
		dest := filepath.Join(env.CacheHome, "repository")
		if err := env.EnsureDirs(); err != nil {
			return "", err
		}
		if _, err := run(ctx, env, "helm", "pull", ref, "--version", version, "--destination", dest); err != nil {
			return "", err
		}
		p := filepath.Join(dest, fmt.Sprintf("%s-%s.tgz", chartName, version))
		if _, err := os.Stat(p); err != nil {
			return "", err
		}
		return p, nil
	}

	repoName := RepoNameForURL(repoURL)
	if err := RepoAdd(ctx, env, repoName, repoURL); err != nil {
		return "", err
	}
	ref := repoName + "/" + chartName
	dest := filepath.Join(env.CacheHome, "repository")
	if err := env.EnsureDirs(); err != nil {
		return "", err
	}
	if _, err := run(ctx, env, "helm", "pull", ref, "--version", version, "--destination", dest); err != nil {
		// If pull failed, try a targeted stale-aware update and retry once.
		_ = RepoUpdateIfStaleNames(ctx, env, 0, repoName)
		if _, err2 := run(ctx, env, "helm", "pull", ref, "--version", version, "--destination", dest); err2 != nil {
			return "", err
		}
	}
	// Helm pull writes <chartName>-<version>.tgz into destination.
	p := filepath.Join(dest, fmt.Sprintf("%s-%s.tgz", chartName, version))
	if _, err := os.Stat(p); err != nil {
		return "", err
	}
	return p, nil
}
