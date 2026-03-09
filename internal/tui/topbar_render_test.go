package tui

import (
	"strings"
	"testing"

	"helmdex/internal/instances"
)

func TestTopBarRenderWithNOCOLOR(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("HELMDEX_NO_ICONS", "1")
	t.Setenv("HELMDEX_NO_LOGO", "1")
	t.Setenv("HELMDEX_NO_TITLE", "1")

	m := NewAppModel(Params{RepoRoot: "/tmp/test-repo"})
	m.width = 120
	m.height = 40
	m.screen = ScreenInstance
	inst := instances.Instance{Name: "alpha", Path: "/tmp/test-repo/apps/alpha"}
	m.selected = &inst
	m.activeTab = 0

	topBar := renderTopBar(m)
	t.Logf("topBar raw: %q", topBar)
	t.Logf("topBar len: %d", len(topBar))

	// Strip ANSI
	stripped := stripANSI(topBar)
	t.Logf("topBar stripped: %q", stripped)

	// Trim trailing spaces (like normalizeScreenText does)
	trimmed := strings.TrimRight(stripped, " \t")
	t.Logf("topBar trimmed: %q", trimmed)

	if !strings.Contains(trimmed, "alpha") {
		t.Fatalf("expected 'alpha' in top bar, got %q", trimmed)
	}
}
