package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type paletteCmdID string

const (
	palNewInstance  paletteCmdID = "new-instance"
	palReload       paletteCmdID = "reload"
	palQuit         paletteCmdID = "quit"
	palBack         paletteCmdID = "back"
	palAddDep       paletteCmdID = "add-dep"
	palRegenValues  paletteCmdID = "regen-values"
	palForceRefresh paletteCmdID = "force-refresh-ah"
)

type paletteItem struct {
	ID   paletteCmdID
	Name string
	Desc string
}

func (p paletteItem) Title() string       { return p.Name }
func (p paletteItem) Description() string { return p.Desc }
func (p paletteItem) FilterValue() string { return p.Name + " " + p.Desc }

type paletteModel struct {
	query textinput.Model
	list  list.Model
	// all items before query filtering
	items []paletteItem
}

func newPaletteModel() paletteModel {
	q := textinput.New()
	q.Placeholder = "type a command…"
	q.Prompt = "> "
	q.Focus()

	l := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Commands"
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)

	return paletteModel{query: q, list: l}
}

func (p *paletteModel) SetSize(w, h int) {
	// Reserve some space for title, query and footer.
	p.list.SetSize(w, max(3, h-6))
}

func (p *paletteModel) Open(m AppModel) {
	p.query.SetValue("")
	p.query.Focus()
	p.items = paletteItemsFor(m)
	p.applyQueryFilter()
}

func (p *paletteModel) QueryFocused() bool { return p.query.Focused() }

func (p *paletteModel) View() string {
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2)
	content := lipgloss.NewStyle().Bold(true).Render("Command palette") + "\n\n" + p.query.View() + "\n\n" + p.list.View() + "\n" + lipgloss.NewStyle().Faint(true).Render("esc: close • enter: run")
	return box.Render(content)
}

func (p *paletteModel) Update(msg tea.Msg) (tea.Cmd, bool) {
	var cmds []tea.Cmd

	var cmd tea.Cmd
	p.query, cmd = p.query.Update(msg)
	cmds = append(cmds, cmd)
	// Apply filter after query update.
	p.applyQueryFilter()

	p.list, cmd = p.list.Update(msg)
	cmds = append(cmds, cmd)

	if km, ok := msg.(tea.KeyMsg); ok && km.Type == tea.KeyEnter {
		return tea.Batch(cmds...), true
	}
	return tea.Batch(cmds...), false
}

func (p *paletteModel) selected() (paletteItem, bool) {
	it := p.list.SelectedItem()
	if it == nil {
		return paletteItem{}, false
	}
	pi, ok := it.(paletteItem)
	return pi, ok
}

func paletteItemsFor(m AppModel) []paletteItem {
	items := []paletteItem{}

	// Always available.
	items = append(items,
		paletteItem{ID: palReload, Name: "Reload instances", Desc: "Refresh instances list"},
		paletteItem{ID: palNewInstance, Name: "New instance", Desc: "Create a new instance"},
	)

	if m.screen == ScreenInstance {
		items = append(items,
			paletteItem{ID: palAddDep, Name: "Add dependency", Desc: "Open add-dependency wizard"},
			paletteItem{ID: palRegenValues, Name: "Regenerate values.yaml", Desc: "Rebuild merged values.yaml"},
			paletteItem{ID: palBack, Name: "Back", Desc: "Return to dashboard"},
		)
	}

	if m.addingDep && m.depStep == depStepAHDetail && m.ahSelected != nil && m.ahSelectedVersion != "" {
		items = append(items, paletteItem{ID: palForceRefresh, Name: "Force refresh chart detail", Desc: "Re-run helm show and bypass cache"})
	}

	items = append(items, paletteItem{ID: palQuit, Name: "Quit", Desc: "Exit helmdex"})
	return items
}

func (p *paletteModel) applyQueryFilter() {
	q := strings.TrimSpace(strings.ToLower(p.query.Value()))
	if q == "" {
		li := make([]list.Item, 0, len(p.items))
		for _, it := range p.items {
			li = append(li, it)
		}
		p.list.SetItems(li)
		p.list.Select(0)
		return
	}
	li := []list.Item{}
	for _, it := range p.items {
		v := strings.ToLower(it.FilterValue())
		if strings.Contains(v, q) {
			li = append(li, it)
		}
	}
	p.list.SetItems(li)
	p.list.Select(0)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

