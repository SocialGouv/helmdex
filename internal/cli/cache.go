package cli

import (
	"fmt"
	"path/filepath"
	"os"

	"helmdex/internal/helmutil"
	"helmdex/internal/repo"

	"github.com/spf13/cobra"
)

func newCacheCmd(f *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cache",
		Short: "Manage local helmdex caches",
	}
	cmd.AddCommand(newCacheClearCmd(f))
	return cmd
}

func newCacheClearCmd(f *rootFlags) *cobra.Command {
	var clearHelm bool
	cmd := &cobra.Command{
		Use:   "clear",
		Short: "Clear helmdex caches (helm show cache by default)",
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, err := repo.ResolveRoot(f.RepoRoot)
			if err != nil {
				return err
			}
			if err := helmutil.ClearShowCache(repoRoot); err != nil {
				return err
			}
			if clearHelm {
				// Wipe isolated helm env(s) too.
				p := filepath.Join(repoRoot, ".helmdex", "helm")
				if err := removeAll(p); err != nil {
					return err
				}
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Cache cleared")
			return nil
		},
	}
	cmd.Flags().BoolVar(&clearHelm, "helm", false, "Also clear isolated Helm env caches under .helmdex/helm")
	return cmd
}

// removeAll exists to keep file ops in one place if we later need safeguards.
func removeAll(p string) error {
	return os.RemoveAll(p)
}
