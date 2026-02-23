package tui

import (
	"strings"

	"github.com/charmbracelet/glamour"
	glamourstyles "github.com/charmbracelet/glamour/styles"
)

// renderMarkdownForDisplay renders markdown to ANSI for terminal display.
//
// Policy:
// - If NO_COLOR is set or TERM=dumb, return the raw markdown (no rendering).
// - Always use a consistent dark theme.
// - Wrap to the provided width (best-effort).
func renderMarkdownForDisplay(width int, md string) string {
	if strings.TrimSpace(md) == "" {
		return md
	}
	// Respect NO_COLOR and dumb terminals by showing raw markdown.
	if !syntaxColorEnabled() {
		return md
	}

	// Glamour word-wrap is the primary mechanism for reflow on resize.
	// Keep a floor so tiny terminals don't panic or degenerate too badly.
	if width <= 0 {
		width = 80
	}
	if width > 2 {
		// Viewports have borders/padding outside; wrap slightly inside to reduce clipping.
		width -= 2
	}
	if width < 20 {
		width = 20
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(glamourstyles.DarkStyle),
		glamour.WithWordWrap(width),
		glamour.WithPreservedNewLines(),
		glamour.WithEmoji(),
	)
	if err != nil {
		return md
	}
	out, err := r.Render(md)
	if err != nil {
		return md
	}
	return strings.TrimRight(out, "\n")
}
