package tui

import (
	"testing"

	"helmdex/internal/instances"
	"helmdex/internal/yamlchart"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func TestConfirmModal_DependencyDelete_FromDepsList_CancelAndConfirm(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "."})
	m.screen = ScreenInstance
	m.activeTab = InstanceTabDeps
	inst := instances.Instance{Name: "my-app", Path: "apps/my-app"}
	m.selected = &inst

	dep := yamlchart.Dependency{Name: "postgresql", Repository: "https://example.com", Version: "1.2.3"}
	m.depsList.SetItems([]list.Item{depItem{Dep: dep}})

	// Press d to open confirm modal.
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	mm := nm.(AppModel)
	if !mm.confirmOpen {
		t.Fatalf("expected confirm modal to open")
	}
	if mm.confirmKind != confirmDeleteDependency {
		t.Fatalf("expected confirm kind to be dependency delete, got %v", mm.confirmKind)
	}

	// Cancel with n.
	nm2, _ := mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	mm2 := nm2.(AppModel)
	if mm2.confirmOpen {
		t.Fatalf("expected confirm modal to close on cancel")
	}

	// Re-open and confirm with y.
	nm3, _ := mm2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	mm3 := nm3.(AppModel)
	if !mm3.confirmOpen {
		t.Fatalf("expected confirm modal to open")
	}
	nm4, cmd := mm3.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	mm4 := nm4.(AppModel)
	if mm4.confirmOpen {
		t.Fatalf("expected confirm modal to close on confirm")
	}
	if cmd == nil {
		t.Fatalf("expected a command to be returned on confirm")
	}
}

func TestConfirmModal_InstanceDelete_FromInstanceTab_Cancel(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "."})
	m.screen = ScreenInstance
	m.activeTab = InstanceTabInstance
	inst := instances.Instance{Name: "my-app", Path: "apps/my-app"}
	m.selected = &inst

	// Press d to open confirm modal.
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	mm := nm.(AppModel)
	if !mm.confirmOpen {
		t.Fatalf("expected confirm modal to open")
	}
	if mm.confirmKind != confirmDeleteInstance {
		t.Fatalf("expected confirm kind to be instance delete, got %v", mm.confirmKind)
	}

	// Cancel with esc.
	nm2, _ := mm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	mm2 := nm2.(AppModel)
	if mm2.confirmOpen {
		t.Fatalf("expected confirm modal to close on esc")
	}
}
