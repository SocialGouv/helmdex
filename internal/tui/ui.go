package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

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
	// Empirically: top bar + spacers + context help/status + base padding consumes
	// ~9 terminal rows.
	return max(8, m.height-9)
}

func renderWithModal(m AppModel, body, modal string) string {
	// Simple composition for now: render modal above body.
	// (True terminal overlay can be added later without changing call sites.)
	modalBlock := lipgloss.NewStyle().MarginBottom(1).Render(modal)
	return modalBlock + "\n" + body
}

func renderHelpOverlay(m AppModel) string {
	panel := stylePanelBox

	lines := []string{}
	lines = append(lines, styleHeading.Render("Help"))
	lines = append(lines, "")
	lines = append(lines, "Global:")
	lines = append(lines, "  m     command palette")
	lines = append(lines, "  ?     toggle help")
	lines = append(lines, "  /     filter (lists)")
	lines = append(lines, "  Esc   back / close / clear filter")
	lines = append(lines, "  q     quit")
	lines = append(lines, "  Ctrl+C twice   quit")
	lines = append(lines, "  Ctrl+D         quit")
	lines = append(lines, "")
	lines = append(lines, "Navigation:")
	lines = append(lines, "  ↑/↓ or j/k    move")
	lines = append(lines, "  Enter         open / confirm")
	lines = append(lines, "  ←/→ or h/l    tabs")
	lines = append(lines, "  Shift+Tab     previous field")
	lines = append(lines, "")
	lines = append(lines, "Context:")
	lines = append(lines, "  "+m.contextHelpLine())
	lines = append(lines, "")
	lines = append(lines, styleMuted.Render("Esc or ? to close"))

	return panel.Render(strings.Join(lines, "\n"))
}

func repoDirLabel(m AppModel) string {
	// RepoRoot is required by the program entry point, but keep this defensive.
	p := strings.TrimRight(strings.TrimSpace(m.params.RepoRoot), string(filepath.Separator))
	if p == "" {
		return "helmdex"
	}
	base := filepath.Base(p)
	if strings.TrimSpace(base) == "" || base == "." || base == string(filepath.Separator) {
		return "helmdex"
	}
	return base
}

func instanceContextLabel(m AppModel) string {
	// Wizard/modal contexts override the underlying active tab label.
	// Keep this aligned with AppModel.noModalOpen().
	if m.infoOpen {
		return withIcon(iconHelp, "Help")
	}
	if m.paletteOpen {
		return withIcon(iconCmd, "Commands")
	}
	if m.sourcesOpen {
		return withIcon(iconCmd, "Configure sources")
	}
	if m.confirmOpen {
		return withIcon(iconTrash, "Confirm")
	}
	if m.instanceManageOpen {
		// Currently only rename exists.
		return withIcon(iconRename, "Rename")
	}
	if m.depDiffOpen {
		return withIcon(iconVersions, "Upgrade diff")
	}
	if m.depDetailOpen {
		return withIcon(iconDeps, "Dependency")
	}
	if m.depEditOpen {
		return withIcon(iconVersions, "Change version")
	}
	if m.valuesPreviewOpen {
		return withIcon(iconValues, "Preview values")
	}
	if m.applyOpen {
		return withIcon(iconBusy, "Applying")
	}
	// NOTE: add-dep wizard breadcrumb is rendered by renderTopBar() using
	// addDepCrumbsStyled(). Keep this function focused on non-wizard contexts.
	if m.activeTab >= 0 && m.activeTab < len(m.tabNames) {
		return m.tabNames[m.activeTab]
	}
	return ""
}

func truncateCrumbMiddle(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	// Keep both ends readable.
	if max <= 1 {
		return "…"
	}
	keepLeft := (max - 1) / 2
	keepRight := max - 1 - keepLeft
	r := []rune(s)
	return string(r[:keepLeft]) + "…" + string(r[len(r)-keepRight:])
}

// renderTopBar renders the single-line persistent top bar.
//
// Rules (approved):
//   - Dashboard: "📁 <repoDir>" only.
//   - Instance: "📁 <repoDir> › 📦 <instanceName> › <context>".
//     Where <context> is either the active tab name or an overriding modal/wizard label.
func renderTopBar(m AppModel) string {
	parts := []string{withIcon(iconFolder, repoDirLabel(m))}
	if m.screen == ScreenDashboard {
		parts = append(parts, withIcon(iconDashboard, "Instances"))
	}
	if m.screen == ScreenInstance {
		if m.selected != nil && strings.TrimSpace(m.selected.Name) != "" {
			// Instance names are user content; keep them readable and unstyled.
			parts = append(parts, withIcon(iconInstance, m.selected.Name))
		}
		if m.addingDep {
			// Add-dep wizard: render a complete breadcrumb including source kind.
			parts = append(parts, addDepCrumbsStyled(m)...)
			// Prefer truncating the earliest long crumb (typically the entry ID).
			// Keep this conservative; terminals vary (emoji width etc.).
			if m.width > 0 {
				maxTotal := max(20, m.width-4)
				// Approximate rendered width by rune count (best-effort).
				sep := " › "
				for {
					total := 0
					for i, p := range parts {
						if i > 0 {
							total += utf8.RuneCountInString(sep)
						}
						total += utf8.RuneCountInString(p)
					}
					if total <= maxTotal {
						break
					}
					// Find earliest crumb after the 3 fixed crumbs (repo, instance, Add dep)
					// that can be shortened.
					idx := -1
					for i := 3; i < len(parts); i++ {
						if utf8.RuneCountInString(parts[i]) > 12 {
							idx = i
							break
						}
					}
					if idx == -1 {
						break
					}
					parts[idx] = truncateCrumbMiddle(parts[idx], 12)
				}
			}
		} else {
			if ctx := instanceContextLabel(m); strings.TrimSpace(ctx) != "" {
				parts = append(parts, ctx)
			}
		}
	}

	// Styling: subtle pill background, with the last crumb emphasized.
	sepStyle := styleCrumbSep
	soft := styleCrumbSoft
	strong := styleCrumbStrong
	bar := styleCrumbBar
	// Ensure the bar is always a single line and doesn't blank out if it exceeds
	// terminal width (some terminals clear lines on overflow in alt-screen).
	bar = bar.Width(max(0, m.width-2))

	sep := " " + sepStyle.Render("›") + "  "
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
	} else if m.quitArmed {
		left = styleInfo.Render("Press Ctrl+C again to quit")
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
	box := stylePanelBox
	// Clamp modal height so its top border never scrolls off-screen.
	// The global View() renders: header + breadcrumb + body + contextHelp + status,
	// all inside a padded base style.
	box = box.Height(modalMaxHeight(m))
	if !m.depEditOpen {
		return ""
	}

	header := styleHeading.Render(withIcon(iconVersions, "Change dependency version"))
	if m.depEditDep.Name != "" {
		header += "\n" + styleMuted.Render(fmt.Sprintf("%s @ %s", m.depEditDep.Name, m.depEditDep.Repository))
	}
	if m.modalErr != "" {
		header += "\n" + styleErrStrong.Render(withIcon(iconErr, "Error:")+" "+m.modalErr)
	}

	var body string
	switch m.depEditMode {
	case depEditModeManual:
		body = "Enter an exact version:\n\n" + m.depEditVersionInput.View() + "\n\n" + styleMuted.Render("Enter apply • Esc cancel")
		if m.depEditSourceOK && m.depEditSource.Kind == depSourceCatalog {
			body += "\n" + styleMuted.Render("D: detach from catalog")
		}
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
			body = m.depEditVersions.View() + refreshing + "\n" + styleMuted.Render(withIcon(iconFilter, "/: filter")+" • Enter apply • Esc cancel")
			if m.depEditSourceOK && m.depEditSource.Kind == depSourceCatalog {
				body += "\n" + styleMuted.Render("D: detach from catalog")
			}
		}
	}

	return box.Render(header + "\n\n" + body)
}

func renderDepDetailModal(m AppModel) string {
	box := stylePanelBox
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

	header := styleHeading.Render(withIcon(iconDeps, "Dependency"))
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
			body = "Enter an exact version:\n\n" + m.depDetailVersionInput.View() + "\n\n" + styleMuted.Render("Enter apply • Esc cancel")
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
				body = m.depDetailVersions.View() + refreshing + "\n" + styleMuted.Render(withIcon(iconFilter, "/: filter")+" • Enter apply • Esc cancel") +
					"\n" + styleMuted.Render("Hint: this is the in-context version picker (same as `v` from Dependencies).")
			}
		}
	} else {
		if m.depDetailLoading {
			body = styleMuted.Render("Loading…")
		} else {
			body = m.depDetailPreview.View()
		}
	}

	// UX: avoid a constant footer that duplicates (and sometimes conflicts with)
	// the global context help line, which is tab-aware.
	return box.Render(header + "\n\n" + tabsLine + "\n\n" + body)
}

func renderConfirmModal(m AppModel) string {
	box := stylePanelBox
	box = box.Height(modalMaxHeight(m))
	if !m.confirmOpen {
		return ""
	}

	header := ""
	body := ""
	switch m.confirmKind {
	case confirmDeleteInstance:
		header = styleHeading.Render(withIcon(iconTrash, "Delete instance"))
		name := strings.TrimSpace(m.confirmInstanceName)
		if name != "" {
			header += "\n" + styleMuted.Render(name)
		}
		body = styleErrStrong.Render("This will delete the instance directory and its depmeta.") + "\n\n" +
			styleMuted.Render("y delete • n cancel • Esc cancel")
	case confirmDeleteDependency:
		header = styleHeading.Render(withIcon(iconTrash, "Delete dependency"))
		dep := m.confirmDep
		line := ""
		if strings.TrimSpace(dep.Name) != "" {
			line = fmt.Sprintf("%s @ %s", dep.Name, dep.Repository)
			if strings.TrimSpace(dep.Version) != "" {
				line += "  (" + dep.Version + ")"
			}
		}
		if line != "" {
			header += "\n" + styleMuted.Render(line)
		}
		body = styleErrStrong.Render("This will remove it from Chart.yaml and delete depID-keyed data (values.instance.yaml key, values.dep-set markers, depmeta).") + "\n\n" +
			styleMuted.Render("y delete • n cancel • Esc cancel")
	default:
		header = styleHeading.Render(withIcon(iconErr, "Confirm"))
		body = styleMuted.Render("No action") + "\n\n" + styleMuted.Render("Esc cancel")
	}

	return box.Render(header + "\n\n" + body)
}

func renderDepDiffModal(m AppModel) string {
	box := stylePanelBox
	box = box.Height(modalMaxHeight(m))
	if !m.depDiffOpen {
		return ""
	}
	oldV := strings.TrimSpace(m.depDiffOldDep.Version)
	newV := strings.TrimSpace(m.depDiffNewDep.Version)
	dep := m.depDiffNewDep
	label := ""
	if strings.TrimSpace(dep.Name) != "" {
		label = fmt.Sprintf("%s @ %s", dep.Name, dep.Repository)
	}
	change := ""
	if oldV != "" || newV != "" {
		change = fmt.Sprintf("%s → %s", oldV, newV)
	}

	header := styleHeading.Render(withIcon(iconVersions, "Upgrade diff"))
	if label != "" {
		header += "\n" + styleMuted.Render(label)
	}
	if change != "" {
		header += "\n" + styleMuted.Render(change)
	}
	if strings.TrimSpace(m.depDiffCountsText) != "" {
		header += "\n" + styleMuted.Render(m.depDiffCountsText)
	}
	if strings.TrimSpace(m.depDiffErr) != "" {
		header += "\n" + styleErrStrong.Render(withIcon(iconErr, "Error:")+" "+m.depDiffErr)
	}

	tabsLine := renderTabs(m.depDiffTabNames, m.depDiffTab)
	var body string
	if m.depDiffLoading {
		body = styleMuted.Render("Loading diff…")
	} else {
		body = m.depDiffPreview.View()
	}
	footer := styleMuted.Render("←/→ tabs • t toggle view • w wrap • j/k or ↑/↓ scroll • y apply • n/Esc cancel")
	return box.Render(header + "\n\n" + tabsLine + "\n\n" + body + "\n\n" + footer)
}

func renderValuesPreviewModal(m AppModel) string {
	box := stylePanelBox
	// Clamp modal height so its top border never scrolls off-screen.
	box = box.Height(modalMaxHeight(m))
	if !m.valuesPreviewOpen {
		return ""
	}
	label := "Values"
	if strings.TrimSpace(m.valuesPreviewPath) != "" {
		label = m.valuesPreviewPath
	}
	header := styleHeading.Render(withIcon(iconValues, label))
	body := m.valuesPreview.View()
	footer := styleMuted.Render("Esc close")
	return box.Render(header + "\n\n" + body + "\n\n" + footer)
}
