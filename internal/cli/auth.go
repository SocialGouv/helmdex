package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"helmdex/internal/authsvc"
	"helmdex/internal/creds"
	"helmdex/internal/repo"

	"github.com/spf13/cobra"
)

func newAuthCmd(f *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage credentials for private chart sources (OCI registries, Helm repos, git remotes)",
	}
	cmd.AddCommand(newAuthListCmd())
	cmd.AddCommand(newAuthDetectCmd())
	cmd.AddCommand(newAuthLoginCmd(f))
	cmd.AddCommand(newAuthRemoveCmd(f))
	return cmd
}

func newAuthListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List stored credentials (never shows secrets)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			list, err := creds.List()
			if err != nil {
				return err
			}
			if len(list) == 0 {
				cmd.Println("no credentials stored")
				return nil
			}
			for _, c := range list {
				extra := ""
				if c.SSHKeyPath != "" {
					extra = "  ssh-key=" + c.SSHKeyPath
				}
				cmd.Printf("%-12s %-30s user=%s source=%s%s\n", c.Kind, c.Host, c.Username, c.Source, extra)
			}
			return nil
		},
	}
}

func newAuthDetectCmd() *cobra.Command {
	var kind string
	cmd := &cobra.Command{
		Use:   "detect <host>",
		Short: "Probe local config (docker/podman/helm/git/gh/glab) for credentials usable against a host",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			k := creds.Kind(kind)
			if !creds.ValidKind(k) {
				return fmt.Errorf("--kind must be one of oci, git, helm-repo")
			}
			cands := creds.Detect(cmd.Context(), args[0])
			if len(cands) == 0 {
				cmd.Println("no local credentials found")
			}
			for _, c := range cands {
				cmd.Printf("%-16s host=%s user=%s (%s)\n", c.Source, c.Host, c.Username, c.Label)
			}
			page := creds.TokenPageFor(args[0], k)
			cmd.Printf("\ntoken creation page (%s): %s\n", page.Provider, page.URL)
			cmd.Printf("sign in with: helmdex auth login %s --kind %s [--use <source>]\n", args[0], k)
			return nil
		},
	}
	cmd.Flags().StringVar(&kind, "kind", string(creds.KindOCI), "Credential kind: oci, git or helm-repo")
	return cmd
}

func newAuthLoginCmd(f *rootFlags) *cobra.Command {
	var kind, username, token, use, sshKey, url string
	var passwordStdin, openTokenPage bool

	cmd := &cobra.Command{
		Use:   "login <host>",
		Short: "Store a credential for a host, verifying it against the remote when possible",
		Long: `Store a credential for a host.

The secret can come from:
  --use <source>       a detected local source (see 'helmdex auth detect')
  --token <secret>     a PAT/password given inline (discouraged; prefer stdin)
  --password-stdin     the secret read from stdin
  --ssh-key <path>     a private key path for git-over-SSH remotes

With --open-token-page the provider's token-creation page (pre-filled name
and scopes for GitLab/GitHub) opens in the browser first.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			host := args[0]
			k := creds.Kind(kind)
			if !creds.ValidKind(k) {
				return fmt.Errorf("--kind must be one of oci, git, helm-repo")
			}

			if openTokenPage {
				page := creds.TokenPageFor(host, k)
				cmd.Printf("opening %s\n", page.URL)
				if err := authsvc.OpenBrowser(page.URL); err != nil {
					cmd.Printf("could not open a browser (%v) — open it manually\n", err)
				}
			}

			req := authsvc.LoginRequest{Host: host, Kind: k, URL: url, Username: username}
			switch {
			case use != "":
				req.Method = authsvc.MethodDetected
				req.Source = use
				req.SourceHost = host
			case sshKey != "":
				req.Method = authsvc.MethodManual
				req.SSHKeyPath = sshKey
			case passwordStdin:
				secret, err := readSecretStdin()
				if err != nil {
					return err
				}
				req.Method = authsvc.MethodManual
				req.Secret = secret
			case token != "":
				req.Method = authsvc.MethodManual
				req.Secret = token
			default:
				return fmt.Errorf("provide the secret via --use, --token, --password-stdin or --ssh-key")
			}

			// Login verification needs a repo root only for OCI (shared
			// registry config); missing repo is fine for git/helm-repo.
			repoRoot, err := repo.ResolveRoot(f.RepoRoot)
			if err != nil {
				repoRoot = ""
			}
			res, err := authsvc.Login(cmd.Context(), repoRoot, req)
			if err != nil {
				return err
			}
			state := "verified"
			if !res.Verified {
				state = res.Message
			}
			cmd.Printf("stored %s credential for %s (user=%s, %s)\n", res.Kind, res.Host, res.Username, state)
			return nil
		},
	}
	cmd.Flags().StringVar(&kind, "kind", string(creds.KindOCI), "Credential kind: oci, git or helm-repo")
	cmd.Flags().StringVar(&username, "username", "", "Username (defaults to 'oauth2', accepted by forges with a PAT)")
	cmd.Flags().StringVar(&token, "token", "", "Token/password (discouraged; prefer --password-stdin)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "Read the token/password from stdin")
	cmd.Flags().StringVar(&use, "use", "", "Use a detected local source: docker-config, podman, helm-registry, git-credential, gh-cli, glab-cli")
	cmd.Flags().StringVar(&sshKey, "ssh-key", "", "Private key path for git-over-SSH (stored by path, never read)")
	cmd.Flags().StringVar(&url, "url", "", "Repository/remote URL to verify the credential against")
	cmd.Flags().BoolVar(&openTokenPage, "open-token-page", false, "Open the provider's token-creation page in the browser first")
	return cmd
}

func newAuthRemoveCmd(f *rootFlags) *cobra.Command {
	var kind string
	cmd := &cobra.Command{
		Use:   "remove <host>",
		Short: "Remove stored credentials for a host",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			k := creds.Kind(kind)
			if kind != "" && !creds.ValidKind(k) {
				return fmt.Errorf("--kind must be one of oci, git, helm-repo (or omitted for all)")
			}
			repoRoot, err := repo.ResolveRoot(f.RepoRoot)
			if err != nil {
				repoRoot = ""
			}
			removed, err := authsvc.Logout(repoRoot, args[0], k)
			if err != nil {
				return err
			}
			if !removed {
				return fmt.Errorf("no stored credential for %q", args[0])
			}
			cmd.Printf("removed credentials for %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "", "Limit removal to one kind: oci, git or helm-repo")
	return cmd
}

func readSecretStdin() (string, error) {
	r := bufio.NewReader(os.Stdin)
	b, err := r.ReadString('\n')
	if err != nil && b == "" {
		return "", fmt.Errorf("read secret from stdin: %w", err)
	}
	secret := strings.TrimRight(b, "\r\n")
	if secret == "" {
		return "", fmt.Errorf("empty secret on stdin")
	}
	return secret, nil
}
