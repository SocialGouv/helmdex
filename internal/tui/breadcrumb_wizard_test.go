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

	b := renderBreadcrumbBar(m)
	if !strings.Contains(b, "Add dep") {
		t.Fatalf("expected breadcrumb to include Add dep when wizard is open; got %q", b)
	}
	if strings.Contains(b, "Dependencies") {
		t.Fatalf("expected breadcrumb NOT to include underlying tab when wizard is open; got %q", b)
	}
}

