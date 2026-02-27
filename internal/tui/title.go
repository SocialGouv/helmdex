package tui

import (
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	windowTitlePrefix = "🧭 HelmDex — "
	crumbSep          = " › "
)

func windowTitleEnabled() bool {
	return strings.TrimSpace(os.Getenv("HELMDEX_NO_TITLE")) != "1"
}

// buildTitleCrumbs returns a plain-text breadcrumb describing the user's current
// navigation context. It must never include ANSI styling.
func buildTitleCrumbs(m AppModel) []string {
	crumbs := []string{"Dashboard"}
	if m.screen == ScreenInstance {
		crumbs = append(crumbs, "Instance")
		if m.selected != nil {
			name := strings.TrimSpace(m.selected.Name)
			if name != "" {
				crumbs = append(crumbs, name)
			}
		}
	}

	// Append current task crumbs with a strict priority order.
	// NOTE: for the add-dep wizard we append multiple crumbs (source kind + details).
	if cs := currentTaskCrumbs(m); len(cs) > 0 {
		crumbs = append(crumbs, cs...)
	}

	return crumbs
}

func buildWindowTitle(m AppModel) string {
	crumbs := buildTitleCrumbs(m)
	return windowTitlePrefix + strings.Join(crumbs, crumbSep)
}

// wrapWithWindowTitle applies window-title updates (if enabled) to any returned
// model that is an AppModel.
func wrapWithWindowTitle(model tea.Model, cmd tea.Cmd) (tea.Model, tea.Cmd) {
	am, ok := model.(AppModel)
	if !ok {
		return model, cmd
	}
	if am.skipWindowTitleOnce {
		am.skipWindowTitleOnce = false
		return am, cmd
	}
	return am.withWindowTitle(cmd)
}

func (m AppModel) withWindowTitle(cmd tea.Cmd) (tea.Model, tea.Cmd) {
	if !windowTitleEnabled() {
		return m, cmd
	}

	title := buildWindowTitle(m)
	if title == m.lastWindowTitle {
		return m, cmd
	}
	m.lastWindowTitle = title

	set := tea.SetWindowTitle(title)
	if cmd == nil {
		return m, set
	}
	return m, tea.Batch(cmd, set)
}

func currentTaskCrumbs(m AppModel) []string {
	// Overlays/modals (highest priority first).
	if m.infoOpen {
		return []string{"Help / About"}
	}
	if m.applyOpen {
		return []string{"Applying"}
	}
	if m.paletteOpen {
		return []string{"Commands"}
	}
	if m.sourcesOpen {
		return []string{"Configure sources"}
	}
	if m.confirmOpen {
		return []string{"Confirm"}
	}
	if m.depEditOpen {
		return []string{"Change dependency version"}
	}
	if m.depDetailOpen {
		return []string{"Dependency detail"}
	}
	if m.valuesPreviewOpen {
		return []string{"Preview values"}
	}

	// Wizard (lowest priority).
	if m.addingDep {
		return addDepCrumbsPlain(m)
	}

	return nil
}
