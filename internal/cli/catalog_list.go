package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"helmdex/internal/catalog"
	"helmdex/internal/repo"

	"github.com/spf13/cobra"
)

func newCatalogListCmd(f *rootFlags) *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List locally cached catalog entries (from .helmdex/catalog)",
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, err := repo.ResolveRoot(f.RepoRoot)
			if err != nil {
				return err
			}
			entries, err := catalog.LoadLocalCatalogEntries(repoRoot)
			if err != nil {
				return err
			}
			ff := parseFormat(format, formatJSON)
			switch ff {
			case formatTable, formatPlain:
				for _, e := range entries {
					sets := strings.Join(e.DefaultSets, ",")
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\t%s\n", e.ID, e.Chart.Name, e.Version, e.Chart.Repo, sets)
				}
				return nil
			default:
				return writeJSON(cmd.OutOrStdout(), entries)
			}
		},
	}
	cmd.Flags().StringVar(&format, "format", string(formatJSON), "Output format: json|table")
	return cmd
}

func newCatalogGetCmd(f *rootFlags) *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get one locally cached catalog entry by id",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, err := repo.ResolveRoot(f.RepoRoot)
			if err != nil {
				return err
			}
			entries, err := catalog.LoadLocalCatalogEntries(repoRoot)
			if err != nil {
				return err
			}
			id := strings.TrimSpace(args[0])
			for _, e := range entries {
				if e.ID == id {
					ff := parseFormat(format, formatJSON)
					switch ff {
					case formatTable, formatPlain:
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\n", e.ID, e.Chart.Name, e.Version, e.Chart.Repo)
						if len(e.DefaultSets) > 0 {
							_, _ = fmt.Fprintf(cmd.OutOrStdout(), "defaultSets:\t%s\n", strings.Join(e.DefaultSets, ","))
						}
						if strings.TrimSpace(e.Description) != "" {
							_, _ = fmt.Fprintf(cmd.OutOrStdout(), "description:\t%s\n", strings.TrimSpace(e.Description))
						}
						return nil
					default:
						return writeJSON(cmd.OutOrStdout(), e)
					}
				}
			}
			catDir := filepath.Join(repoRoot, ".helmdex", "catalog")
			return fmt.Errorf("catalog entry %q not found (cache dir: %s; run 'helmdex catalog sync')", id, catDir)
		},
	}
	cmd.Flags().StringVar(&format, "format", string(formatJSON), "Output format: json|table")
	return cmd
}

