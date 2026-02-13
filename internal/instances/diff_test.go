package instances

import (
	"os"
	"path/filepath"
	"testing"

	"helmdex/internal/yamlchart"
)

func TestDepsChanged_NoLock_NoDeps(t *testing.T) {
	dir := t.TempDir()
	chart := yamlchart.Chart{APIVersion: "v2", Name: "x", Version: "0.1.0"}
	if err := yamlchart.WriteChart(filepath.Join(dir, "Chart.yaml"), chart); err != nil {
		t.Fatalf("WriteChart: %v", err)
	}

	changed, err := DepsChanged(dir)
	if err != nil {
		t.Fatalf("DepsChanged: %v", err)
	}
	if changed {
		t.Fatalf("expected unchanged")
	}
}

func TestDepsChanged_NoLock_WithDeps(t *testing.T) {
	dir := t.TempDir()
	chart := yamlchart.Chart{APIVersion: "v2", Name: "x", Version: "0.1.0", Dependencies: []yamlchart.Dependency{{Name: "a", Version: "1.0.0", Repository: "https://example.invalid"}}}
	if err := yamlchart.WriteChart(filepath.Join(dir, "Chart.yaml"), chart); err != nil {
		t.Fatalf("WriteChart: %v", err)
	}

	changed, err := DepsChanged(dir)
	if err != nil {
		t.Fatalf("DepsChanged: %v", err)
	}
	if !changed {
		t.Fatalf("expected changed")
	}
}

func TestDepsChanged_WithLock_Matches(t *testing.T) {
	dir := t.TempDir()
	chart := yamlchart.Chart{APIVersion: "v2", Name: "x", Version: "0.1.0", Dependencies: []yamlchart.Dependency{{Name: "a", Version: "1.0.0", Repository: "https://example.invalid"}}}
	if err := yamlchart.WriteChart(filepath.Join(dir, "Chart.yaml"), chart); err != nil {
		t.Fatalf("WriteChart: %v", err)
	}
	lock := "apiVersion: v2\ndependencies:\n- name: a\n  version: 1.0.0\n  repository: https://example.invalid\n"
	if err := os.WriteFile(filepath.Join(dir, "Chart.lock"), []byte(lock), 0o644); err != nil {
		t.Fatalf("write lock: %v", err)
	}

	changed, err := DepsChanged(dir)
	if err != nil {
		t.Fatalf("DepsChanged: %v", err)
	}
	if changed {
		t.Fatalf("expected unchanged")
	}
}

func TestDepsChanged_WithLock_Differs(t *testing.T) {
	dir := t.TempDir()
	chart := yamlchart.Chart{APIVersion: "v2", Name: "x", Version: "0.1.0", Dependencies: []yamlchart.Dependency{{Name: "a", Version: "2.0.0", Repository: "https://example.invalid"}}}
	if err := yamlchart.WriteChart(filepath.Join(dir, "Chart.yaml"), chart); err != nil {
		t.Fatalf("WriteChart: %v", err)
	}
	lock := "apiVersion: v2\ndependencies:\n- name: a\n  version: 1.0.0\n  repository: https://example.invalid\n"
	if err := os.WriteFile(filepath.Join(dir, "Chart.lock"), []byte(lock), 0o644); err != nil {
		t.Fatalf("write lock: %v", err)
	}

	changed, err := DepsChanged(dir)
	if err != nil {
		t.Fatalf("DepsChanged: %v", err)
	}
	if !changed {
		t.Fatalf("expected changed")
	}
}

