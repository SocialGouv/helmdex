package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"helmdex/internal/config"
	"helmdex/internal/repo"

	"github.com/spf13/cobra"
)

func newInitCmd(f *rootFlags) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a helmdex repo (create helmdex.yaml)",
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, err := repo.ResolveRoot(f.RepoRoot)
			if err != nil {
				return err
			}

			cfgPath := f.Config
			if cfgPath == "" {
				cfgPath = filepath.Join(repoRoot, "helmdex.yaml")
			}

			if _, err := os.Stat(cfgPath); err == nil && !force {
				return fmt.Errorf("config already exists at %s (use --force to overwrite)", cfgPath)
			}

			cfg := config.DefaultConfig()
			if err := config.WriteFile(cfgPath, cfg); err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Wrote %s\n", cfgPath)
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing helmdex.yaml")
	return cmd
}

