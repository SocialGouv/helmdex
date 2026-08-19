package gitutil

import (
	"context"
	"os"
	"testing"
	"time"

	"helmdex/internal/creds"
)

// A shell payload in SSHKeyPath must not execute: git runs GIT_SSH_COMMAND
// through `sh -c`, so the path must be ferried via an env var, not
// interpolated into the command string.
func TestAuthForCredNoSSHKeyShellInjection(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	defer func() { _ = os.Chdir(old) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	// Slash-free filename so it is a single valid path component carrying a
	// command substitution.
	keyName := "$(touch pwned).key"
	if err := os.WriteFile(keyName, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	c := creds.Credential{Host: "x", Kind: creds.KindGit, SSHKeyPath: keyName}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Unreachable host: ls-remote fails, but ssh (and thus GIT_SSH_COMMAND
	// parsing) is invoked first — that is where the injection would fire.
	_ = VerifyAccessWithCredential(ctx, "ssh://git@127.0.0.1:1/x.git", c)

	if _, err := os.Stat("pwned"); err == nil {
		t.Fatal("shell payload in SSHKeyPath executed via GIT_SSH_COMMAND")
	}
}
