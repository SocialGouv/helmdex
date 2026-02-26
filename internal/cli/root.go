package cli

import (
	"os"
	"path/filepath"

	"helmdex/internal/appinfo"
	"helmdex/internal/config"
	"helmdex/internal/repo"
	"helmdex/internal/tui"

	"github.com/spf13/cobra"
)

type rootFlags struct {
	RepoRoot string
	Config   string
}

func NewRootCmd() *cobra.Command {
	var f rootFlags

	cmd := &cobra.Command{
		Use:   "helmdex",
		Short: appinfo.Short,
		Long:  appinfo.Long,
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, err := repo.ResolveRoot(f.RepoRoot)
			if err != nil {
				return err
			}
			cfgPath := f.Config
			if cfgPath == "" {
				cfgPath = filepath.Join(repoRoot, "helmdex.yaml")
			}

			var cfg *config.Config
			loaded, err := config.LoadFile(cfgPath)
			if err == nil {
				cfg = &loaded
			}

			return tui.Run(cmd.Context(), tui.Params{
				RepoRoot: repoRoot,
				ConfigPath: cfgPath,
				Config: cfg,
			})
		},
	}

	cmd.PersistentFlags().StringVar(&f.RepoRoot, "repo", "", "Repo root (defaults to auto-detect from current directory)")
	cmd.PersistentFlags().StringVar(&f.Config, "config", "", "Path to helmdex config (defaults to <repoRoot>/helmdex.yaml)")

	cmd.AddCommand(newInitCmd(&f))
	cmd.AddCommand(newCatalogCmd(&f))
	cmd.AddCommand(newCacheCmd(&f))
	cmd.AddCommand(newRegistryCmd(&f))
	cmd.AddCommand(newInstanceCmd(&f))
	cmd.AddCommand(newArtifactHubCmd())
	cmd.AddCommand(newTUICmd(&f))

	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	return cmd
}
