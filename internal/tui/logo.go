package tui

import (
	_ "embed"
	"os"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
)

//go:embed logo-ascii.txt
var dashboardLogoANSI string

// ANSI SGR sequences (e.g. \x1b[38;2;...m).
var ansiSGR = regexp.MustCompile("\\x1b\\[[0-9;]*m")

func stripANSI(s string) string {
	return ansiSGR.ReplaceAllString(s, "")
}

// Keep a plain (non-colored) fallback for NO_COLOR / TERM=dumb.
var dashboardLogoPlain = strings.TrimRight(stripANSI(dashboardLogoANSI), "\n")

var dashboardLogoLineCount = func() int {
	// Count lines in the plain version for stable dimensions.
	if strings.TrimSpace(dashboardLogoPlain) == "" {
		return 0
	}
	return 1 + strings.Count(dashboardLogoPlain, "\n")
}()

func dashboardLogoEnabled() bool {
	if strings.TrimSpace(os.Getenv("HELMDEX_NO_LOGO")) == "1" {
		return false
	}
	return true
}

// shouldShowDashboardLogo returns whether we should render the decorative logo.
// It is intentionally conservative to avoid pushing list content off-screen.
func shouldShowDashboardLogo(m AppModel) bool {
	if !dashboardLogoEnabled() {
		return false
	}
	// Only when the dashboard list is truly empty (no instances in the repo).
	//
	// Important: do NOT show the logo when a filter yields 0 visible results; the
	// logo is decorative and would compete with the list's "no matches" UX.
	if len(m.insts) != 0 {
		return false
	}
	// Only on dashboard, only when no overlay/modal is open.
	if m.screen != ScreenDashboard || !m.noModalOpen() {
		return false
	}
	// Hide while filtering (either typing filter or filter applied) to reduce
	// perceived UI jitter and keep focus on the results.
	if m.instList.FilterState() != list.Unfiltered {
		return false
	}
	// Require a minimum terminal size.
	if m.width < 80 || m.height < 28 {
		return false
	}
	// Require the list viewport to be tall enough to overlay the art without
	// replacing basically the entire list.
	if dashboardLogoLineCount > 0 {
		if h := m.instList.Height(); h > 0 && h < dashboardLogoLineCount+2 {
			return false
		}
	}
	return true
}

// renderDashboardLogoLines returns centered logo lines (with ANSI colors when
// enabled). It returns nil if it cannot be safely centered.
func renderDashboardLogoLines(contentWidth int) []string {
	if contentWidth <= 0 {
		return nil
	}

	raw := strings.TrimRight(dashboardLogoANSI, "\n")
	if !syntaxColorEnabled() {
		// Respect NO_COLOR/TERM=dumb: emit no ANSI.
		raw = dashboardLogoPlain
		if strings.TrimSpace(raw) == "" {
			return nil
		}
		// Apply subtle styling in monochrome mode so it still feels "intentional".
		raw = styleLogoArt.Render(raw)
	}

	// Center each line independently (ANSI-safe width measurements).
	stylePad := lipgloss.NewStyle().Width(contentWidth)
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		w := lipgloss.Width(ln)
		pad := max(0, (contentWidth-w)/2)
		out = append(out, stylePad.Render(strings.Repeat(" ", pad)+ln))
	}
	return out
}

// overlayDashboardLogoOnView overlays the logo onto the bottom rows of the list
// view, without adding extra lines (so the list remains visible).
//
// We only overlay when the view has enough rows to paint the art.
func overlayDashboardLogoOnView(view string, contentWidth int) string {
	logoLines := renderDashboardLogoLines(contentWidth)
	if len(logoLines) == 0 {
		return view
	}
	rows := strings.Split(view, "\n")
	if len(rows) <= len(logoLines)+2 {
		// Not enough vertical space in the list view to safely overlay.
		return view
	}
	start := len(rows) - len(logoLines)
	for i := 0; i < len(logoLines); i++ {
		// Replace the entire row to avoid combining ANSI sequences.
		rows[start+i] = logoLines[i]
	}
	return strings.Join(rows, "\n")
}
