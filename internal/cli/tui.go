package cli

import (
	"helmdex/internal/config"
	"helmdex/internal/instances"
	"helmdex/internal/repo"
	"helmdex/internal/tui"

	"github.com/spf13/cobra"
)

func newTUICmd(f *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "tui",
		Short:  "Launch the interactive dashboard",
		Hidden: true, // running `helmdex` with no args opens the TUI directly
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, err := repo.ResolveRoot(f.RepoRoot)
			if err != nil {
				return err
			}
			res, err := config.Resolve(repoRoot, f.Config)
			if err != nil {
				return err
			}
			cfg := instances.ApplyLayout(repoRoot, res)

			return tui.Run(cmd.Context(), tui.Params{
				RepoRoot:     repoRoot,
				ConfigPath:   res.Path,
				Config:       &cfg,
				ConfigSource: res.Source,
				StartScreen:  tui.ScreenDashboard,
			})
		},
	}
	return cmd
}
