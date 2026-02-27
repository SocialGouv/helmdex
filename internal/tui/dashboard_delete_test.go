package tui

import (
	"strings"
	"testing"

	"helmdex/internal/instances"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func TestDashboardDeleteShortcut_OpensConfirmForSelectedInstance(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "."})
	m.screen = ScreenDashboard
	m.instList.SetItems([]list.Item{instanceItem{Inst: instances.Instance{Name: "x", Path: "apps/x"}, RepoRoot: m.params.RepoRoot}})

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	mm := nm.(AppModel)
	if !mm.confirmOpen {
		t.Fatalf("expected confirm modal to open")
	}
	if mm.confirmKind != confirmDeleteInstance {
		t.Fatalf("expected confirm kind delete instance")
	}
}

func TestDashboardFilteringCapturesTyping_AndDoesNotTriggerDeleteShortcut(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "."})
	m.screen = ScreenDashboard
	// Ensure list can enter filtering mode.
	m.instList.SetItems([]list.Item{instanceItem{Inst: instances.Instance{Name: "demo", Path: "apps/demo"}, RepoRoot: m.params.RepoRoot}})

	// Start filtering (handled by bubbles/list; routed through normal Update).
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	mm := nm.(AppModel)
	if mm.instList.FilterState() == list.Unfiltered {
		t.Fatalf("expected instList to enter filtering")
	}

	// While filtering, typing 'd' must be treated as filter input, not delete.
	nm2, _ := mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	mm2 := nm2.(AppModel)
	if mm2.confirmOpen {
		t.Fatalf("expected confirm modal to stay closed while filtering")
	}
}

func TestInstanceListRendersRelativePath(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "/repo"})
	m.screen = ScreenDashboard
	// Ensure list renders items.
	m.instList.SetSize(60, 10)

	m.instList.SetItems([]list.Item{instanceItem{Inst: instances.Instance{Name: "demo", Path: "/repo/apps/demo"}, RepoRoot: m.params.RepoRoot}})

	v := m.instList.View()
	if strings.Contains(v, "/repo/apps/demo") {
		t.Fatalf("expected absolute path to be hidden; got %q", v)
	}
	if !strings.Contains(v, "apps/demo") {
		t.Fatalf("expected relative path to be shown; got %q", v)
	}
}
