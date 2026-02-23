package cli

import (
	"context"
	"fmt"
	"strings"

	"helmdex/internal/artifacthub"

	"github.com/spf13/cobra"
)

func newArtifactHubCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "artifacthub",
		Short: "Artifact Hub search helpers (non-interactive)",
	}
	cmd.AddCommand(newArtifactHubSearchCmd())
	cmd.AddCommand(newArtifactHubVersionsCmd())
	return cmd
}

func newArtifactHubSearchCmd() *cobra.Command {
	var limit int
	var format string
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search for Helm charts on Artifact Hub",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := strings.TrimSpace(args[0])
			if q == "" {
				return fmt.Errorf("query is required")
			}
			f := parseFormat(format, formatJSON)
			c := artifacthub.NewClient()
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			res, err := c.SearchHelm(ctx, q, limit)
			if err != nil {
				return err
			}
			switch f {
			case formatTable, formatPlain:
				for _, p := range res {
					// one chart per line; stable, grep-friendly.
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\n", p.RepositoryKey, p.Name, p.LatestVersion, p.RepositoryURL)
				}
				return nil
			default:
				return writeJSON(cmd.OutOrStdout(), res)
			}
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 20, "Max results")
	cmd.Flags().StringVar(&format, "format", string(formatJSON), "Output format: json|table")
	return cmd
}

func newArtifactHubVersionsCmd() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "versions <repoKey> <package>",
		Short: "List available versions for a Helm chart on Artifact Hub",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoKey := strings.TrimSpace(args[0])
			pkg := strings.TrimSpace(args[1])
			if repoKey == "" || pkg == "" {
				return fmt.Errorf("repoKey and package are required")
			}
			f := parseFormat(format, formatJSON)
			c := artifacthub.NewClient()
			detail, err := c.GetHelmPackage(cmd.Context(), repoKey, pkg)
			if err != nil {
				return err
			}
			switch f {
			case formatTable, formatPlain:
				for _, v := range detail.Versions {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), v.Version)
				}
				return nil
			default:
				return writeJSON(cmd.OutOrStdout(), detail)
			}
		},
	}
	cmd.Flags().StringVar(&format, "format", string(formatJSON), "Output format: json|table")
	return cmd
}

