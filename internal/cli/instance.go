package cli

import (
	"fmt"
	"path/filepath"

	"helmdex/internal/config"
	"helmdex/internal/instances"
	"helmdex/internal/repo"
	"helmdex/internal/values"

	"github.com/spf13/cobra"
)

func newInstanceCmd(f *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "instance",
		Short: "Manage umbrella chart instances",
	}

	cmd.AddCommand(newInstanceCreateCmd(f))
	cmd.AddCommand(newInstanceListCmd(f))
	cmd.AddCommand(newInstanceUpdateCmd(f))
	cmd.AddCommand(newInstanceRmCmd(f))

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

			// v0.1 skeleton: generate values.yaml from whatever layers exist (at least instance)
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

			// v0.1 skeleton: dependency diffing not implemented yet. Only relock when explicitly requested.
			if relock {
				if err := instances.RelockDependencies(cmd.Context(), inst.Path); err != nil {
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

