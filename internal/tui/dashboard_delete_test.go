package tui

import (
	"testing"

	"helmdex/internal/instances"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func TestDashboardDeleteShortcut_OpensConfirmForSelectedInstance(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "."})
	m.screen = ScreenDashboard
	m.instList.SetItems([]list.Item{instanceItem(instances.Instance{Name: "x", Path: "apps/x"})})

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	mm := nm.(AppModel)
	if !mm.confirmOpen {
		t.Fatalf("expected confirm modal to open")
	}
	if mm.confirmKind != confirmDeleteInstance {
		t.Fatalf("expected confirm kind delete instance")
	}
}
