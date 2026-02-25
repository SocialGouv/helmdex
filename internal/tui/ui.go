package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// modalMaxHeight returns a conservative max height for full-body modals.
//
// AppModel.View() wraps the body with:
// - base padding (top+bottom)
// - app header + breadcrumb
// - context help + status footer
// plus blank spacer lines.
// Keeping modals under this height prevents the terminal from scrolling and
// cutting off the modal's top border.
func modalMaxHeight(m AppModel) int {
	// Empirically: header/breadcrumb + spacers + context help/status + base padding
	// consumes ~10 terminal rows.
	return max(8, m.height-10)
}

func renderWithModal(m AppModel, body, modal string) string {
	// Simple composition for now: render modal above body.
	// (True terminal overlay can be added later without changing call sites.)
	modalBlock := lipgloss.NewStyle().MarginBottom(1).Render(modal)
	return modalBlock + "\n" + body
}

func renderHelpOverlay(m AppModel) string {
	panel := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2)

	lines := []string{}
	lines = append(lines, lipgloss.NewStyle().Bold(true).Render("Help"))
	lines = append(lines, "")
	lines = append(lines, "Global:")
	lines = append(lines, "  m     commands")
	lines = append(lines, "  ?     toggle help")
	lines = append(lines, "  /     filter (lists)")
	lines = append(lines, "  esc   back / close / clear filter")
	lines = append(lines, "  q     quit")
	lines = append(lines, "  ctrl+d quit")
	lines = append(lines, "")
	lines = append(lines, "Navigation:")
	lines = append(lines, "  ↑/↓ or j/k   move")
	lines = append(lines, "  enter        open / confirm")
	lines = append(lines, "  ←/→ or h/l   tabs")
	lines = append(lines, "  shift+tab    previous field")
	lines = append(lines, "")
	lines = append(lines, "Context:")
	lines = append(lines, "  "+m.contextHelpLine())
	lines = append(lines, "")
	lines = append(lines, lipgloss.NewStyle().Faint(true).Render("esc or ? to close"))

	return panel.Render(strings.Join(lines, "\n"))
}

// renderBreadcrumbBar renders a persistent top breadcrumb so it's always clear
// which instance we're looking at, regardless of the active tab.
func renderBreadcrumbBar(m AppModel) string {
	// Content
	parts := []string{withIcon(iconDashboard, "Dashboard")}
	if m.screen == ScreenInstance {
		parts = append(parts, withIcon(iconInstance, "Instance"))
		if m.selected != nil && strings.TrimSpace(m.selected.Name) != "" {
			// Instance names are user content; keep them readable and unstyled.
			parts = append(parts, withIcon(iconFolder, m.selected.Name))
		}
		// When no full-screen modal/wizard is open, include the active instance tab.
		// This makes it obvious what "screen" we're in (Deps/Values/Instance).
		if m.noModalOpen() {
			if m.activeTab >= 0 && m.activeTab < len(m.tabNames) {
				parts = append(parts, m.tabNames[m.activeTab])
			}
		} else if m.addingDep {
			parts = append(parts, withIcon(iconAdd, "Add dep"))
		}
	}

	// Styling: subtle pill background, with the last crumb emphasized.
	sepStyle := styleCrumbSep
	soft := styleCrumbSoft
	strong := styleCrumbStrong
	bar := styleCrumbBar

	sep := " " + sepStyle.Render("›") + " "
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += sep
		}
		if i == len(parts)-1 {
			out += strong.Render(p)
		} else {
			out += soft.Render(p)
		}
	}
	return bar.Render(out)
}

// renderFooterStatusLine renders transient state (errors, modes, spinners) at
// the bottom. Errors never replace the top breadcrumb.
func renderFooterStatusLine(m AppModel) string {
	flags := []string{}
	if m.paletteOpen {
		flags = append(flags, styleInfo.Render(withIcon(iconCmd, "CMD")))
	}
	if m.isAnyFilterActive() {
		flags = append(flags, styleInfo.Render(withIcon(iconFilter, "FILTER")))
	}
	if m.busy > 0 {
		label := strings.TrimSpace(m.busyLabel)
		if label == "" {
			label = "Loading"
		}
		flags = append(flags, withIcon(iconBusy, m.spin.View()+" "+label))
	}

	// Left side: status line (error or OK). Errors persist until dismissed.
	left := styleMuted.Render(" ")
	if strings.TrimSpace(m.statusErr) != "" {
		left = styleErrStrong.Render(withIcon(iconErr, "ERR") + " " + m.statusErr)
	} else if strings.TrimSpace(m.statusOK) != "" {
		left = styleInfo.Render(withIcon("", "OK") + " " + m.statusOK)
	}

	right := strings.Join(flags, " ")
	if right == "" {
		right = styleMuted.Render(" ")
	}
	// Note: flags already carry their own ANSI styling; do not wrap them again
	// (it can override colors and reduce readability).

	// Render as a single line.
	line := fmt.Sprintf("%s  %s", left, right)
	return line
}

func renderDepEditModal(m AppModel) string {
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2)
	// Clamp modal height so its top border never scrolls off-screen.
	// The global View() renders: header + breadcrumb + body + contextHelp + status,
	// all inside a padded base style.
	box = box.Height(modalMaxHeight(m))
	if !m.depEditOpen {
		return ""
	}

	header := lipgloss.NewStyle().Bold(true).Render(withIcon(iconVersions, "Change dependency version"))
	if m.depEditDep.Name != "" {
		header += "\n" + styleMuted.Render(fmt.Sprintf("%s @ %s", m.depEditDep.Name, m.depEditDep.Repository))
	}
	if m.modalErr != "" {
		header += "\n" + styleErrStrong.Render(withIcon(iconErr, "Error:") + " " + m.modalErr)
	}

	var body string
	switch m.depEditMode {
	case depEditModeManual:
		body = "Enter an exact version:\n\n" + m.depEditVersionInput.View() + "\n\n(enter: apply • esc: cancel)"
	default:
		if len(m.depEditVersionsData) == 0 {
			if m.depEditLoading {
				body = styleMuted.Render("Loading versions…")
			} else {
				body = styleMuted.Render("No versions found.")
			}
		} else {
			refreshing := ""
			if m.depEditLoading {
				refreshing = "  " + styleMuted.Render("(refreshing…)")
			}
			body = m.depEditVersions.View() + refreshing + "\n" + styleMuted.Render(withIcon(iconFilter, "/: filter")+" • enter: apply • esc: cancel")
		}
	}

	return box.Render(header + "\n\n" + body)
}

func renderDepActionsModal(m AppModel) string {
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2)
	box = box.Height(modalMaxHeight(m))
	if !m.depActionsOpen {
		return ""
	}
	dep := m.depActionsDep
	header := lipgloss.NewStyle().Bold(true).Render(withIcon(iconCmd, "Dependency actions"))
	if strings.TrimSpace(dep.Name) != "" {
		line := fmt.Sprintf("%s @ %s  (%s)", dep.Name, dep.Repository, dep.Version)
		if _, label := depSourceTagAndLabel(m.depActionsSource, m.depActionsSourceOK); strings.TrimSpace(label) != "" {
			line += "  •  " + label
		}
		header += "\n" + styleMuted.Render(line)
	}
	if m.modalErr != "" {
		header += "\n" + styleErrStrong.Render(withIcon(iconErr, "Error:")+" "+m.modalErr)
	}
	body := m.depActionsList.View() + "\n" + styleMuted.Render("enter run • esc close")
	return box.Render(header + "\n\n" + body)
}

func renderDepDetailModal(m AppModel) string {
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2)
	// Clamp modal height so its top border never scrolls off-screen.
	box = box.Height(modalMaxHeight(m))
	if !m.depDetailOpen {
		return ""
	}
	dep := m.depDetailDep
	depLabel := ""
	if strings.TrimSpace(dep.Name) != "" {
		depLabel = fmt.Sprintf("%s @ %s", dep.Name, dep.Repository)
		if strings.TrimSpace(dep.Version) != "" {
			depLabel += "  (" + dep.Version + ")"
		}
		if _, label := depSourceTagAndLabel(m.depDetailSource, m.depDetailSourceOK); strings.TrimSpace(label) != "" {
			depLabel += "  •  " + label
		}
	}

	header := lipgloss.NewStyle().Bold(true).Render(withIcon(iconDeps, "Dependency"))
	if depLabel != "" {
		header += "\n" + styleMuted.Render(depLabel)
	}
	if m.modalErr != "" {
		header += "\n" + styleErrStrong.Render(withIcon(iconErr, "Error:")+" "+m.modalErr)
	}

	// Tabs
	tabsLine := renderTabs(m.depDetailTabNames, m.depDetailTab)

	var body string
	versionsTab := len(m.depDetailTabNames) - 1
	// Versions tab is last.
	if m.depDetailTab == versionsTab {
		switch m.depDetailMode {
		case depEditModeManual:
			body = "Enter an exact version:\n\n" + m.depDetailVersionInput.View() + "\n\n(enter: apply • esc: cancel)"
		default:
			if len(m.depDetailVersionsData) == 0 {
				if m.depDetailVersionsLoading {
					body = styleMuted.Render("Loading versions…")
				} else {
					body = styleMuted.Render("No versions found.")
				}
			} else {
				refreshing := ""
				if m.depDetailVersionsLoading {
					refreshing = "  " + styleMuted.Render("(refreshing…)")
				}
				body = m.depDetailVersions.View() + refreshing + "\n" + styleMuted.Render(withIcon(iconFilter, "/: filter")+" • enter: apply • esc: cancel")
			}
		}
	} else {
		if m.depDetailLoading {
			body = styleMuted.Render("Loading…")
		} else {
			body = m.depDetailPreview.View()
		}
	}

	footer := styleMuted.Render("←/→ tabs • esc close")
	return box.Render(header + "\n\n" + tabsLine + "\n\n" + body + "\n\n" + footer)
}

func renderValuesPreviewModal(m AppModel) string {
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2)
	// Clamp modal height so its top border never scrolls off-screen.
	box = box.Height(modalMaxHeight(m))
	if !m.valuesPreviewOpen {
		return ""
	}
	label := "Values"
	if strings.TrimSpace(m.valuesPreviewPath) != "" {
		label = m.valuesPreviewPath
	}
	header := lipgloss.NewStyle().Bold(true).Render(withIcon(iconValues, label))
	body := m.valuesPreview.View()
	footer := styleMuted.Render("esc close")
	return box.Render(header + "\n\n" + body + "\n\n" + footer)
}
