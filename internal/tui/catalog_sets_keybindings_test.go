package tui

import (
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func TestCatalogSetsToggleDefaults_UsesShiftD(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "."})
	m.screen = ScreenInstance
	m.addingDep = true
	m.depStep = depStepCatalogDetail
	m.catalogSetsLoading = false
	m.catalogSetList.SetItems([]list.Item{
		setChoiceItem{C: setChoice{Name: "dev", Default: true, On: false}},
		setChoiceItem{C: setChoice{Name: "prod", Default: true, On: false}},
		setChoiceItem{C: setChoice{Name: "extra", Default: false, On: false}},
	})

	// Lowercase d should no-op.
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	mm := nm.(AppModel)
	items := mm.catalogSetList.Items()
	for _, it := range items {
		si := it.(setChoiceItem)
		if si.C.On {
			t.Fatalf("expected lowercase d to not toggle defaults")
		}
	}

	// Uppercase D should toggle defaults on.
	nm2, _ := mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	mm2 := nm2.(AppModel)
	items2 := mm2.catalogSetList.Items()
	for _, it := range items2 {
		si := it.(setChoiceItem)
		if si.C.Default && !si.C.On {
			t.Fatalf("expected uppercase D to toggle default sets on")
		}
		if !si.C.Default && si.C.On {
			t.Fatalf("expected uppercase D to not affect non-default sets")
		}
	}
}

