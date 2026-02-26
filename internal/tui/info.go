package tui

import (
	"fmt"
	"strings"

	"helmdex/internal/appinfo"
)

type infoTab int

const (
	infoTabHelp infoTab = iota
	infoTabAbout
)

func (t infoTab) label() string {
	switch t {
	case infoTabAbout:
		return withIcon(iconInfo, "About")
	default:
		return withIcon(iconHelp, "Help")
	}
}

func renderInfoOverlay(m AppModel) string {
	box := stylePanelBox
	box = box.Height(modalMaxHeight(m))

	tabs := []string{infoTabHelp.label(), infoTabAbout.label()}
	if m.infoTab < 0 || m.infoTab >= len(tabs) {
		// Defensive default.
		m.infoTab = 0
	}
	lineTabs := renderTabs(tabs, m.infoTab)

	body := ""
	switch infoTab(m.infoTab) {
	case infoTabAbout:
		body = renderAboutBody(m)
	default:
		body = renderHelpOverlay(m)
	}

	content := lineTabs + "\n\n" + body + "\n\n" + styleMuted.Render("←/→ tabs • esc/? close")
	return box.Render(content)
}

func renderAboutBody(m AppModel) string {
	contentWidth := max(10, m.width-6)
	lines := []string{}

	if logo := renderDashboardLogoLines(contentWidth); len(logo) > 0 {
		lines = append(lines, logo...)
		lines = append(lines, "")
	}

	lines = append(lines, appinfo.Long)
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("Version: %s", appinfo.FullVersion()))
	lines = append(lines, fmt.Sprintf("Repo:    %s", appinfo.RepoURL))
	return strings.Join(lines, "\n")
}
