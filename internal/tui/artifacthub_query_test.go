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

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatalf("expected quit command")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg, got %T", msg)
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
