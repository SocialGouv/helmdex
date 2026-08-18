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

// ValidateSegment is the single guard between user-supplied strings and the
// filesystem: instance names, dependency ids and source names all reach the
// state dir through it.
func TestValidateSegment(t *testing.T) {
	accepted := []string{
		"alpha",
		"demo-preprod",
		"web.edge", // dots are legitimate in aliases and chart names
		"bitnami-nginx-15.0.0",
		"a_b",
	}
	for _, v := range accepted {
		t.Run("ok/"+v, func(t *testing.T) {
			if err := ValidateSegment("thing", v); err != nil {
				t.Fatalf("ValidateSegment(%q) = %v, want nil", v, err)
			}
		})
	}

	rejected := []string{
		"",
		"   ",
		".",
		"..",
		"../evil",
		"a/b",
		`a\b`,
		"a/../b",
		"..%2Fevil", // already decoded by net/http when it reaches us
		"nested/../..",
		"/absolute",
	}
	for _, v := range rejected {
		t.Run("rejected/"+v, func(t *testing.T) {
			if err := ValidateSegment("thing", v); err == nil {
				t.Fatalf("ValidateSegment(%q) = nil, want an error", v)
			}
		})
	}
}

// A rejected segment must never have been joinable into the state dir.
func TestValidateSegment_RejectsWhatWouldEscapeState(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "helmdex.yaml"), []byte("kind: HelmdexConfig\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stateDir := StateDir(repo)

	for _, v := range []string{"../../../evil", "../../../../etc/passwd"} {
		if err := ValidateSegment("thing", v); err == nil {
			t.Fatalf("ValidateSegment(%q) accepted a value that escapes", v)
		}
		// Without the guard, this is what the value would have addressed.
		joined := filepath.Clean(State(repo, "depmeta", "alpha", v+".yaml"))
		if strings.HasPrefix(joined, stateDir+string(filepath.Separator)) {
			t.Fatalf("expected %q to escape %s once joined, got %s", v, stateDir, joined)
		}
	}
}
