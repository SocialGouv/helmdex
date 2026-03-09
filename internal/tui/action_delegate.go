package tui

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	tea "github.com/charmbracelet/bubbletea"
)

// chosenItem is implemented by list items that can be "chosen" (e.g. a version
// selected by the user via Enter/Space). The delegate renders chosen items
// with a distinct green accent so they stand out from the normal cursor
// highlight.
type chosenItem interface {
	IsChosen() bool
}

// actionAwareDelegate wraps a list.DefaultDelegate. For actionItem items it
// renders with a distinct accent color scheme; for chosenItem items it renders
// with a green "chosen" style; for all other items it falls through to the
// inner delegate unchanged.
type actionAwareDelegate struct {
	inner list.DefaultDelegate

	// Styles applied exclusively to actionItem entries.
	actionNormal   lipgloss.Style
	actionSelected lipgloss.Style
	actionDimmed   lipgloss.Style

	actionDescNormal   lipgloss.Style
	actionDescSelected lipgloss.Style
	actionDescDimmed   lipgloss.Style

	// Styles for chosen items (e.g. selected version).
	chosenNormal   lipgloss.Style
	chosenSelected lipgloss.Style
}

func newActionAwareDelegate() actionAwareDelegate {
	inner := list.NewDefaultDelegate()

	actionNormal := lipgloss.NewStyle().
		Foreground(colInfo).
		Padding(0, 0, 0, 2)
	actionSelected := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(colInfo).
		Foreground(colInfo).Bold(true).
		Padding(0, 0, 0, 1)
	actionDimmed := lipgloss.NewStyle().
		Foreground(colSep).
		Padding(0, 0, 0, 2)

	actionDescNormal := actionNormal.Bold(false).Foreground(colSep)
	actionDescSelected := actionSelected.Bold(false).Foreground(colSep)
	actionDescDimmed := actionDimmed

	chosenNormal := lipgloss.NewStyle().
		Foreground(colSuccess).
		Padding(0, 0, 0, 2)
	chosenSelected := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(colSuccess).
		Foreground(colSuccess).Bold(true).
		Padding(0, 0, 0, 1)

	return actionAwareDelegate{
		inner:              inner,
		actionNormal:       actionNormal,
		actionSelected:     actionSelected,
		actionDimmed:       actionDimmed,
		actionDescNormal:   actionDescNormal,
		actionDescSelected: actionDescSelected,
		actionDescDimmed:   actionDescDimmed,
		chosenNormal:       chosenNormal,
		chosenSelected:     chosenSelected,
	}
}

func (d actionAwareDelegate) Height() int  { return d.inner.Height() }
func (d actionAwareDelegate) Spacing() int { return d.inner.Spacing() }
func (d actionAwareDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return d.inner.Update(msg, m)
}

func (d actionAwareDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	// Check for chosen items first (they take priority over default rendering
	// but not over action items).
	if ci, ok := item.(chosenItem); ok && ci.IsChosen() {
		d.renderChosen(w, m, index, item)
		return
	}

	act, isAction := item.(actionItem)
	if !isAction {
		d.inner.Render(w, m, index, item)
		return
	}

	d.renderAction(w, m, index, act)
}

func (d actionAwareDelegate) renderChosen(w io.Writer, m list.Model, index int, item list.Item) {
	di, ok := item.(list.DefaultItem)
	if !ok {
		return
	}
	title := iconOK + " " + di.Title()
	if m.Width() <= 0 {
		return
	}
	textwidth := m.Width() - d.chosenNormal.GetPaddingLeft() - d.chosenNormal.GetPaddingRight()
	title = ansi.Truncate(title, textwidth, "…")

	isSelected := index == m.Index()
	if isSelected && m.FilterState() != list.Filtering {
		title = d.chosenSelected.Render(title)
	} else {
		title = d.chosenNormal.Render(title)
	}
	// Chosen items are version strings with no description; single line.
	_, _ = fmt.Fprintf(w, "%s", title)
}

func (d actionAwareDelegate) renderAction(w io.Writer, m list.Model, index int, act actionItem) {
	// Action-item rendering (mirrors DefaultDelegate logic with accent styles).
	title := act.Title()
	desc := act.Description()

	if m.Width() <= 0 {
		return
	}

	textwidth := m.Width() - d.actionNormal.GetPaddingLeft() - d.actionNormal.GetPaddingRight()
	title = ansi.Truncate(title, textwidth, "…")
	desc = ansi.Truncate(desc, textwidth, "…")

	isSelected := index == m.Index()
	emptyFilter := m.FilterState() == list.Filtering && m.FilterValue() == ""

	if emptyFilter {
		title = d.actionDimmed.Render(title)
		desc = d.actionDescDimmed.Render(desc)
	} else if isSelected && m.FilterState() != list.Filtering {
		title = d.actionSelected.Render(title)
		desc = d.actionDescSelected.Render(desc)
	} else {
		title = d.actionNormal.Render(title)
		desc = d.actionDescNormal.Render(desc)
	}

	if d.inner.ShowDescription {
		_, _ = fmt.Fprintf(w, "%s\n%s", title, desc)
		return
	}
	_, _ = fmt.Fprintf(w, "%s", title)
}
