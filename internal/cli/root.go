package cli

import (
	"os"

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
		Short: "helmdex scaffolds and maintains GitOps-friendly Helm umbrella chart instances",
		Long: "helmdex is a TUI-first organizer for Helm umbrella chart instances (no template rendering, no deploy).",
		RunE: func(cmd *cobra.Command, args []string) error {
			// v0.1 skeleton: show help by default
			return cmd.Help()
		},
	}

	cmd.PersistentFlags().StringVar(&f.RepoRoot, "repo", "", "Repo root (defaults to auto-detect from current directory)")
	cmd.PersistentFlags().StringVar(&f.Config, "config", "", "Path to helmdex config (defaults to <repoRoot>/helmdex.yaml)")

	cmd.AddCommand(newInitCmd(&f))
	cmd.AddCommand(newCatalogCmd(&f))
	cmd.AddCommand(newInstanceCmd(&f))

	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	return cmd
}
