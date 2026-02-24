package presets

import (
	"os"
	"path/filepath"
	"testing"

	"helmdex/internal/config"
	"helmdex/internal/yamlchart"
)

func TestImport_CopiesDefaultPlatformAndSelectedSet(t *testing.T) {
	repoRoot := t.TempDir()
	instancePath := filepath.Join(repoRoot, "apps", "inst")
	if err := os.MkdirAll(instancePath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Selected set is defined by presence of local file.
	if err := os.WriteFile(filepath.Join(instancePath, "values.set.prod.yaml"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write selected set: %v", err)
	}

	cfg := config.Config{
		APIVersion: config.APIVersion,
		Kind:       config.Kind,
		Platform:   config.PlatformConfig{Name: "eks"},
		Sources: []config.Source{{
			Name: "s1",
			Git:  config.GitRef{URL: "https://example.invalid"},
			Presets: config.PresetsConfig{Enabled: true, ChartsPath: "charts"},
		}},
	}

	// Cached preset repo layout for dep.
	base := filepath.Join(repoRoot, ".helmdex", "cache", "s1", "charts", "postgresql", "1.2.3")
	write(t, filepath.Join(base, "values.default.yaml"), "a: 1\n")
	write(t, filepath.Join(base, "values.platform.eks.yaml"), "p: 1\n")
	write(t, filepath.Join(base, "values.set.prod.yaml"), "s: 1\n")

	deps := []yamlchart.Dependency{{Name: "postgresql", Version: "1.2.3", Repository: "https://charts.example.invalid"}}
	_, err := Import(ImportParams{RepoRoot: repoRoot, InstancePath: instancePath, Config: cfg, Dependencies: deps})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	// Default copied.
	if b, err := os.ReadFile(filepath.Join(instancePath, "values.default.yaml")); err != nil {
		t.Fatalf("read default: %v", err)
	} else if string(b) != "postgresql:\n    a: 1\n" {
		t.Fatalf("unexpected default: %q", string(b))
	}
	// Platform copied.
	if b, err := os.ReadFile(filepath.Join(instancePath, "values.platform.yaml")); err != nil {
		t.Fatalf("read platform: %v", err)
	} else if string(b) != "postgresql:\n    p: 1\n" {
		t.Fatalf("unexpected platform: %q", string(b))
	}
	// Set overwritten.
	if b, err := os.ReadFile(filepath.Join(instancePath, "values.set.prod.yaml")); err != nil {
		t.Fatalf("read set: %v", err)
	} else if string(b) == "{}\n" {
		t.Fatalf("expected set to be overwritten")
	}
}
