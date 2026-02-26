package tui

import (
	"os"
	"strings"
	"testing"

	"helmdex/internal/instances"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func TestShouldShowDashboardLogo_SizeThreshold(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "."})
	m.screen = ScreenDashboard
	// Must be empty to show logo.
	m.insts = nil
	// noModalOpen() should be true by default

	// Too small.
	m.width, m.height = 79, 28
	if shouldShowDashboardLogo(m) {
		t.Fatalf("expected logo hidden when width < 80")
	}
	m.width, m.height = 80, 27
	if shouldShowDashboardLogo(m) {
		t.Fatalf("expected logo hidden when height < 28")
	}

	// Big enough.
	m.width, m.height = 80, 28
	if !shouldShowDashboardLogo(m) {
		t.Fatalf("expected logo visible at >= 80x28")
	}
}

func TestShouldShowDashboardLogo_DisabledByEnv(t *testing.T) {
	old := os.Getenv("HELMDEX_NO_LOGO")
	os.Setenv("HELMDEX_NO_LOGO", "1")
	t.Cleanup(func() { _ = os.Setenv("HELMDEX_NO_LOGO", old) })

	m := NewAppModel(Params{RepoRoot: "."})
	m.screen = ScreenDashboard
	m.insts = nil
	m.width, m.height = 120, 40

	if shouldShowDashboardLogo(m) {
		t.Fatalf("expected logo hidden when HELMDEX_NO_LOGO=1")
	}
}

func TestShouldShowDashboardLogo_HidesWhenFiltering(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "."})
	m.screen = ScreenDashboard
	// Make sure the logo would otherwise be eligible.
	m.insts = nil
	m.width, m.height = 120, 40

	// Ensure list has items + size so it can enter filtering.
	m.instList.SetItems([]list.Item{instanceItem(instances.Instance{Name: "x"})})
	m.instList.SetSize(40, 10)

	// Start filtering via normal key routing.
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	mm := nm.(AppModel)

	if mm.instList.FilterState() == list.Unfiltered {
		t.Fatalf("expected instList to enter filtering")
	}
	if shouldShowDashboardLogo(mm) {
		t.Fatalf("expected logo hidden while filtering")
	}
}

func TestShouldShowDashboardLogo_HidesWhenInstancesExist(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "."})
	m.screen = ScreenDashboard
	m.width, m.height = 120, 40
	m.insts = []instances.Instance{{Name: "x", Path: "apps/x"}}

	if shouldShowDashboardLogo(m) {
		t.Fatalf("expected logo hidden when instances exist")
	}
}

func TestOverlayDashboardLogoOnView_DoesNotAddLines(t *testing.T) {
	view := strings.Join([]string{
		"line1",
		"line2",
		"line3",
		"line4",
		"line5",
		"line6",
		"line7",
		"line8",
		"line9",
		"line10",
		"line11",
		"line12",
		"line13",
		"line14",
		"line15",
		"line16",
		"line17",
}, "\n")

	out := overlayDashboardLogoOnView(view, 80)
	if strings.Count(out, "\n") != strings.Count(view, "\n") {
		t.Fatalf("expected overlay to keep same number of lines")
	}
}
