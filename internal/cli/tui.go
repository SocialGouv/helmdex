package cli

import (
	"helmdex/internal/config"
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

			return tui.Run(cmd.Context(), tui.Params{
				RepoRoot:     repoRoot,
				ConfigPath:   res.Path,
				Config:       &res.Config,
				ConfigSource: res.Source,
				StartScreen:  tui.ScreenDashboard,
			})
		},
	}
	return cmd
}
