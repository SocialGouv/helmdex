package tui

import (
	"os"
	"path/filepath"
	"testing"

	"helmdex/internal/config"
)

func TestSaveSources_AllowsFilesystemDirAndClearsRef(t *testing.T) {
	tmp := t.TempDir()

	// Local directory source with no .git should be accepted.
	srcDir := filepath.Join(tmp, "fs-source")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("mkdir srcDir: %v", err)
	}

	cfgPath := filepath.Join(tmp, "helmdex.yaml")
	m := NewAppModel(Params{RepoRoot: tmp, ConfigPath: cfgPath})

	cmd := m.saveSourcesCmd("example", srcDir, "main", "eks")
	msg := cmd()
	saved, ok := msg.(sourcesSavedMsg)
	if !ok {
		t.Fatalf("expected sourcesSavedMsg, got %T", msg)
	}
	if saved.err != nil {
		t.Fatalf("unexpected error: %v", saved.err)
	}
	if saved.cfg == nil {
		t.Fatalf("expected cfg to be non-nil")
	}
	if got, want := len(saved.cfg.Sources), 1; got != want {
		t.Fatalf("sources length: got %d want %d", got, want)
	}
	if got, want := saved.cfg.Sources[0].Git.URL, srcDir; got != want {
		t.Fatalf("git.url: got %q want %q", got, want)
	}
	if got := saved.cfg.Sources[0].Git.Ref; got != "" {
		t.Fatalf("git.ref: got %q want empty (filesystem sources clear ref)", got)
	}

	// Ensure config was actually written and round-trips.
	loaded, err := config.LoadFile(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if got, want := loaded.Sources[0].Git.Ref, ""; got != want {
		t.Fatalf("loaded git.ref: got %q want %q", got, want)
	}
}

func TestSaveSources_LocalPathMustBeDirectory(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "not-a-dir")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	cfgPath := filepath.Join(tmp, "helmdex.yaml")
	m := NewAppModel(Params{RepoRoot: tmp, ConfigPath: cfgPath})

	cmd := m.saveSourcesCmd("example", path, "", "eks")
	msg := cmd()
	saved := msg.(sourcesSavedMsg)
	if saved.err == nil {
		t.Fatalf("expected error")
	}
}

func TestSaveSources_LocalPathMustExist(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "missing")

	cfgPath := filepath.Join(tmp, "helmdex.yaml")
	m := NewAppModel(Params{RepoRoot: tmp, ConfigPath: cfgPath})

	cmd := m.saveSourcesCmd("example", path, "", "eks")
	msg := cmd()
	saved := msg.(sourcesSavedMsg)
	if saved.err == nil {
		t.Fatalf("expected error")
	}
}

