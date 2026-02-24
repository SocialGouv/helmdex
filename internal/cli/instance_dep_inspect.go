package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"helmdex/internal/helmutil"
	"helmdex/internal/yamlchart"

	"github.com/spf13/cobra"
)

func newInstanceDepInspectCmd(f *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect",
		Short: "Inspect dependency artifacts (readme/default values/schema)",
	}
	cmd.AddCommand(newInstanceDepInspectReadmeCmd(f))
	cmd.AddCommand(newInstanceDepInspectValuesCmd(f))
	cmd.AddCommand(newInstanceDepInspectSchemaCmd(f))
	return cmd
}

func depByIDOrErr(chart yamlchart.Chart, id string) (yamlchart.Dependency, error) {
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

type depInspectKind string

const (
	kindReadme depInspectKind = "readme"
	kindValues depInspectKind = "values"
	kindSchema depInspectKind = "schema"
)

func loadDepInspectContent(ctx context.Context, repoRoot string, instPath string, dep yamlchart.Dependency, kind depInspectKind) (string, error) {
	// 0) Vendored chart files
	switch kind {
	case kindReadme:
		if s, ok, err := readVendoredChartFile(instPath, dep, "README.md"); err != nil {
			return "", err
		} else if ok {
			_ = helmutil.WriteShowCache(repoRoot, dep.Repository, dep.Name, dep.Version, helmutil.ShowKindReadme, s)
			return s, nil
		}
	case kindValues:
		if s, ok, err := readVendoredChartFile(instPath, dep, "values.yaml"); err != nil {
			return "", err
		} else if ok {
			_ = helmutil.WriteShowCache(repoRoot, dep.Repository, dep.Name, dep.Version, helmutil.ShowKindValues, s)
			return s, nil
		}
	case kindSchema:
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
			case kindReadme:
				if strings.TrimSpace(readme) != "" {
					return readme, nil
				}
			case kindValues:
				if strings.TrimSpace(values) != "" {
					return values, nil
				}
			case kindSchema:
				if strings.TrimSpace(schema) != "" {
					return schema, nil
				}
			}
		}
	}

	// 2) helmdex show cache
	switch kind {
	case kindReadme:
		if s, ok, err := helmutil.ReadShowCache(repoRoot, dep.Repository, dep.Name, dep.Version, helmutil.ShowKindReadme); err != nil {
			return "", err
		} else if ok {
			return s, nil
		}
	case kindValues:
		if s, ok, err := helmutil.ReadShowCache(repoRoot, dep.Repository, dep.Name, dep.Version, helmutil.ShowKindValues); err != nil {
			return "", err
		} else if ok {
			return s, nil
		}
	case kindSchema:
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
			case kindReadme:
				if strings.TrimSpace(readme) != "" {
					return readme, nil
				}
			case kindValues:
				if strings.TrimSpace(values) != "" {
					return values, nil
				}
			case kindSchema:
				if strings.TrimSpace(schema) != "" {
					return schema, nil
				}
			}
		}
	}

	// 4) Last resort: helm show (readme/values only)
	if kind == kindSchema {
		return "", fmt.Errorf("schema not available via helm show; try relocking/pulling chart archive")
	}
	ctx3, cancel3 := context.WithTimeout(ctx, 60*time.Second)
	defer cancel3()
	if strings.HasPrefix(dep.Repository, "oci://") {
		ref, err := helmutil.OCIChartRef(dep.Repository, dep.Name)
		if err != nil {
			return "", err
		}
		if kind == kindReadme {
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
	if kind == kindReadme {
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

func newInstanceDepInspectReadmeCmd(f *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "readme <instance> <depID>",
		Short: "Show dependency README.md (best effort, cached)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, _, cfg, err := resolveRepoAndConfig(f)
			if err != nil {
				return err
			}
			inst, err := resolveInstanceByName(repoRoot, cfg, args[0])
			if err != nil {
				return err
			}
			chart, err := readInstanceChart(inst)
			if err != nil {
				return err
			}
			dep, err := depByIDOrErr(chart, args[1])
			if err != nil {
				return err
			}
			s, err := loadDepInspectContent(cmd.Context(), repoRoot, inst.Path, dep, kindReadme)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprint(cmd.OutOrStdout(), s)
			return nil
		},
	}
	return cmd
}

func newInstanceDepInspectValuesCmd(f *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "values <instance> <depID>",
		Short: "Show dependency default values.yaml (best effort, cached)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, _, cfg, err := resolveRepoAndConfig(f)
			if err != nil {
				return err
			}
			inst, err := resolveInstanceByName(repoRoot, cfg, args[0])
			if err != nil {
				return err
			}
			chart, err := readInstanceChart(inst)
			if err != nil {
				return err
			}
			dep, err := depByIDOrErr(chart, args[1])
			if err != nil {
				return err
			}
			s, err := loadDepInspectContent(cmd.Context(), repoRoot, inst.Path, dep, kindValues)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprint(cmd.OutOrStdout(), s)
			return nil
		},
	}
	return cmd
}

func newInstanceDepInspectSchemaCmd(f *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schema <instance> <depID>",
		Short: "Show dependency values.schema.json (best effort, cached)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, _, cfg, err := resolveRepoAndConfig(f)
			if err != nil {
				return err
			}
			inst, err := resolveInstanceByName(repoRoot, cfg, args[0])
			if err != nil {
				return err
			}
			chart, err := readInstanceChart(inst)
			if err != nil {
				return err
			}
			dep, err := depByIDOrErr(chart, args[1])
			if err != nil {
				return err
			}
			s, err := loadDepInspectContent(cmd.Context(), repoRoot, inst.Path, dep, kindSchema)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprint(cmd.OutOrStdout(), s)
			return nil
		},
	}
	return cmd
}
