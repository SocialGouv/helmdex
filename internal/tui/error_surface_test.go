package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestErrMsgPrefersModalErrorWhenWizardOpen_NoFooterDup(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "."})
	m.screen = ScreenInstance
	m.addingDep = true
	m.depStep = depStepCatalog

	nm, _ := m.Update(errMsg{err: assertErr("boom")})
	mm := nm.(AppModel)
	if mm.modalErr == "" {
		t.Fatalf("expected modalErr to be set")
	}
	if mm.statusErr != "" {
		t.Fatalf("expected statusErr to remain empty while wizard open; got %q", mm.statusErr)
	}

	// Ensure Esc still works after an error; it should step back within the wizard.
	mm.depStep = depStepAHQuery
	nm2, _ := mm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	mm2 := nm2.(AppModel)
	if !mm2.addingDep || mm2.depStep != depStepChooseSource {
		t.Fatalf("expected esc to step back to choose source; addingDep=%v depStep=%v", mm2.addingDep, mm2.depStep)
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
