package paths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStateDir_OptedRepoUsesInRepoState(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "helmdex.yaml"), []byte("apiVersion: helmdex.io/v1alpha1\nkind: HelmdexConfig\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, want := StateDir(repo), filepath.Join(repo, ".helmdex"); got != want {
		t.Fatalf("StateDir = %q, want %q", got, want)
	}
}

func TestStateDir_AgnosticRepoStaysOutsideRepo(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("HELMDEX_CACHE_DIR", cache)

	repo := t.TempDir()
	got := StateDir(repo)
	if strings.HasPrefix(got, repo) {
		t.Fatalf("StateDir %q must not live inside the agnostic repo %q", got, repo)
	}
	if !strings.HasPrefix(got, filepath.Join(cache, "repos")) {
		t.Fatalf("StateDir %q not under cache dir %q", got, cache)
	}
	// Stable across calls.
	if again := StateDir(repo); again != got {
		t.Fatalf("StateDir not stable: %q vs %q", again, got)
	}
	// Distinct repos get distinct state dirs.
	other := t.TempDir()
	if StateDir(other) == got {
		t.Fatalf("distinct repos must get distinct state dirs")
	}
}

func TestState_JoinsParts(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("HELMDEX_CACHE_DIR", cache)
	repo := t.TempDir()
	got := State(repo, "cache", "helmcharts")
	if got != filepath.Join(StateDir(repo), "cache", "helmcharts") {
		t.Fatalf("State join mismatch: %q", got)
	}
}
