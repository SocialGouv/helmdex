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

	// Append one “current task” crumb with a strict priority order.
	if c := currentTaskCrumb(m); c != "" {
		crumbs = append(crumbs, c)
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

func currentTaskCrumb(m AppModel) string {
	// Overlays/modals (highest priority first).
	if m.infoOpen {
		return "Help / About"
	}
	if m.applyOpen {
		return "Applying"
	}
	if m.paletteOpen {
		return "Commands"
	}
	if m.sourcesOpen {
		return "Configure sources"
	}
	if m.confirmOpen {
		return "Confirm"
	}
	if m.depEditOpen {
		return "Change dependency version"
	}
	if m.depDetailOpen {
		return "Dependency detail"
	}
	if m.valuesPreviewOpen {
		return "Preview values"
	}

	// Wizard (lowest priority).
	if m.addingDep {
		step := depWizardStepLabel(m.depStep)
		if step != "" {
			return "Add dep" + crumbSep + step
		}
		return "Add dep"
	}

	return ""
}

func depWizardStepLabel(s depWizardStep) string {
	switch s {
	case depStepChooseSource:
		return "Choose source"
	case depStepCatalog:
		return "Catalog"
	case depStepCatalogDetail:
		return "Catalog detail"
	case depStepCatalogCollision:
		return "Resolve collision"
	case depStepAHQuery:
		return "Artifact Hub search"
	case depStepAHResults:
		return "Artifact Hub results"
	case depStepAHVersions:
		return "Artifact Hub versions"
	case depStepAHDetail:
		return "Artifact Hub detail"
	case depStepArbitrary:
		return "Arbitrary"
	default:
		return ""
	}
}
