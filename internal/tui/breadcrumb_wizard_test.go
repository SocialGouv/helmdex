package tui

import (
	"strings"
	"testing"

	"helmdex/internal/instances"
)

func TestBreadcrumbShowsAddDepInsteadOfUnderlyingTabWhenWizardOpen(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "."})
	m.screen = ScreenInstance
	m.selected = &instances.Instance{Name: "my-app"}
	m.activeTab = InstanceTabDeps
	m.addingDep = true
	m.depStep = depStepChooseSource

	b := renderTopBar(m)
	if !strings.Contains(b, "Add dep") {
		t.Fatalf("expected top bar to include Add dep when wizard is open; got %q", b)
	}
	if strings.Contains(b, "Dependencies") {
		t.Fatalf("expected top bar NOT to include underlying tab when wizard is open; got %q", b)
	}
}
