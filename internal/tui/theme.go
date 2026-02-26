package tui

import (
	"os"
	"strings"
)

import "github.com/charmbracelet/lipgloss"

// Centralized colors + icons for the TUI.
//
// Notes:
// - Icons are simple Unicode emojis/symbols (no Nerd Font dependency).
// - Keep strings short: terminals can be narrow and lists already have padding.

// Icons (keep 1 glyph per UI element where possible).
const (

	iconApp       = "🧭"
	iconDashboard = "🏠"
	iconInstance  = "📦"
	iconFolder    = "📁"
	iconAdd       = "➕"
	iconBack      = "↩"
	iconReload    = "🔄"
	iconQuit      = "⛔"
	iconRegen     = "♻"
	iconForce     = "🔃"
	iconWizard    = "🪄"
	iconCatalog   = "📚"
	iconAH        = "🌐"
	iconCustom    = "🧰"

	iconDeps     = "🧩"
	iconValues   = "⚙"
	iconPresets  = "🎛"

	iconReadme   = "📖"
	iconAHValues = "🧾"
	iconSchema   = "🧬"
	iconVersions = "🏷"
	iconSettings = "🛠"
	iconRename   = "✏"
	iconTrash    = "🗑"

	iconCmd    = "⌘"
	iconFilter = "🔎"
	iconBusy   = "⏳"
	iconErr    = "✖"
	iconInfo   = "ℹ"
	iconHelp   = "?"
)

// Color palette (ANSI 256).
const (
	colBgSubtle = lipgloss.Color("236")
	colSep      = lipgloss.Color("240")
	colText     = lipgloss.Color("252")
	colTextHi   = lipgloss.Color("231")
	colErr      = lipgloss.Color("9")
	colDiffAdd  = lipgloss.Color("10")
	colDiffDel  = lipgloss.Color("9")
	colDiffHdr  = lipgloss.Color("39")
	// Intraline highlights (backgrounds for changed substrings).
	colDiffAddIntraBg = lipgloss.Color("22")
	colDiffDelIntraBg = lipgloss.Color("52")
	colInfo     = lipgloss.Color("81")
)

// Shared styles.
var (
	// Global layout.
	styleBase = lipgloss.NewStyle().Padding(1, 1)
	// Headings (panel headers, section titles).
	styleHeading = lipgloss.NewStyle().Bold(true)
	// Common bordered panels/modals.
	stylePanelBox = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2)
	// Tabs.
	styleTabActive   = lipgloss.NewStyle().Bold(true).Underline(true)
	styleTabInactive = lipgloss.NewStyle().Faint(true)
	// Decorative ASCII art.
	styleLogoArt    = lipgloss.NewStyle().Foreground(colSep).Faint(true)
	styleLogoAccent = lipgloss.NewStyle().Foreground(colInfo).Bold(true)

	styleMuted     = lipgloss.NewStyle().Faint(true)
	styleErrStrong = lipgloss.NewStyle().Foreground(colErr).Bold(true)
	styleInfo      = lipgloss.NewStyle().Foreground(colInfo)

	// Diff styles (git-like).
	styleDiffAdd = lipgloss.NewStyle().Foreground(colDiffAdd)
	styleDiffDel = lipgloss.NewStyle().Foreground(colDiffDel)
	styleDiffHdr = lipgloss.NewStyle().Foreground(colDiffHdr).Faint(true)
	// Intraline highlights: keep foreground consistent, add background.
	styleDiffAddIntra = lipgloss.NewStyle().Foreground(colDiffAdd).Background(colDiffAddIntraBg)
	styleDiffDelIntra = lipgloss.NewStyle().Foreground(colDiffDel).Background(colDiffDelIntraBg)

	styleCrumbBar    = lipgloss.NewStyle().Background(colBgSubtle).Padding(0, 1)
	styleCrumbSep    = lipgloss.NewStyle().Foreground(colSep)
	styleCrumbSoft   = lipgloss.NewStyle().Foreground(colText)
	styleCrumbStrong = lipgloss.NewStyle().Foreground(colTextHi).Bold(true)
)

func withIcon(ic, label string) string {
	// Emoji/icon rendering can be disabled to mitigate terminal width/alignment
	// issues (some terminals treat emoji as double-width).
	if strings.TrimSpace(os.Getenv("HELMDEX_NO_ICONS")) == "1" {
		return label
	}
	if ic == "" {
		return label
	}
	if label == "" {
		return ic
	}
	return ic + " " + label
}
