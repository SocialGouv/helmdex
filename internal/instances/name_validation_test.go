package instances

import (
	"os"
	"path/filepath"
	"testing"
)

// Path-traversal guard: Get/Remove must reject names that escape the apps dir
// (the HTTP API decodes %2F in path segments, so an unvalidated name is an
// arbitrary-path primitive — see the security review).
func TestGetRejectsTraversal(t *testing.T) {
	repo := t.TempDir()
	outside := t.TempDir()
	// A Chart.yaml outside the repo that a traversal could otherwise reach.
	if err := os.WriteFile(filepath.Join(outside, "Chart.yaml"), []byte("name: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rel, _ := filepath.Rel(filepath.Join(repo, "apps"), filepath.Join(outside))
	for _, bad := range []string{rel, "..", "../..", "a/b", `a\b`, "../" + filepath.Base(outside)} {
		if _, err := Get(repo, "apps", bad); err == nil {
			t.Fatalf("Get accepted traversal name %q", bad)
		}
		if err := Remove(repo, "apps", bad); err == nil {
			t.Fatalf("Remove accepted traversal name %q", bad)
		}
	}
}

func TestValidateName(t *testing.T) {
	for _, ok := range []string{"demo", "demo-preprod", "app_1", "a.b"} {
		if err := ValidateName(ok); err != nil {
			t.Fatalf("ValidateName(%q) rejected: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "..", ".", "a/b", `a\b`, "../x", "a/../b"} {
		if err := ValidateName(bad); err == nil {
			t.Fatalf("ValidateName(%q) accepted", bad)
		}
	}
}
