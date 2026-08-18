package config

import (
	"os"
	"path/filepath"
	"testing"
)

// A per-repo override must never be promoted to the global user config on
// Save (it would silently change the layout for every other agnostic repo).
func TestSave_PerRepoOverrideDoesNotLeakGlobally(t *testing.T) {
	uc := filepath.Join(t.TempDir(), "config.yaml")
	t.Setenv("HELMDEX_USER_CONFIG", uc)
	repoA := t.TempDir()
	content := "repo:\n  appsDir: apps\nrepos:\n  " + repoA + ":\n    appsDir: charts\n"
	if err := os.WriteFile(uc, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	resA, err := Resolve(repoA, "")
	if err != nil {
		t.Fatal(err)
	}
	if resA.Config.Repo.AppsDir != "charts" {
		t.Fatalf("override not applied: %q", resA.Config.Repo.AppsDir)
	}
	// Simulate a sources save while working in repoA.
	if _, err := Save(resA, resA.Config); err != nil {
		t.Fatal(err)
	}

	// The global appsDir on disk must still be "apps"; another repo must not
	// inherit repoA's "charts".
	repoB := t.TempDir()
	resB, err := Resolve(repoB, "")
	if err != nil {
		t.Fatal(err)
	}
	if resB.Config.Repo.AppsDir != "apps" {
		t.Fatalf("per-repo override leaked to global: repoB appsDir = %q", resB.Config.Repo.AppsDir)
	}
	// And repoA's override must survive.
	resA2, _ := Resolve(repoA, "")
	if resA2.Config.Repo.AppsDir != "charts" {
		t.Fatalf("repoA override lost after save: %q", resA2.Config.Repo.AppsDir)
	}
}
