package tui

import (
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func TestEscDismissesPersistentStatusErrorWhenNoModalOpen(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "."})
	m.statusErr = "boom"
	// No modal flags set by default.

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	mm := nm.(AppModel)
	if mm.statusErr != "" {
		t.Fatalf("expected statusErr to be cleared on esc, got %q", mm.statusErr)
	}
}

func TestStatusOKClearsOnNextInput(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "."})
	m.statusOK = "Applied"

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	mm := nm.(AppModel)
	if mm.statusOK != "" {
		t.Fatalf("expected statusOK to clear on next key, got %q", mm.statusOK)
	}
}

func TestIsAnyFilterActiveIncludesValuesList(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "."})
	m.screen = ScreenInstance
	m.activeTab = InstanceTabValues
	// Ensure the list has items + size so bubbles/list can enter filtering mode.
	m.valuesList.SetItems([]list.Item{valuesFileItem("values.instance.yaml")})
	m.valuesList.SetSize(40, 10)
	if m.isAnyFilterActive() {
		t.Fatalf("expected no filters active")
	}

	// Start filtering on values list via keypress routing.
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	mm := nm.(AppModel)
	if mm.valuesList.FilterState() == list.Unfiltered {
		t.Fatalf("expected valuesList to enter filtering")
	}
	if !mm.isAnyFilterActive() {
		t.Fatalf("expected filter to be detected as active")
	}
}
