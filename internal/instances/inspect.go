package instances

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"helmdex/internal/helmutil"
	"helmdex/internal/yamlchart"
)

// DepByID finds a dependency by its id (alias or name).
func DepByID(chart yamlchart.Chart, id string) (yamlchart.Dependency, error) {
	want := yamlchart.DepID(strings.TrimSpace(id))
	for _, d := range chart.Dependencies {
		if yamlchart.DependencyID(d) == want {
			return d, nil
		}
	}
	return yamlchart.Dependency{}, fmt.Errorf("dependency %q not found", want)
}

func readVendoredChartFile(instancePath string, dep yamlchart.Dependency, rel string) (string, bool, error) {
	base := filepath.Join(instancePath, "charts", dep.Name)
	st, err := os.Stat(base)
	if err != nil || !st.IsDir() {
		return "", false, nil
	}
	p := filepath.Join(base, rel)
	b, err := os.ReadFile(p)
	if err != nil {
		return "", false, nil
	}
	out := string(b)
	if strings.TrimSpace(out) == "" {
		return "", false, nil
	}
	return out, true, nil
}

// InspectKind selects which dependency artifact to load.
type InspectKind string

const (
	InspectReadme InspectKind = "readme"
	InspectValues InspectKind = "values"
	InspectSchema InspectKind = "schema"
)

// LoadDepInspectContent loads a dependency artifact (readme / default
// values / schema), trying vendored charts, caches, then the network.
func LoadDepInspectContent(ctx context.Context, repoRoot string, instPath string, dep yamlchart.Dependency, kind InspectKind) (string, error) {
	// 0) Vendored chart files
	switch kind {
	case InspectReadme:
		if s, ok, err := readVendoredChartFile(instPath, dep, "README.md"); err != nil {
			return "", err
		} else if ok {
			_ = helmutil.WriteShowCache(repoRoot, dep.Repository, dep.Name, dep.Version, helmutil.ShowKindReadme, s)
			return s, nil
		}
	case InspectValues:
		if s, ok, err := readVendoredChartFile(instPath, dep, "values.yaml"); err != nil {
			return "", err
		} else if ok {
			_ = helmutil.WriteShowCache(repoRoot, dep.Repository, dep.Name, dep.Version, helmutil.ShowKindValues, s)
			return s, nil
		}
	case InspectSchema:
		if s, ok, err := readVendoredChartFile(instPath, dep, "values.schema.json"); err != nil {
			return "", err
		} else if ok {
			_ = helmutil.WriteShowCache(repoRoot, dep.Repository, dep.Name, dep.Version, helmutil.ShowKindSchema, s)
			return s, nil
		}
	}

	// 1) Cached chart archive (.tgz)
	if tgzPath, ok := helmutil.FindCachedChartArchive(repoRoot, dep.Repository, dep.Name, dep.Version); ok {
		readme, values, schema, err := helmutil.ReadChartArchiveFilesWithSchema(tgzPath)
		if err == nil {
			if strings.TrimSpace(readme) != "" {
				_ = helmutil.WriteShowCache(repoRoot, dep.Repository, dep.Name, dep.Version, helmutil.ShowKindReadme, readme)
			}
			if strings.TrimSpace(values) != "" {
				_ = helmutil.WriteShowCache(repoRoot, dep.Repository, dep.Name, dep.Version, helmutil.ShowKindValues, values)
			}
			if strings.TrimSpace(schema) != "" {
				_ = helmutil.WriteShowCache(repoRoot, dep.Repository, dep.Name, dep.Version, helmutil.ShowKindSchema, schema)
			}
			switch kind {
			case InspectReadme:
				if strings.TrimSpace(readme) != "" {
					return readme, nil
				}
			case InspectValues:
				if strings.TrimSpace(values) != "" {
					return values, nil
				}
			case InspectSchema:
				if strings.TrimSpace(schema) != "" {
					return schema, nil
				}
			}
		}
	}

	// 2) helmdex show cache
	switch kind {
	case InspectReadme:
		if s, ok, err := helmutil.ReadShowCache(repoRoot, dep.Repository, dep.Name, dep.Version, helmutil.ShowKindReadme); err != nil {
			return "", err
		} else if ok {
			return s, nil
		}
	case InspectValues:
		if s, ok, err := helmutil.ReadShowCache(repoRoot, dep.Repository, dep.Name, dep.Version, helmutil.ShowKindValues); err != nil {
			return "", err
		} else if ok {
			return s, nil
		}
	case InspectSchema:
		if s, ok, err := helmutil.ReadShowCache(repoRoot, dep.Repository, dep.Name, dep.Version, helmutil.ShowKindSchema); err != nil {
			return "", err
		} else if ok {
			return s, nil
		}
	}

	// 3) Pull chart archive and read
	env := helmutil.EnvForRepoURL(repoRoot, dep.Repository)
	ctx2, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if tgzPath, err := helmutil.PullChartArchive(ctx2, env, dep.Repository, dep.Name, dep.Version); err == nil {
		readme, values, schema, err2 := helmutil.ReadChartArchiveFilesWithSchema(tgzPath)
		if err2 == nil {
			if strings.TrimSpace(readme) != "" {
				_ = helmutil.WriteShowCache(repoRoot, dep.Repository, dep.Name, dep.Version, helmutil.ShowKindReadme, readme)
			}
			if strings.TrimSpace(values) != "" {
				_ = helmutil.WriteShowCache(repoRoot, dep.Repository, dep.Name, dep.Version, helmutil.ShowKindValues, values)
			}
			if strings.TrimSpace(schema) != "" {
				_ = helmutil.WriteShowCache(repoRoot, dep.Repository, dep.Name, dep.Version, helmutil.ShowKindSchema, schema)
			}
			switch kind {
			case InspectReadme:
				if strings.TrimSpace(readme) != "" {
					return readme, nil
				}
			case InspectValues:
				if strings.TrimSpace(values) != "" {
					return values, nil
				}
			case InspectSchema:
				if strings.TrimSpace(schema) != "" {
					return schema, nil
				}
			}
		}
	}

	// 4) Last resort: helm show (readme/values only)
	if kind == InspectSchema {
		return "", fmt.Errorf("schema not available via helm show; try relocking/pulling chart archive")
	}
	ctx3, cancel3 := context.WithTimeout(ctx, 60*time.Second)
	defer cancel3()
	if strings.HasPrefix(dep.Repository, "oci://") {
		ref, err := helmutil.OCIChartRef(dep.Repository, dep.Name)
		if err != nil {
			return "", err
		}
		if kind == InspectReadme {
			s, err := helmutil.ShowReadme(ctx3, env, ref, dep.Version)
			if err != nil {
				return "", err
			}
			_ = helmutil.WriteShowCache(repoRoot, dep.Repository, dep.Name, dep.Version, helmutil.ShowKindReadme, s)
			return s, nil
		}
		s, err := helmutil.ShowValues(ctx3, env, ref, dep.Version)
		if err != nil {
			return "", err
		}
		_ = helmutil.WriteShowCache(repoRoot, dep.Repository, dep.Name, dep.Version, helmutil.ShowKindValues, s)
		return s, nil
	}
	repoName := helmutil.RepoNameForURL(dep.Repository)
	_ = helmutil.RepoAdd(ctx3, env, repoName, dep.Repository)
	ref := repoName + "/" + dep.Name
	if kind == InspectReadme {
		s, err := helmutil.ShowReadmeBestEffort(ctx3, env, ref, dep.Version, 24*time.Hour)
		if err != nil {
			return "", err
		}
		_ = helmutil.WriteShowCache(repoRoot, dep.Repository, dep.Name, dep.Version, helmutil.ShowKindReadme, s)
		return s, nil
	}
	s, err := helmutil.ShowValuesBestEffort(ctx3, env, ref, dep.Version, 24*time.Hour)
	if err != nil {
		return "", err
	}
	_ = helmutil.WriteShowCache(repoRoot, dep.Repository, dep.Name, dep.Version, helmutil.ShowKindValues, s)
	return s, nil
}
