package desktopstate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFileYieldsZeroState(t *testing.T) {
	st, err := loadFrom(filepath.Join(t.TempDir(), "desktop.json"))
	if err != nil {
		t.Fatalf("load missing: %v", err)
	}
	if len(st.Repos) != 0 || st.Active != "" || st.Window != nil {
		t.Fatalf("expected zero state, got %+v", st)
	}
}

func TestLoadMalformedIsError(t *testing.T) {
	p := filepath.Join(t.TempDir(), "desktop.json")
	if err := os.WriteFile(p, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadFrom(p); err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "nested", "desktop.json")
	in := State{
		Repos:  []string{"/a", "/b"},
		Active: "/b",
		Window: &Window{Width: 1200, Height: 800, Maximized: true},
	}
	if err := saveTo(p, in); err != nil {
		t.Fatalf("save: %v", err)
	}
	out, err := loadFrom(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(out.Repos) != 2 || out.Repos[0] != "/a" || out.Repos[1] != "/b" {
		t.Fatalf("repos mismatch: %+v", out.Repos)
	}
	if out.Active != "/b" {
		t.Fatalf("active mismatch: %q", out.Active)
	}
	if out.Window == nil || *out.Window != (Window{Width: 1200, Height: 800, Maximized: true}) {
		t.Fatalf("window mismatch: %+v", out.Window)
	}
}

func TestLoadMigratesLegacyLastRepo(t *testing.T) {
	p := filepath.Join(t.TempDir(), "desktop.json")
	if err := os.WriteFile(p, []byte(`{"lastRepo":"/legacy"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := loadFrom(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(st.Repos) != 1 || st.Repos[0] != "/legacy" || st.Active != "/legacy" {
		t.Fatalf("expected migrated state, got %+v", st)
	}
	if st.LastRepo != "" {
		t.Fatalf("lastRepo should be cleared after migration, got %q", st.LastRepo)
	}
}

func TestLoadRepairsDanglingActive(t *testing.T) {
	p := filepath.Join(t.TempDir(), "desktop.json")
	if err := os.WriteFile(p, []byte(`{"repos":["/a","/b"],"active":"/gone"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := loadFrom(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if st.Active != "/a" {
		t.Fatalf("expected active repaired to first repo, got %q", st.Active)
	}
}

func TestNormalizeResolvesSymlinks(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	normReal, err := Normalize(real)
	if err != nil {
		t.Fatalf("normalize real: %v", err)
	}
	normLink, err := Normalize(link)
	if err != nil {
		t.Fatalf("normalize link: %v", err)
	}
	if normLink != normReal {
		t.Fatalf("aliases must normalize identically: %q vs %q", normLink, normReal)
	}

	// Nonexistent paths cannot be resolved: fall back to abs+clean.
	ghost, err := Normalize("/no/such/dir/./x/..")
	if err != nil || ghost != "/no/such/dir" {
		t.Fatalf("fallback = (%q, %v), want (/no/such/dir, nil)", ghost, err)
	}
}

func TestLoadNormalizesStoredEntries(t *testing.T) {
	// Older versions stored the picker result as-is; non-canonical entries
	// must not survive as duplicate folders.
	p := filepath.Join(t.TempDir(), "desktop.json")
	if err := os.WriteFile(p, []byte(`{"repos":["/a/./","/a","/b/../a"],"active":"/a/./"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := loadFrom(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(st.Repos) != 1 || st.Repos[0] != "/a" || st.Active != "/a" {
		t.Fatalf("expected deduped normalized state, got %+v", st)
	}
}

func TestAddRepoDedupsAndActivates(t *testing.T) {
	st := State{Repos: []string{"/a"}, Active: "/a"}

	st, err := AddRepo(st, "/b/./sub/..")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if len(st.Repos) != 2 || st.Repos[1] != "/b" || st.Active != "/b" {
		t.Fatalf("expected /b appended+active, got %+v", st)
	}

	// Adding an already-open folder only activates it.
	st, err = AddRepo(st, "/a")
	if err != nil {
		t.Fatalf("re-add: %v", err)
	}
	if len(st.Repos) != 2 || st.Active != "/a" {
		t.Fatalf("expected dedup with /a active, got %+v", st)
	}
}

func TestRemoveRepoPicksNeighbor(t *testing.T) {
	st := State{Repos: []string{"/a", "/b", "/c"}, Active: "/b"}

	// Removing the active middle entry activates the same-index neighbor.
	st = RemoveRepo(st, "/b")
	if len(st.Repos) != 2 || st.Active != "/c" {
		t.Fatalf("expected /c active after removing /b, got %+v", st)
	}

	// Removing the active last entry activates the new last.
	st = RemoveRepo(st, "/c")
	if len(st.Repos) != 1 || st.Active != "/a" {
		t.Fatalf("expected /a active after removing /c, got %+v", st)
	}

	// Removing an inactive entry keeps the active one.
	st = State{Repos: []string{"/a", "/b"}, Active: "/a"}
	st = RemoveRepo(st, "/b")
	if st.Active != "/a" || len(st.Repos) != 1 {
		t.Fatalf("expected /a untouched, got %+v", st)
	}

	// Removing the only entry empties the state.
	st = RemoveRepo(st, "/a")
	if len(st.Repos) != 0 || st.Active != "" {
		t.Fatalf("expected empty state, got %+v", st)
	}

	// Removing an unknown entry is a no-op.
	st = State{Repos: []string{"/a"}, Active: "/a"}
	st = RemoveRepo(st, "/nope")
	if len(st.Repos) != 1 || st.Active != "/a" {
		t.Fatalf("expected no-op, got %+v", st)
	}
}
