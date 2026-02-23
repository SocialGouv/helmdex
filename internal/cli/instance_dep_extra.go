package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"helmdex/internal/catalog"
	"helmdex/internal/helmutil"
	"helmdex/internal/semverutil"
	"helmdex/internal/yamlchart"

	"github.com/spf13/cobra"
)

func newInstanceDepAddFromCatalogCmd(f *rootFlags) *cobra.Command {
	var catalogID string
	var apply bool
	var relock bool
	cmd := &cobra.Command{
		Use:   "add-from-catalog <instance>",
		Short: "Add a dependency from the local catalog cache (optionally apply)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(catalogID) == "" {
				return fmt.Errorf("--id is required")
			}
			repoRoot, _, cfg, err := resolveRepoAndConfig(f)
			if err != nil {
				return err
			}
			inst, err := resolveInstanceByName(repoRoot, cfg, args[0])
			if err != nil {
				return err
			}
			entries, err := catalog.LoadLocalCatalogEntries(repoRoot)
			if err != nil {
				return err
			}
			var e *catalog.Entry
			for i := range entries {
				if entries[i].ID == catalogID {
					e = &entries[i]
					break
				}
			}
			if e == nil {
				return fmt.Errorf("catalog entry %q not found (run 'helmdex catalog sync')", catalogID)
			}
			chartPath := filepath.Join(inst.Path, "Chart.yaml")
			c, err := yamlchart.ReadChart(chartPath)
			if err != nil {
				return err
			}
			dep := yamlchart.Dependency{Name: e.Chart.Name, Repository: e.Chart.Repo, Version: e.Version}
			if err := c.UpsertDependency(dep); err != nil {
				return err
			}
			if err := yamlchart.WriteChart(chartPath, c); err != nil {
				return err
			}
			// Materialize default sets.
			for _, setName := range e.DefaultSets {
				setName = strings.TrimSpace(setName)
				if setName == "" {
					continue
				}
				p := filepath.Join(inst.Path, fmt.Sprintf("values.set.%s.yaml", setName))
				if _, err := os.Stat(p); err == nil {
					continue
				}
				_ = os.WriteFile(p, []byte("{}\n"), 0o644)
			}
			if apply {
				// mirror instance apply pipeline
				cmd2 := NewRootCmd()
				args2 := []string{"--repo", repoRoot}
				if f.Config != "" {
					args2 = append(args2, "--config", f.Config)
				}
				args2 = append(args2, "instance", "apply", args[0])
				if relock {
					args2 = append(args2, "--relock")
				}
				cmd2.SetArgs(args2)
				cmd2.SetOut(cmd.OutOrStdout())
				cmd2.SetErr(cmd.ErrOrStderr())
				return cmd2.Execute()
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Upserted dependency %s from catalog %s in %s\n", yamlchart.DependencyID(dep), e.ID, args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&catalogID, "id", "", "Catalog entry id")
	cmd.Flags().BoolVar(&apply, "apply", false, "Apply instance after adding")
	cmd.Flags().BoolVar(&relock, "relock", false, "Force relock when applying")
	return cmd
}

func newInstanceDepSetVersionCmd(f *rootFlags) *cobra.Command {
	var version string
	var validate bool
	var apply bool
	var relock bool
	cmd := &cobra.Command{
		Use:   "set-version <instance> <depID>",
		Short: "Set exact dependency version (optionally validate and apply)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(version) == "" {
				return fmt.Errorf("--version is required")
			}
			repoRoot, _, cfg, err := resolveRepoAndConfig(f)
			if err != nil {
				return err
			}
			inst, err := resolveInstanceByName(repoRoot, cfg, args[0])
			if err != nil {
				return err
			}
			chartPath := filepath.Join(inst.Path, "Chart.yaml")
			c, err := yamlchart.ReadChart(chartPath)
			if err != nil {
				return err
			}
			id := yamlchart.DepID(strings.TrimSpace(args[1]))
			found := false
			for i := range c.Dependencies {
				if yamlchart.DependencyID(c.Dependencies[i]) == id {
					c.Dependencies[i].Version = version
					if validate && !strings.HasPrefix(c.Dependencies[i].Repository, "oci://") {
						ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
						defer cancel()
						env := helmutil.EnvForRepoURL(repoRoot, c.Dependencies[i].Repository)
						repoName := helmutil.RepoNameForURL(c.Dependencies[i].Repository)
						ref := repoName + "/" + c.Dependencies[i].Name
						if err := helmutil.RepoAdd(ctx, env, repoName, c.Dependencies[i].Repository); err != nil {
							return err
						}
						if _, err := helmutil.ShowChart(ctx, env, ref, version); err != nil {
							return fmt.Errorf("invalid version %q for %s: %w", version, id, err)
						}
					}
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("dependency %q not found", id)
			}
			if err := yamlchart.WriteChart(chartPath, c); err != nil {
				return err
			}
			if apply {
				cmd2 := NewRootCmd()
				args2 := []string{"--repo", repoRoot}
				if f.Config != "" {
					args2 = append(args2, "--config", f.Config)
				}
				args2 = append(args2, "instance", "apply", args[0])
				if relock {
					args2 = append(args2, "--relock")
				}
				cmd2.SetArgs(args2)
				cmd2.SetOut(cmd.OutOrStdout())
				cmd2.SetErr(cmd.ErrOrStderr())
				return cmd2.Execute()
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Set %s version to %s in %s\n", id, version, args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&version, "version", "", "Exact version")
	cmd.Flags().BoolVar(&validate, "validate", true, "Validate version exists (non-OCI only)")
	cmd.Flags().BoolVar(&apply, "apply", false, "Apply instance after setting version")
	cmd.Flags().BoolVar(&relock, "relock", false, "Force relock when applying")
	return cmd
}

func newInstanceDepUpgradeCmd(f *rootFlags) *cobra.Command {
	var apply bool
	var relock bool
	cmd := &cobra.Command{
		Use:   "upgrade <instance> <depID>",
		Short: "Upgrade a dependency to the latest stable SemVer (non-OCI)",
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
			chartPath := filepath.Join(inst.Path, "Chart.yaml")
			c, err := yamlchart.ReadChart(chartPath)
			if err != nil {
				return err
			}
			id := yamlchart.DepID(strings.TrimSpace(args[1]))
			var dep *yamlchart.Dependency
			for i := range c.Dependencies {
				if yamlchart.DependencyID(c.Dependencies[i]) == id {
					dep = &c.Dependencies[i]
					break
				}
			}
			if dep == nil {
				return fmt.Errorf("dependency %q not found", id)
			}
			if strings.HasPrefix(dep.Repository, "oci://") {
				return fmt.Errorf("cannot auto-upgrade OCI dependency %s; set exact version", id)
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 75*time.Second)
			defer cancel()
			vs, err := helmutil.RepoChartVersions(ctx, repoRoot, dep.Repository, dep.Name, 24*time.Hour)
			if err != nil {
				return err
			}
			best, ok := semverutil.BestStable(vs)
			if !ok {
				return fmt.Errorf("no stable SemVer versions found for %s", id)
			}
			if strings.TrimSpace(best) == strings.TrimSpace(dep.Version) {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "No change (already at %s)\n", best)
				return nil
			}
			dep.Version = best
			if err := yamlchart.WriteChart(chartPath, c); err != nil {
				return err
			}
			if apply {
				cmd2 := NewRootCmd()
				args2 := []string{"--repo", repoRoot}
				if f.Config != "" {
					args2 = append(args2, "--config", f.Config)
				}
				args2 = append(args2, "instance", "apply", args[0])
				if relock {
					args2 = append(args2, "--relock")
				}
				cmd2.SetArgs(args2)
				cmd2.SetOut(cmd.OutOrStdout())
				cmd2.SetErr(cmd.ErrOrStderr())
				return cmd2.Execute()
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Upgraded %s to %s in %s\n", id, best, args[0])
			return nil
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "Apply instance after upgrading")
	cmd.Flags().BoolVar(&relock, "relock", false, "Force relock when applying")
	return cmd
}

func newInstanceDepVersionsCmd(f *rootFlags) *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "versions <instance> <depID>",
		Short: "List versions for a dependency (non-OCI) using isolated helm",
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
			c, err := yamlchart.ReadChart(filepath.Join(inst.Path, "Chart.yaml"))
			if err != nil {
				return err
			}
			id := yamlchart.DepID(strings.TrimSpace(args[1]))
			var dep *yamlchart.Dependency
			for i := range c.Dependencies {
				if yamlchart.DependencyID(c.Dependencies[i]) == id {
					dep = &c.Dependencies[i]
					break
				}
			}
			if dep == nil {
				return fmt.Errorf("dependency %q not found", id)
			}
			if strings.HasPrefix(dep.Repository, "oci://") {
				return fmt.Errorf("OCI dependency %s has no version listing", id)
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 60*time.Second)
			defer cancel()
			vs, err := helmutil.RepoChartVersions(ctx, repoRoot, dep.Repository, dep.Name, 24*time.Hour)
			if err != nil {
				return err
			}
			sort.Strings(vs)
			ff := parseFormat(format, formatJSON)
			if ff == formatTable {
				for _, v := range vs {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), v)
				}
				return nil
			}
			return writeJSON(cmd.OutOrStdout(), vs)
		},
	}
	cmd.Flags().StringVar(&format, "format", string(formatJSON), "Output format: json|table")
	return cmd
}
