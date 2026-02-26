package cli

import (
	"path/filepath"

	"helmdex/internal/helmutil"
	"helmdex/internal/repo"

	"github.com/spf13/cobra"
)

func newRegistryCmd(f *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "registry",
		Short: "Manage OCI registry credentials for helmdex (stored under .helmdex/)",
	}
	cmd.AddCommand(newRegistryLoginCmd(f))
	return cmd
}

func newRegistryLoginCmd(f *rootFlags) *cobra.Command {
	var username string
	var password string
	var passwordStdin bool

	cmd := &cobra.Command{
		Use:   "login <registry>",
		Short: "Run 'helm registry login' using helmdex-isolated config (no user ~/.docker or ~/.config/helm)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, err := repo.ResolveRoot(f.RepoRoot)
			if err != nil {
				return err
			}
			// Repo-root shared registry config.
			env := helmutil.EnvForRepo(repoRoot)
			// Ensure registry config is repo-root scoped.
			env.RegistryConfig = filepath.Join(repoRoot, ".helmdex", "helm", "registry", "config.json")

			reg := args[0]
			return helmutil.RegistryLogin(cmd.Context(), env, reg, helmutil.RegistryLoginOptions{
				Username:      username,
				Password:      password,
				PasswordStdin: passwordStdin,
			})
		},
	}

	cmd.Flags().StringVar(&username, "username", "", "Username")
	cmd.Flags().StringVar(&password, "password", "", "Password (discouraged; prefer --password-stdin)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "Read password from stdin")
	return cmd
}
