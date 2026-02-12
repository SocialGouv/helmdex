package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLI_Smoke_InitCreateList(t *testing.T) {
	repoRoot := t.TempDir()

	// init
	{
		cmd := NewRootCmd()
		cmd.SetArgs([]string{"--repo", repoRoot, "init"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("init: %v", err)
		}
	}

	// create
	{
		cmd := NewRootCmd()
		cmd.SetArgs([]string{"--repo", repoRoot, "instance", "create", "app1"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("create: %v", err)
		}
	}

	// list
	{
		var b strings.Builder
		cmd := NewRootCmd()
		cmd.SetOut(&b)
		cmd.SetArgs([]string{"--repo", repoRoot, "instance", "list"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("list: %v", err)
		}
		out := b.String()
		if !strings.Contains(out, "app1") {
			t.Fatalf("expected app1 in output, got: %q", out)
		}
	}

	// verify generated files exist
	instDir := filepath.Join(repoRoot, "apps", "app1")
	if _, err := os.Stat(filepath.Join(instDir, "Chart.yaml")); err != nil {
		t.Fatalf("missing Chart.yaml: %v", err)
	}
	if _, err := os.Stat(filepath.Join(instDir, "values.yaml")); err != nil {
		t.Fatalf("missing values.yaml: %v", err)
	}
}

