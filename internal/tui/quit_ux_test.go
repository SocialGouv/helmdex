package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestCtrlCWorksWhileHelpOverlayOpen(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "."})
	m.helpOpen = true

	// First Ctrl+C should arm.
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	mm := nm.(AppModel)
	if !mm.quitArmed {
		t.Fatalf("expected quitArmed to be set")
	}

	// Second Ctrl+C should quit.
	_, cmd := mm.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	assertCmdQuits(t, cmd)
}
