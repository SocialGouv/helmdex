package tui

import (
	"helmdex/internal/catalog"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func TestAddDepWizardEsc_StepwiseCatalogDetailToCatalogList(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "."})
	m.screen = ScreenInstance
	m.addingDep = true
	m.depStep = depStepCatalogDetail
	// Setup list/filter state to ensure we don't lose it.
	m.catalogList.SetItems([]list.Item{catalogListItem{E: dummyCatalogEntry}})
	// Keep a selection index.
	m.catalogList.Select(0)
	// Simulate we were on a specific entry.
	m.catalogDetailEntry = &dummyCatalogEntry

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	mm := nm.(AppModel)
	if !mm.addingDep {
		t.Fatalf("expected wizard to remain open")
	}
	if mm.depStep != depStepCatalog {
		t.Fatalf("expected esc to step back to catalog list; got %v", mm.depStep)
	}
	if mm.catalogDetailEntry != nil {
		t.Fatalf("expected detail entry to be cleared when stepping back")
	}
}

func TestAddDepWizardEsc_StepwiseCatalogCollisionToDetail(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "."})
	m.screen = ScreenInstance
	m.addingDep = true
	m.depStep = depStepCatalogCollision
	m.catalogDetailEntry = &dummyCatalogEntry
	m.catalogCollisionAlias.SetValue("bad")
	m.catalogCollisionAlias.Focus()

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	mm := nm.(AppModel)
	if mm.depStep != depStepCatalogDetail {
		t.Fatalf("expected esc to step back to detail; got %v", mm.depStep)
	}
	if mm.catalogCollisionAlias.Focused() {
		t.Fatalf("expected alias input to be blurred")
	}
	if got := mm.catalogCollisionAlias.Value(); got != "" {
		t.Fatalf("expected alias input value to be cleared; got %q", got)
	}
}

// dummyCatalogEntry is a minimal non-nil placeholder; only Entry.ID is used in
// staleness checks elsewhere.
var dummyCatalogEntry = func() catalog.EntryWithSource {
	return catalog.EntryWithSource{Entry: catalog.Entry{ID: "x"}}
}()
