package tui

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

	iconCmd    = "⌘"
	iconInsert = "✍"
	iconFilter = "🔎"
	iconBusy   = "⏳"
	iconErr    = "✖"
)

// Color palette (ANSI 256).
const (
	colBgSubtle = lipgloss.Color("236")
	colSep      = lipgloss.Color("240")
	colText     = lipgloss.Color("252")
	colTextHi   = lipgloss.Color("231")
	colErr      = lipgloss.Color("9")
	colInfo     = lipgloss.Color("81")
)

// Shared styles.
var (
	styleMuted     = lipgloss.NewStyle().Faint(true)
	styleErrStrong = lipgloss.NewStyle().Foreground(colErr).Bold(true)
	styleInfo      = lipgloss.NewStyle().Foreground(colInfo)

	styleCrumbBar    = lipgloss.NewStyle().Background(colBgSubtle).Padding(0, 1)
	styleCrumbSep    = lipgloss.NewStyle().Foreground(colSep)
	styleCrumbSoft   = lipgloss.NewStyle().Foreground(colText)
	styleCrumbStrong = lipgloss.NewStyle().Foreground(colTextHi).Bold(true)
)

func withIcon(ic, label string) string {
	if ic == "" {
		return label
	}
	if label == "" {
		return ic
	}
	return ic + " " + label
}
