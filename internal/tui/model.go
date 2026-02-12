package tui

import (
	"fmt"
	"strings"

	"helmdex/internal/instances"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type AppModel struct {
	params Params

	screen ScreenID

	// dashboard
	instList list.Model
	insts    []instances.Instance

	// instance detail
	selected *instances.Instance
	activeTab int
	tabNames  []string
	content  viewport.Model

	width  int
	height int

	keys keyMap
}

type keyMap struct {
	Quit   key.Binding
	Back   key.Binding
	Open   key.Binding
	Reload key.Binding
	TabLeft  key.Binding
	TabRight key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Open, k.TabLeft, k.TabRight, k.Reload, k.Back, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Open, k.Reload}, {k.Back, k.Quit}}
}

func NewAppModel(p Params) AppModel {
	keys := keyMap{
		Quit: key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		Back: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Open: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		Reload: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reload")),
		TabLeft:  key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "prev tab")),
		TabRight: key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "next tab")),
	}

	l := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Instances"
	l.SetShowHelp(false)

	tabNames := []string{"Overview", "Deps", "Values", "Presets"}
	vp := viewport.New(0, 0)

	m := AppModel{
		params: p,
		screen: p.StartScreen,
		instList: l,
		activeTab: 0,
		tabNames:  tabNames,
		content: vp,
		keys: keys,
	}

	return m
}

func (m AppModel) Init() tea.Cmd {
	return m.reloadInstancesCmd()
}

type errMsg struct{ err error }

type instancesMsg struct{ items []instances.Instance }

func (m AppModel) reloadInstancesCmd() tea.Cmd {
	return func() tea.Msg {
		insts, err := instances.List(m.params.RepoRoot, "apps")
		if err != nil {
			return errMsg{err}
		}
		return instancesMsg{items: insts}
	}
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.instList.SetSize(msg.Width-2, msg.Height-4)
		m.content.Width = msg.Width - 2
		m.content.Height = msg.Height - 6
		return m, nil
	case instancesMsg:
		m.insts = msg.items
		items := make([]list.Item, 0, len(msg.items))
		for _, inst := range msg.items {
			items = append(items, instanceItem(inst))
		}
		m.instList.SetItems(items)
		return m, nil
	case errMsg:
		// Show error in viewport for now.
		m.screen = ScreenInstance
		m.selected = nil
		m.content.SetContent("Error: " + msg.err.Error())
		return m, nil
	case tea.KeyMsg:
		if key.Matches(msg, m.keys.Quit) {
			return m, tea.Quit
		}
		if key.Matches(msg, m.keys.Reload) {
			return m, m.reloadInstancesCmd()
		}
		if key.Matches(msg, m.keys.Back) {
			if m.screen == ScreenInstance {
				m.screen = ScreenDashboard
				m.selected = nil
				return m, nil
			}
		}
		if key.Matches(msg, m.keys.Open) {
			if m.screen == ScreenDashboard {
				if it, ok := m.instList.SelectedItem().(instanceItem); ok {
					inst := instances.Instance(it)
					m.selected = &inst
					m.screen = ScreenInstance
					m.activeTab = 0
					m.content.SetContent(renderInstanceOverview(inst))
					return m, nil
				}
			}
		}
		if m.screen == ScreenInstance {
			if key.Matches(msg, m.keys.TabLeft) {
				m.activeTab = (m.activeTab - 1 + len(m.tabNames)) % len(m.tabNames)
				if m.selected != nil {
					m.content.SetContent(renderInstanceTab(*m.selected, m.activeTab))
				}
				return m, nil
			}
			if key.Matches(msg, m.keys.TabRight) {
				m.activeTab = (m.activeTab + 1) % len(m.tabNames)
				if m.selected != nil {
					m.content.SetContent(renderInstanceTab(*m.selected, m.activeTab))
				}
				return m, nil
			}
		}
	}

	// Delegate to focused widget.
	if m.screen == ScreenDashboard {
		var cmd tea.Cmd
		m.instList, cmd = m.instList.Update(msg)
		return m, cmd
	}

	if m.screen == ScreenInstance {
		var cmd tea.Cmd
		m.content, cmd = m.content.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m AppModel) View() string {
	base := lipgloss.NewStyle().Padding(1, 1)

	header := lipgloss.NewStyle().Bold(true).Render("helmdex")
	if m.params.RepoRoot != "" {
		header += "  " + lipgloss.NewStyle().Faint(true).Render(m.params.RepoRoot)
	}

	var body string
	switch m.screen {
	case ScreenDashboard:
		body = m.instList.View()
	case ScreenInstance:
		tabsLine := renderTabs(m.tabNames, m.activeTab)
		body = tabsLine + "\n" + m.content.View()
	default:
		body = "unknown screen"
	}

	help := lipgloss.NewStyle().Faint(true).Render(shortHelp(m.keys))

	return base.Render(strings.TrimRight(header+"\n\n"+body+"\n\n"+help, "\n"))
}

func renderTabs(names []string, active int) string {
	activeStyle := lipgloss.NewStyle().Bold(true).Underline(true)
	inactiveStyle := lipgloss.NewStyle().Faint(true)
	parts := make([]string, 0, len(names))
	for i, n := range names {
		if i == active {
			parts = append(parts, activeStyle.Render(n))
		} else {
			parts = append(parts, inactiveStyle.Render(n))
		}
	}
	return strings.Join(parts, "  ")
}

func shortHelp(k keyMap) string {
	parts := []string{}
	for _, b := range k.ShortHelp() {
		parts = append(parts, b.Help().Key+": "+b.Help().Desc)
	}
	return strings.Join(parts, " • ")
}

type instanceItem instances.Instance

func (i instanceItem) FilterValue() string { return i.Name }

func renderInstanceOverview(inst instances.Instance) string {
	return fmt.Sprintf("Instance: %s\nPath: %s\n\n(Detail tabs are stubbed in v0.2 skeleton.)", inst.Name, inst.Path)
}

func renderInstanceTab(inst instances.Instance, tab int) string {
	switch tab {
	case 0:
		return renderInstanceOverview(inst)
	case 1:
		return "Deps tab (stub): will show Chart.yaml dependencies."
	case 2:
		return "Values tab (stub): will preview values.*.yaml and allow opening $EDITOR for values.instance.yaml."
	case 3:
		return "Presets tab (stub): will preview resolved preset files per dependency."
	default:
		return "unknown tab"
	}
}
