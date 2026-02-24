package instances

import (
	"os"
	"path/filepath"
	"testing"

	"helmdex/internal/yamlchart"
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

func TestRename_RenamesDirAndUpdatesChartName(t *testing.T) {
	repoRoot := t.TempDir()
	inst, err := Create(repoRoot, "apps", "old")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	newInst, err := Rename(repoRoot, "apps", "old", "new")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if newInst.Name != "new" {
		t.Fatalf("unexpected name: %s", newInst.Name)
	}
	if _, err := os.Stat(inst.Path); err == nil {
		t.Fatalf("expected old path to be gone")
	}
	if _, err := os.Stat(newInst.Path); err != nil {
		t.Fatalf("expected new path to exist: %v", err)
	}

	c, err := yamlchart.ReadChart(filepath.Join(newInst.Path, "Chart.yaml"))
	if err != nil {
		t.Fatalf("ReadChart: %v", err)
	}
	if c.Name != "new" {
		t.Fatalf("expected Chart.yaml name to be updated; got %q", c.Name)
	}
}
