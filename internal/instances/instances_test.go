package instances

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreate_CreatesFiles(t *testing.T) {
	repoRoot := t.TempDir()
	inst, err := Create(repoRoot, "apps", "myapp")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if inst.Name != "myapp" {
		t.Fatalf("unexpected name: %s", inst.Name)
	}

	if _, err := os.Stat(filepath.Join(inst.Path, "Chart.yaml")); err != nil {
		t.Fatalf("missing Chart.yaml: %v", err)
	}
	if _, err := os.Stat(filepath.Join(inst.Path, "values.instance.yaml")); err != nil {
		t.Fatalf("missing values.instance.yaml: %v", err)
	}
}

func TestCreate_RejectsBadName(t *testing.T) {
	repoRoot := t.TempDir()
	if _, err := Create(repoRoot, "apps", "a/b"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestCreate_NotIdempotent(t *testing.T) {
	repoRoot := t.TempDir()
	if _, err := Create(repoRoot, "apps", "myapp"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := Create(repoRoot, "apps", "myapp"); err == nil {
		t.Fatalf("expected error on second create")
	}
}

