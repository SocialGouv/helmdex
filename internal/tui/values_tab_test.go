package tui

import (
	"os"
	"path/filepath"
	"testing"

	"helmdex/internal/instances"

	tea "github.com/charmbracelet/bubbletea"
)

func TestValuesTabEnterOpensPreviewModal(t *testing.T) {
	tmp := t.TempDir()
	instPath := filepath.Join(tmp, "apps", "x")
	if err := os.MkdirAll(instPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(instPath, "values.instance.yaml"), []byte("a: 1\n"), 0o644); err != nil {
		t.Fatalf("write values.instance.yaml: %v", err)
	}

	m := NewAppModel(Params{RepoRoot: tmp})
	m.screen = ScreenInstance
	m.selected = &instances.Instance{Name: "x", Path: instPath}
	m.activeTab = 2
	m.refreshValuesList()

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := nm.(AppModel)
	if !mm.valuesPreviewOpen {
		t.Fatalf("expected values preview modal to open")
	}
}

func TestValuesTabEscClosesPreviewModal(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "."})
	m.screen = ScreenInstance
	m.activeTab = 2
	m.valuesPreviewOpen = true
	m.valuesPreviewPath = "values.instance.yaml"

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	mm := nm.(AppModel)
	if mm.valuesPreviewOpen {
		t.Fatalf("expected values preview modal to close")
	}
}
