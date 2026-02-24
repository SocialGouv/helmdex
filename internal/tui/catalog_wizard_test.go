package tui

import (
	"path/filepath"
	"runtime"
	"testing"

	"helmdex/internal/config"

	tea "github.com/charmbracelet/bubbletea"
)

func projectRootDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	// thisFile is <repo>/internal/tui/catalog_wizard_test.go
	// => repo root is ../../ from internal/tui
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

func TestAddDepCatalog_AutoSyncTriggersWhenEmptyAndSourcesConfigured(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "helmdex.yaml")
	root := projectRootDir(t)

	cfg := config.DefaultConfig()
	cfg.Platform.Name = "eks"
	cfg.Sources = append(cfg.Sources, config.Source{
		Name: "example",
		Git:  config.GitRef{URL: filepath.Join(root, "fixtures", "remote-source")},
		Presets: config.PresetsConfig{
			Enabled:    true,
			ChartsPath: "charts",
		},
		Catalog: config.CatalogConfig{
			Enabled: true,
			Path:    "catalog.yaml",
		},
	})
	if err := config.WriteFile(cfgPath, cfg); err != nil {
		t.Fatalf("write cfg: %v", err)
	}
	loaded, err := config.LoadFile(cfgPath)
	if err != nil {
		t.Fatalf("load cfg: %v", err)
	}

	m := NewAppModel(Params{RepoRoot: tmp, ConfigPath: cfgPath, Config: &loaded})
	m.screen = ScreenInstance
	m.addingDep = true
	m.depStep = depStepChooseSource

	// Choose "Predefined catalog".
	m.depSource.Select(0)
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := nm.(AppModel)
	if mm.depStep != depStepCatalog {
		t.Fatalf("expected depStep=%v, got %v", depStepCatalog, mm.depStep)
	}
	if !mm.catalogWizardSyncing {
		t.Fatalf("expected catalogWizardSyncing=true")
	}
	if cmd == nil {
		t.Fatalf("expected a command (catalog sync batch), got nil")
	}
}

func TestAddDepCatalog_EmptyCatalogShortcuts_OpenSourcesModal(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "."})
	m.screen = ScreenInstance
	m.addingDep = true
	m.depStep = depStepCatalog
	// Ensure "empty".
	m.catalogEntries = nil

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	mm := nm.(AppModel)
	if !mm.sourcesOpen {
		t.Fatalf("expected sources modal to open")
	}
}

func TestSourcesSavedMsg_WhileInWizard_TriggersCatalogSync(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "helmdex.yaml")
	root := projectRootDir(t)

	cfg := config.DefaultConfig()
	cfg.Platform.Name = "eks"
	cfg.Sources = []config.Source{{
		Name: "example",
		Git:  config.GitRef{URL: filepath.Join(root, "fixtures", "remote-source")},
		Presets: config.PresetsConfig{
			Enabled:    true,
			ChartsPath: "charts",
		},
		Catalog: config.CatalogConfig{Enabled: true, Path: "catalog.yaml"},
	}}

	if err := config.WriteFile(cfgPath, cfg); err != nil {
		t.Fatalf("write cfg: %v", err)
	}
	loaded, err := config.LoadFile(cfgPath)
	if err != nil {
		t.Fatalf("load cfg: %v", err)
	}

	m := NewAppModel(Params{RepoRoot: tmp, ConfigPath: cfgPath, Config: &loaded})
	m.screen = ScreenInstance
	m.addingDep = true
	m.depStep = depStepCatalog
	m.catalogEntries = nil

	nm, cmd := m.Update(sourcesSavedMsg{cfg: &loaded, err: nil})
	mm := nm.(AppModel)
	if !mm.catalogWizardSyncing {
		t.Fatalf("expected catalogWizardSyncing=true")
	}
	if cmd == nil {
		t.Fatalf("expected a sync command")
	}
}
