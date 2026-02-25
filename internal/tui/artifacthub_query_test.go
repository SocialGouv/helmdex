package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestArtifactHubQueryEscGoesBackToChooseSource(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "."})
	m.screen = ScreenInstance
	m.addingDep = true
	m.depStep = depStepAHQuery
	m.ahQuery.Focus()

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	mm := nm.(AppModel)
	if !mm.addingDep {
		t.Fatalf("expected wizard to remain open")
	}
	if mm.depStep != depStepChooseSource {
		t.Fatalf("expected depStep=%v, got %v", depStepChooseSource, mm.depStep)
	}
}

func TestArtifactHubQueryCtrlCQuits(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "."})
	m.screen = ScreenInstance
	m.addingDep = true
	m.depStep = depStepAHQuery
	m.ahQuery.Focus()

	// First Ctrl+C should arm quit (no immediate quit).
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatalf("expected arm timer command")
	}
	mm := nm.(AppModel)
	// Second Ctrl+C should quit.
	_, cmd2 := mm.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	assertCmdQuits(t, cmd2)
}

func TestCtrlCArmClearsOnOtherKey(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "."})
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	// Any other key clears the arm.
	armed := nm.(AppModel)
	nm2, _ := armed.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	mm := nm2.(AppModel)
	if mm.quitArmed {
		t.Fatalf("expected quitArmed to be cleared")
	}
	// Ctrl+C again should re-arm, not quit.
	_, cmd := mm.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatalf("expected arm timer command")
	}
}

func TestCtrlCArmExpiresAfterTick(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "."})
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	mm := nm.(AppModel)
	if !mm.quitArmed {
		t.Fatalf("expected quitArmed to be set")
	}
	// Simulate timer expiry.
	nm2, _ := mm.Update(quitArmExpiredMsg{id: mm.quitArmID})
	mm2 := nm2.(AppModel)
	if mm2.quitArmed {
		t.Fatalf("expected quitArmed to be cleared after expiry")
	}
}

func TestArtifactHubQueryCtrlDQuits(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "."})
	m.screen = ScreenInstance
	m.addingDep = true
	m.depStep = depStepAHQuery
	m.ahQuery.Focus()

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if cmd == nil {
		t.Fatalf("expected quit command")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg, got %T", msg)
	}
}
