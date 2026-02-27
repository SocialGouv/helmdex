package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"helmdex/internal/config"
	"helmdex/internal/instances"
	"helmdex/internal/presets"
	"helmdex/internal/repo"
	"helmdex/internal/values"
	"helmdex/internal/yamlchart"

	"github.com/spf13/cobra"
)

func newInstanceCmd(f *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "instance",
		Short: "Manage umbrella chart instances",
	}

	cmd.AddCommand(newInstanceCreateCmd(f))
	cmd.AddCommand(newInstanceListCmd(f))
	cmd.AddCommand(newInstanceDepCmd(f))
	cmd.AddCommand(newInstanceUpdateCmd(f))
	cmd.AddCommand(newInstanceApplyCmd(f))
	cmd.AddCommand(newInstanceRmCmd(f))
	cmd.AddCommand(newInstanceValuesCmd(f))
	cmd.AddCommand(newInstancePresetsCmd(f))

	return cmd
}

func newInstanceDepCmd(f *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dep",
		Short: "Manage instance dependencies in Chart.yaml",
	}
	cmd.AddCommand(newInstanceDepAddCmd(f))
	cmd.AddCommand(newInstanceDepAddFromCatalogCmd(f))
	cmd.AddCommand(newInstanceDepRmCmd(f))
	cmd.AddCommand(newInstanceDepListCmd(f))
	cmd.AddCommand(newInstanceDepDetachCmd(f))
	cmd.AddCommand(newInstanceDepSyncPresetsCmd(f))
	cmd.AddCommand(newInstanceDepSetVersionCmd(f))
	cmd.AddCommand(newInstanceDepUpgradeCmd(f))
	cmd.AddCommand(newInstanceDepVersionsCmd(f))
	cmd.AddCommand(newInstanceDepValuesCmd(f))
	cmd.AddCommand(newInstanceDepInspectCmd(f))
	return cmd
}

func newInstanceDepAddCmd(f *rootFlags) *cobra.Command {
	var repoURL string
	var name string
	var version string
	var alias string
	var sets []string
	cmd := &cobra.Command{
		Use:   "add <instance>",
		Short: "Add or update a dependency in Chart.yaml",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, err := repo.ResolveRoot(f.RepoRoot)
			if err != nil {
				return err
			}
			cfgPath := f.Config
			if cfgPath == "" {
				cfgPath = filepath.Join(repoRoot, "helmdex.yaml")
			}
			cfg, err := config.LoadFile(cfgPath)
			if err != nil {
				return err
			}
			inst, err := instances.Get(repoRoot, cfg.Repo.AppsDir, args[0])
			if err != nil {
				return err
			}
			chartPath := filepath.Join(inst.Path, "Chart.yaml")
			c, err := yamlchart.ReadChart(chartPath)
			if err != nil {
				return err
			}
			dep := yamlchart.Dependency{Name: name, Repository: repoURL, Version: version, Alias: alias}
			if err := c.UpsertDependency(dep); err != nil {
				return err
			}
			if err := yamlchart.WriteChart(chartPath, c); err != nil {
				return err
			}
			// Materialize selected set files (selection is by presence of values.set.*.yaml).
			for _, setName := range sets {
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
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Upserted dependency %s in %s\n", yamlchart.DependencyID(dep), args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&repoURL, "repo", "", "Repository URL (https://... or oci://...)")
	cmd.Flags().StringVar(&name, "name", "", "Chart name")
	cmd.Flags().StringVar(&version, "version", "", "Exact version")
	cmd.Flags().StringVar(&alias, "alias", "", "Alias (optional)")
	cmd.Flags().StringSliceVar(&sets, "set", nil, "Enable a values set by creating values.set.<set>.yaml (repeatable)")
	_ = cmd.MarkFlagRequired("repo")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("version")
	return cmd
}

func newInstanceDepRmCmd(f *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rm <instance> <depID>",
		Short: "Remove a dependency from Chart.yaml by its id (alias or name)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, err := repo.ResolveRoot(f.RepoRoot)
			if err != nil {
				return err
			}
			cfgPath := f.Config
			if cfgPath == "" {
				cfgPath = filepath.Join(repoRoot, "helmdex.yaml")
			}
			cfg, err := config.LoadFile(cfgPath)
			if err != nil {
				return err
			}
			inst, err := instances.Get(repoRoot, cfg.Repo.AppsDir, args[0])
			if err != nil {
				return err
			}
			chartPath := filepath.Join(inst.Path, "Chart.yaml")
			c, err := yamlchart.ReadChart(chartPath)
			if err != nil {
				return err
			}
			id := yamlchart.DepID(args[1])
			if ok := c.RemoveDependencyByID(id); !ok {
				return fmt.Errorf("dependency %q not found", id)
			}
			if err := yamlchart.WriteChart(chartPath, c); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Removed dependency %s from %s\n", id, args[0])
			return nil
		},
	}
	return cmd
}

func newInstanceDepListCmd(f *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list <instance>",
		Short: "List dependencies from Chart.yaml",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, err := repo.ResolveRoot(f.RepoRoot)
			if err != nil {
				return err
			}
			cfgPath := f.Config
			if cfgPath == "" {
				cfgPath = filepath.Join(repoRoot, "helmdex.yaml")
			}
			cfg, err := config.LoadFile(cfgPath)
			if err != nil {
				return err
			}
			inst, err := instances.Get(repoRoot, cfg.Repo.AppsDir, args[0])
			if err != nil {
				return err
			}
			c, err := yamlchart.ReadChart(filepath.Join(inst.Path, "Chart.yaml"))
			if err != nil {
				return err
			}
			for _, d := range c.Dependencies {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\n", yamlchart.DependencyID(d), d.Name, d.Version, d.Repository)
			}
			return nil
		},
	}
	return cmd
}

func newInstanceApplyCmd(f *rootFlags) *cobra.Command {
	var relock bool
	cmd := &cobra.Command{
		Use:   "apply <name>",
		Short: "Apply instance changes (optional relock, import presets, regenerate values.yaml)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, err := repo.ResolveRoot(f.RepoRoot)
			if err != nil {
				return err
			}
			cfgPath := f.Config
			if cfgPath == "" {
				cfgPath = filepath.Join(repoRoot, "helmdex.yaml")
			}
			cfg, err := config.LoadFile(cfgPath)
			if err != nil {
				return err
			}

			name := args[0]
			inst, err := instances.Get(repoRoot, cfg.Repo.AppsDir, name)
			if err != nil {
				return err
			}

			// Relock.
			if relock {
				if err := instances.RelockDependencies(cmd.Context(), repoRoot, inst.Path); err != nil {
					return err
				}
			} else {
				if _, err := instances.RelockIfDepsChanged(cmd.Context(), repoRoot, inst.Path); err != nil {
					return err
				}
			}

			// Import presets (default/platform + any selected sets).
			c, err := yamlchart.ReadChart(filepath.Join(inst.Path, "Chart.yaml"))
			if err != nil {
				return err
			}
			_, err = presets.Import(presets.ImportParams{RepoRoot: repoRoot, InstancePath: inst.Path, Config: cfg, Dependencies: c.Dependencies})
			if err != nil {
				return err
			}

			if err := values.GenerateMergedValues(inst.Path); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Applied instance %s\n", name)
			return nil
		},
	}
	cmd.Flags().BoolVar(&relock, "relock", false, "Force re-running helm dependency update/build")
	return cmd
}

func newInstanceCreateCmd(f *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new umbrella chart instance",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, err := repo.ResolveRoot(f.RepoRoot)
			if err != nil {
				return err
			}
			cfgPath := f.Config
			if cfgPath == "" {
				cfgPath = filepath.Join(repoRoot, "helmdex.yaml")
			}
			cfg, err := config.LoadFile(cfgPath)
			if err != nil {
				return err
			}

			name := args[0]
			inst, err := instances.Create(repoRoot, cfg.Repo.AppsDir, name)
			if err != nil {
				return err
			}

			// Generate values.yaml from available layers (at least values.instance.yaml).
			if err := values.GenerateMergedValues(inst.Path); err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Created instance %s at %s\n", name, inst.Path)
			return nil
		},
	}
	return cmd
}

func newInstanceListCmd(f *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List instances",
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, err := repo.ResolveRoot(f.RepoRoot)
			if err != nil {
				return err
			}
			cfgPath := f.Config
			if cfgPath == "" {
				cfgPath = filepath.Join(repoRoot, "helmdex.yaml")
			}
			cfg, err := config.LoadFile(cfgPath)
			if err != nil {
				return err
			}

			list, err := instances.List(repoRoot, cfg.Repo.AppsDir)
			if err != nil {
				return err
			}
			for _, inst := range list {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), inst.Name)
			}
			return nil
		},
	}
	return cmd
}

func newInstanceUpdateCmd(f *rootFlags) *cobra.Command {
	var relock bool

	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Update an instance (regen generated values and optionally relock deps)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, err := repo.ResolveRoot(f.RepoRoot)
			if err != nil {
				return err
			}
			cfgPath := f.Config
			if cfgPath == "" {
				cfgPath = filepath.Join(repoRoot, "helmdex.yaml")
			}
			cfg, err := config.LoadFile(cfgPath)
			if err != nil {
				return err
			}

			name := args[0]
			inst, err := instances.Get(repoRoot, cfg.Repo.AppsDir, name)
			if err != nil {
				return err
			}

			// Default: only re-lock when Chart.yaml differs from Chart.lock.
			// --relock forces a relock.
			if relock {
				if err := instances.RelockDependencies(cmd.Context(), repoRoot, inst.Path); err != nil {
					return err
				}
			} else {
				if _, err := instances.RelockIfDepsChanged(cmd.Context(), repoRoot, inst.Path); err != nil {
					return err
				}
			}

			if err := values.GenerateMergedValues(inst.Path); err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Updated instance %s\n", name)
			return nil
		},
	}

	cmd.Flags().BoolVar(&relock, "relock", false, "Force re-running helm dependency update/build")
	return cmd
}

func newInstanceRmCmd(f *rootFlags) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "rm <name>",
		Short: "Remove an instance",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				return fmt.Errorf("refusing to delete without --yes (v0.1 safety)")
			}
			repoRoot, err := repo.ResolveRoot(f.RepoRoot)
			if err != nil {
				return err
			}
			cfgPath := f.Config
			if cfgPath == "" {
				cfgPath = filepath.Join(repoRoot, "helmdex.yaml")
			}
			cfg, err := config.LoadFile(cfgPath)
			if err != nil {
				return err
			}

			name := args[0]
			if err := instances.Remove(repoRoot, cfg.Repo.AppsDir, name); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Removed instance %s\n", name)
			return nil
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "Confirm deletion")
	return cmd
}
