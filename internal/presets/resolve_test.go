package presets

import (
	"os"
	"path/filepath"
	"testing"

	"helmdex/internal/config"
	"helmdex/internal/yamlchart"
)

func TestResolve_PicksExactThenConstraint(t *testing.T) {
	repoRoot := t.TempDir()
	cfg := config.Config{
		APIVersion: config.APIVersion,
		Kind:       config.Kind,
		Platform:   config.PlatformConfig{Name: "eks"},
		Sources: []config.Source{{
			Name:    "s1",
			Git:     config.GitRef{URL: "https://example.invalid"},
			Presets: config.PresetsConfig{Enabled: true, ChartsPath: "charts"},
		}},
	}

	// Create cached preset repo layout.
	base := filepath.Join(repoRoot, ".helmdex", "cache", "s1", "charts", "postgresql")
	// Constraint dir
	write(t, filepath.Join(base, ">=1.0.0 <2.0.0", "values.default.yaml"), "a: 1\n")
	// Exact dir should win
	write(t, filepath.Join(base, "1.2.3", "values.default.yaml"), "a: 2\n")
	write(t, filepath.Join(base, "1.2.3", "values.platform.eks.yaml"), "p: 1\n")
	write(t, filepath.Join(base, "1.2.3", "values.set.prod.yaml"), "s: 1\n")

	deps := []yamlchart.Dependency{{Name: "postgresql", Version: "1.2.3", Repository: "https://charts.example.invalid"}}
	res, err := Resolve(repoRoot, cfg, deps)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	id := yamlchart.DependencyID(deps[0])
	rd, ok := res.ByID[id]
	if !ok {
		t.Fatalf("missing dep")
	}
	if rd.DefaultPath == "" || filepath.Base(filepath.Dir(rd.DefaultPath)) != "1.2.3" {
		t.Fatalf("expected exact default path, got %q", rd.DefaultPath)
	}
	if rd.PlatformPath == "" {
		t.Fatalf("expected platform path")
	}
	if rd.SetPaths["prod"] == "" {
		t.Fatalf("expected prod set path")
	}
}

func write(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}
