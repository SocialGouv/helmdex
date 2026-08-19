package gitutil

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"helmdex/internal/creds"
)

type CloneOrUpdateParams struct {
	URL    string
	Ref    string
	Commit string

	DestDir string
}

type CloneOrUpdateResult struct {
	ResolvedCommit string
}

func CloneOrUpdate(ctx context.Context, p CloneOrUpdateParams) (CloneOrUpdateResult, error) {
	if p.URL == "" {
		return CloneOrUpdateResult{}, fmt.Errorf("git url is required")
	}
	if p.DestDir == "" {
		return CloneOrUpdateResult{}, fmt.Errorf("dest dir is required")
	}

	auth := authFor(p.URL)
	if _, err := os.Stat(filepath.Join(p.DestDir, ".git")); err != nil {
		if err := run(ctx, "", auth, "clone", "--", p.URL, p.DestDir); err != nil {
			return CloneOrUpdateResult{}, err
		}
	} else {
		if err := run(ctx, p.DestDir, auth, "fetch", "--all", "--tags", "--prune"); err != nil {
			return CloneOrUpdateResult{}, err
		}
	}

	// Prefer fixed commit pin if provided.
	if p.Commit != "" {
		if err := run(ctx, p.DestDir, auth, "checkout", "--detach", p.Commit); err != nil {
			return CloneOrUpdateResult{}, err
		}
		return CloneOrUpdateResult{ResolvedCommit: p.Commit}, nil
	}

	if p.Ref == "" {
		p.Ref = "HEAD"
	}
	if err := run(ctx, p.DestDir, auth, "checkout", "--detach", p.Ref); err != nil {
		return CloneOrUpdateResult{}, err
	}

	sha, err := output(ctx, p.DestDir, auth, "rev-parse", "HEAD")
	if err != nil {
		return CloneOrUpdateResult{}, err
	}
	return CloneOrUpdateResult{ResolvedCommit: strings.TrimSpace(sha)}, nil
}

// VerifyAccess checks that the remote is reachable with the currently stored
// credentials.
func VerifyAccess(ctx context.Context, url string) error {
	_, err := output(ctx, "", authFor(url), "ls-remote", "--", url, "HEAD")
	return err
}

// VerifyAccessWithCredential checks that the remote is reachable with an
// explicit credential (used to validate a login before persisting it).
func VerifyAccessWithCredential(ctx context.Context, url string, c creds.Credential) error {
	_, err := output(ctx, "", authForCred(url, c), "ls-remote", "--", url, "HEAD")
	return err
}

// gitAuth carries per-invocation credential wiring for git subprocesses.
type gitAuth struct {
	// configArgs are `-c key=value` arguments placed before the subcommand.
	configArgs []string
	// env entries appended to the process environment.
	env []string
}

// authFor builds the credential wiring for a git URL:
//
//   - prompts are always disabled: helmdex often runs headless (server,
//     desktop, TUI) where an interactive git prompt is an invisible hang;
//     a failed auth surfaces as an error the UI turns into a sign-in flow
//   - https: a stored credential is injected through an inline credential
//     helper reading env vars, so the secret never appears in argv
//   - ssh: a stored deploy-key path is passed via GIT_SSH_COMMAND; without
//     one, the ambient ssh-agent/default keys apply as usual
func authFor(rawURL string) gitAuth {
	c, _ := creds.ForURL(rawURL, creds.KindGit)
	return authForCred(rawURL, c)
}

// authForCred builds the wiring for an explicit credential; a zero-value
// Credential yields prompt-disabling env only.
func authForCred(rawURL string, c creds.Credential) gitAuth {
	a := gitAuth{env: []string{
		"GIT_TERMINAL_PROMPT=0",
		// Git Credential Manager would otherwise pop a GUI prompt.
		"GCM_INTERACTIVE=never",
	}}

	if creds.IsSSHURL(rawURL) {
		if strings.TrimSpace(c.SSHKeyPath) != "" {
			// git runs GIT_SSH_COMMAND through `sh -c`, so the key path must
			// not be interpolated into the command string (%q is Go-quoting,
			// not shell-quoting: `$(...)` in a path would execute). Ferry it
			// through an env var, which the shell expands without re-parsing.
			a.env = append(a.env,
				"HELMDEX_SSH_KEY="+c.SSHKeyPath,
				`GIT_SSH_COMMAND=ssh -i "$HELMDEX_SSH_KEY" -o IdentitiesOnly=yes`,
			)
		}
		return a
	}

	if c.Secret != "" {
		// The empty first value resets the helper list so ambient helpers
		// cannot override the stored credential; the inline helper reads the
		// secret from the environment.
		helper := `!f() { printf 'username=%s\npassword=%s\n' "$HELMDEX_GIT_USERNAME" "$HELMDEX_GIT_PASSWORD"; }; f`
		a.configArgs = []string{"-c", "credential.helper=", "-c", "credential.helper=" + helper}
		a.env = append(a.env,
			"HELMDEX_GIT_USERNAME="+creds.DefaultUsername(c.Username),
			"HELMDEX_GIT_PASSWORD="+c.Secret,
		)
	}
	return a
}

func gitCommand(ctx context.Context, cwd string, auth gitAuth, args ...string) *exec.Cmd {
	full := append(append([]string{}, auth.configArgs...), args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), auth.env...)
	return cmd
}

func run(ctx context.Context, cwd string, auth gitAuth, args ...string) error {
	cmd := gitCommand(ctx, cwd, auth, args...)
	var stderr bytes.Buffer
	cmd.Stdout = os.Stdout
	// Keep streaming progress to the terminal while capturing the tail for
	// error context (the server surfaces it to the UI).
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s failed: %w\n%s", strings.Join(args, " "), err, tailLines(stderr.String(), 15))
	}
	return nil
}

func output(ctx context.Context, cwd string, auth gitAuth, args ...string) (string, error) {
	cmd := gitCommand(ctx, cwd, auth, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s failed: %w\n%s", strings.Join(args, " "), err, tailLines(stderr.String(), 15))
	}
	return stdout.String(), nil
}

func tailLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
