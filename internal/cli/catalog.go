package cli

import (
	"fmt"

	"helmdex/internal/catalog"

	"github.com/spf13/cobra"
)

func newCatalogCmd(f *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "catalog",
		Short: "Catalog operations (predefined catalog sync)",
	}

	cmd.AddCommand(newCatalogSyncCmd(f))
	cmd.AddCommand(newCatalogListCmd(f))
	cmd.AddCommand(newCatalogGetCmd(f))
	return cmd
}

func newCatalogSyncCmd(f *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync remote preset/catalog sources into .helmdex cache",
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, _, cfg, err := resolveRepoAndConfig(f)
			if err != nil {
				return err
			}

			s := catalog.NewSyncer(repoRoot)
			res, err := s.Sync(cmd.Context(), cfg)
			if err != nil {
				return err
			}

			for _, r := range res {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Synced source %s at %s\n", r.SourceName, r.ResolvedCommit)
			}
			return nil
		},
	}

	return cmd
}
