package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

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
	lines = append(lines, "")
	lines = append(lines, "Navigation:")
	lines = append(lines, "  ↑/↓ or j/k   move")
	lines = append(lines, "  enter        open / confirm")
	lines = append(lines, "  ←/→ or h/l   tabs")
	lines = append(lines, "")
	lines = append(lines, "Context:")
	lines = append(lines, "  "+m.contextHelpLine())
	lines = append(lines, "")
	lines = append(lines, lipgloss.NewStyle().Faint(true).Render("esc or ? to close"))

	return panel.Render(strings.Join(lines, "\n"))
}

func renderStatusBar(m AppModel) string {
	// Breadcrumb
	b := "Dashboard"
	if m.screen == ScreenInstance {
		if m.selected != nil {
			b = "Instance / " + m.selected.Name
		} else {
			b = "Instance"
		}
		if m.addingDep {
			b += " / Add dep"
		}
	}

	flags := []string{}
	if m.paletteOpen {
		flags = append(flags, "CMD")
	}
	if m.creating || m.palette.QueryFocused() || (m.addingDep && m.depStep == depStepAHQuery) {
		flags = append(flags, "INSERT")
	}
	if m.isAnyFilterActive() {
		flags = append(flags, "FILTER")
	}
	if m.busy > 0 {
		label := strings.TrimSpace(m.busyLabel)
		if label == "" {
			label = "Loading"
		}
		flags = append(flags, m.spin.View()+" "+label)
	}

	err := ""
	if m.statusErr != "" && time.Since(m.statusErrAt) < 6*time.Second {
		err = "ERR " + m.statusErr
	}

	left := b
	if err != "" {
		left = err
	}
	right := strings.Join(flags, " ")
	if right == "" {
		right = " "
	}

	// Render as a single line.
	line := fmt.Sprintf("%s  %s", left, right)
	return lipgloss.NewStyle().Faint(true).Render(line)
}

func renderDepEditModal(m AppModel) string {
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2)
	if !m.depEditOpen {
		return ""
	}

	header := lipgloss.NewStyle().Bold(true).Render("Change dependency version")
	if m.depEditDep.Name != "" {
		header += "\n" + lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf("%s @ %s", m.depEditDep.Name, m.depEditDep.Repository))
	}
	if m.modalErr != "" {
		header += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("Error: "+m.modalErr)
	}

	var body string
	switch m.depEditMode {
	case depEditModeManual:
		body = "Enter an exact version:\n\n" + m.depEditVersionInput.View() + "\n\n(enter: apply • esc: cancel)"
	default:
		if m.depEditLoading {
			body = lipgloss.NewStyle().Faint(true).Render("Loading versions…")
		} else if len(m.depEditVersionsData) == 0 {
			body = lipgloss.NewStyle().Faint(true).Render("No versions found.")
		} else {
			body = m.depEditVersions.View() + "\n" + lipgloss.NewStyle().Faint(true).Render("/: filter • enter: apply • esc: cancel")
		}
	}

	return box.Render(header + "\n\n" + body)
}
