package tui

import (
	"strings"
	"testing"
)

func TestContextHelpLine_DepDetailValuesMentionsTabsAtRoot(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "."})
	m.screen = ScreenInstance
	m.depDetailOpen = true
	// Force Values tab kind active.
	m.depDetailTabNames = []string{"Values", "Settings"}
	m.depDetailTabKinds = []depDetailTabKind{depDetailTabValues, depDetailTabDependency}
	m.depDetailTab = 0

	line := m.contextHelpLine()
	if want := "tabs (at root)"; !strings.Contains(line, want) {
		t.Fatalf("expected help line to mention %q; got %q", want, line)
	}
}
