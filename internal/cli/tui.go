package cli

import (
	"path/filepath"

	"helmdex/internal/config"
	"helmdex/internal/repo"
	"helmdex/internal/tui"

	"github.com/spf13/cobra"
)

func newTUICmd(f *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Launch the interactive dashboard",
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
				RepoRoot:    repoRoot,
				ConfigPath:  cfgPath,
				Config:      cfg,
				StartScreen: tui.ScreenDashboard,
			})
		},
	}
	return cmd
}

