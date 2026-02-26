package tui

import (
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func TestValuesFilteringCapturesKeysAndPreventsGlobalShortcuts(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "."})
	m.screen = ScreenInstance
	m.activeTab = InstanceTabValues
	// Ensure the list can enter filtering mode.
	m.valuesList.SetItems([]list.Item{valuesFileItem("values.instance.yaml")})
	m.valuesList.SetSize(40, 10)

	// Start filtering.
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	mm := nm.(AppModel)
	if mm.valuesList.FilterState() == list.Unfiltered {
		t.Fatalf("expected valuesList to enter filtering")
	}

	// While filtering, typing 'r' must NOT trigger global regen values.
	// (It should be consumed by the filter input instead.)
	nm2, _ := mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	mm2 := nm2.(AppModel)
	if mm2.busyLabel == "Regenerating values" {
		t.Fatalf("expected global regen shortcut to be ignored while values filter is active")
	}
}
