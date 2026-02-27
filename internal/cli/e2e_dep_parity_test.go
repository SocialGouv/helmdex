package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"helmdex/internal/config"
	"gopkg.in/yaml.v3"
)

func TestE2E_DepDetachAndSyncPresets_Hermetic(t *testing.T) {
	// Hermetic setup: local git repo fixture, no network, no helm binary.
	repoRoot := t.TempDir()

	remoteRepo := t.TempDir()
	fixtureRoot := filepath.Join(findRepoRoot(t), "fixtures", "remote-source")
	copyDir(t, fixtureRoot, remoteRepo)
	gitInitRepo(t, remoteRepo)
	gitCommitAll(t, remoteRepo, "e2e fixtures")

	const (
		sourceName = "s1"
		platform   = "eks"
		entryNGINX = "bitnami-nginx-15.0.0"
	)

	mustRunCLI(t, repoRoot, nil, "init")

	cfgPath := filepath.Join(repoRoot, "helmdex.yaml")
	cfg := config.Config{
		APIVersion: config.APIVersion,
		Kind:       config.Kind,
		Repo:       config.RepoConfig{AppsDir: "apps"},
		Platform:   config.PlatformConfig{Name: platform},
		Sources: []config.Source{
			{
				Name: sourceName,
				Git:  config.GitRef{URL: remoteRepo},
				Presets: config.PresetsConfig{
					Enabled:    true,
					ChartsPath: "charts",
				},
				Catalog: config.CatalogConfig{
					Enabled: true,
					Path:    "catalog.yaml",
				},
			},
		},
	}
	if err := config.WriteFile(cfgPath, cfg); err != nil {
		t.Fatalf("write helmdex.yaml: %v", err)
	}

	mustRunCLI(t, repoRoot, &cfgPath, "catalog", "sync")

	// Instance + dep from catalog.
	const instance = "nginx"
	mustRunCLI(t, repoRoot, &cfgPath, "instance", "create", instance)
	mustRunCLI(t, repoRoot, &cfgPath, "instance", "dep", "add-from-catalog", instance, "--id", entryNGINX)

	instDir := filepath.Join(repoRoot, "apps", instance)
	depID := onlyDepID(t, filepath.Join(instDir, "Chart.yaml"))

	// 1) detach: requires depmeta kind=catalog.
	metaPath := filepath.Join(repoRoot, ".helmdex", "depmeta", instance, fmt.Sprintf("%s.yaml", depID))
	if err := os.MkdirAll(filepath.Dir(metaPath), 0o755); err != nil {
		t.Fatalf("mkdir depmeta dir: %v", err)
	}
	metaBytes, err := yaml.Marshal(map[string]any{
		"kind":          "catalog",
		"catalogID":     entryNGINX,
		"catalogSource": sourceName,
	})
	if err != nil {
		t.Fatalf("marshal depmeta: %v", err)
	}
	if err := os.WriteFile(metaPath, metaBytes, 0o644); err != nil {
		t.Fatalf("write depmeta: %v", err)
	}

	_ = mustRunCLI(t, repoRoot, &cfgPath, "instance", "dep", "detach", instance, depID)
	updated, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("read depmeta: %v", err)
	}
	var got map[string]any
	if err := yaml.Unmarshal(updated, &got); err != nil {
		t.Fatalf("parse depmeta: %v\n%s", err, string(updated))
	}
	if got["kind"] != "arbitrary" {
		t.Fatalf("expected kind=arbitrary after detach, got: %#v", got)
	}

	// 2) sync-presets removes orphan markers and regenerates values.
	orphan := filepath.Join(instDir, fmt.Sprintf("values.dep-set.%s--%s.yaml", depID, "does-not-exist"))
	if err := os.WriteFile(orphan, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write orphan marker: %v", err)
	}
	_ = mustRunCLI(t, repoRoot, &cfgPath, "instance", "dep", "sync-presets", instance, depID)
	if _, err := os.Stat(orphan); err == nil {
		t.Fatalf("expected orphan marker to be removed: %s", orphan)
	}
	if _, err := os.Stat(filepath.Join(instDir, "values.yaml")); err != nil {
		t.Fatalf("expected values.yaml to exist after sync-presets: %v", err)
	}
}

