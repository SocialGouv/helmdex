package tui

import (
	"context"
	"time"
	"os"
	"path/filepath"
	"fmt"
	"strings"

	"helmdex/internal/artifacthub"
	"helmdex/internal/catalog"
	"helmdex/internal/helmutil"
	"helmdex/internal/instances"
	"helmdex/internal/semverutil"
	"helmdex/internal/yamlchart"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
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
	actions    list.Model
	actionsOpen bool

	// create instance
	creating bool
	newName  textinput.Model

	// instance detail
	selected *instances.Instance
	activeTab int
	tabNames  []string
	content  viewport.Model

	// deps state
	depsList list.Model
	chart    *yamlchart.Chart

	// add dep wizard
	addingDep bool
	depStep   depWizardStep
	depSource list.Model
	modalErr  string

	// catalog picker
	catalogList list.Model
	catalogEntries []catalog.Entry

	// artifacthub picker
	ahClient *artifacthub.Client
	ahQuery  textinput.Model
	ahResults list.Model
	ahResultsData []artifacthub.PackageSummary
	ahVersions list.Model
	ahVersionsData []artifacthub.Version
	ahSelected *artifacthub.PackageSummary
	ahSelectedVersion string
	ahDetailTab int
	ahDetailTabNames []string
	ahReadme string
	ahValues string
	ahLoading bool
	ahPreview viewport.Model
	ahForceRefresh bool

	// arbitrary
	arbRepo textinput.Model
	arbName textinput.Model
	arbVersion textinput.Model
	arbAlias textinput.Model
	arbFocus int

	width  int
	height int

	keys keyMap
}

type actionItem string

func (a actionItem) Title() string       { return string(a) }
func (a actionItem) Description() string { return "" }
func (a actionItem) FilterValue() string { return string(a) }

const (
	actionNewInstance          = "New instance"
	actionReloadInstances      = "Reload"
	actionForceRefreshAHDetail = "Force refresh chart detail"
)

type depWizardStep int

const (
	depStepNone depWizardStep = iota
	depStepChooseSource
	depStepCatalog
	depStepAHQuery
	depStepAHResults
	depStepAHVersions
	depStepAHDetail
	depStepArbitrary
)

type keyMap struct {
	Quit   key.Binding
	Back   key.Binding
	Open   key.Binding
	Reload key.Binding
	TabLeft  key.Binding
	TabRight key.Binding
	NewInstance key.Binding
	AddDep key.Binding
	Backspace key.Binding
	Actions key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Open, k.Actions, k.AddDep, k.TabLeft, k.TabRight, k.Reload, k.Back, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Open, k.Reload}, {k.Back, k.Quit}}
}

func NewAppModel(p Params) AppModel {
	keys := keyMap{
		Quit: key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		Back: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Open: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		Reload: key.NewBinding(key.WithKeys("ctrl+r", "f5"), key.WithHelp("ctrl+r", "reload")),
		TabLeft:  key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "prev tab")),
		TabRight: key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "next tab")),
		NewInstance: key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new instance")),
		AddDep: key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add dep")),
		Backspace: key.NewBinding(key.WithKeys("backspace"), key.WithHelp("⌫", "back")),
		Actions: key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "menu")),
	}

	l := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Instances"
	l.SetShowHelp(false)
	l.SetFilteringEnabled(true)

	deps := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	deps.Title = "Dependencies"
	deps.SetShowHelp(false)

	src := list.New([]list.Item{sourceItem("Predefined catalog"), sourceItem("Artifact Hub"), sourceItem("Arbitrary")}, list.NewDefaultDelegate(), 0, 0)
	src.Title = "Select source"
	src.SetShowHelp(false)

	catList := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	catList.Title = "Catalog"
	catList.SetFilteringEnabled(true)
	catList.SetShowHelp(false)

	ahRes := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	ahRes.Title = "Artifact Hub results"
	// Filtering here makes Enter ambiguous (it can apply filter instead of selecting).
	// We rely on the dedicated query input instead.
	ahRes.SetFilteringEnabled(false)
	ahRes.SetShowHelp(false)

	ahVers := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	ahVers.Title = "Select version"
	ahVers.SetShowHelp(false)

	newName := textinput.New()
	newName.Placeholder = "instance name"
	newName.Prompt = "> "

	q := textinput.New()
	q.Placeholder = "search charts (e.g. postgresql)"
	q.Prompt = "? "

	arbRepo := textinput.New(); arbRepo.Placeholder = "repo URL (https://... or oci://...)"; arbRepo.Prompt = "repo> "
	arbName := textinput.New(); arbName.Placeholder = "chart name"; arbName.Prompt = "name> "
	arbVersion := textinput.New(); arbVersion.Placeholder = "exact version"; arbVersion.Prompt = "version> "
	arbAlias := textinput.New(); arbAlias.Placeholder = "alias (optional)"; arbAlias.Prompt = "alias> "

	actions := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	actions.Title = "Actions"
	actions.SetShowHelp(false)
	actions.SetFilteringEnabled(false)
	actions.SetItems([]list.Item{actionItem(actionNewInstance), actionItem(actionReloadInstances), actionItem(actionForceRefreshAHDetail)})

	tabNames := []string{"Overview", "Deps", "Values", "Presets"}
	ahDetailTabNames := []string{"README", "Values", "Versions"}
	vp := viewport.New(0, 0)
	ahvp := viewport.New(0, 0)

	m := AppModel{
		params: p,
		screen: p.StartScreen,
		instList: l,
		actions: actions,
		depsList: deps,
		depSource: src,
		catalogList: catList,
		ahClient: artifacthub.NewClient(),
		ahQuery: q,
		ahResults: ahRes,
		ahVersions: ahVers,
		ahDetailTabNames: ahDetailTabNames,
		ahPreview: ahvp,
		newName: newName,
		arbRepo: arbRepo,
		arbName: arbName,
		arbVersion: arbVersion,
		arbAlias: arbAlias,
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

type chartMsg struct{ chart yamlchart.Chart }

type catalogMsg struct{ entries []catalog.Entry }

type ahSearchMsg struct{ results []artifacthub.PackageSummary }

type ahVersionsMsg struct{ versions []artifacthub.Version }

type ahDetailMsg struct{ readme, values string }

// depAppliedMsg indicates a dependency draft was applied (Chart.yaml written)
// and the modal can be closed.
type depAppliedMsg struct{ chart yamlchart.Chart }

func (m AppModel) loadHelmPreviewsCmd(repoURL, chartName, version string) tea.Cmd {
	return func() tea.Msg {
		// Force refresh is a one-shot toggle.
		force := m.ahForceRefresh
		// Reset happens in Update() when triggering; this closure just reads.

		// Cache: return immediately when present.
		if !force {
			if readme, ok, err := helmutil.ReadShowCache(m.params.RepoRoot, repoURL, chartName, version, helmutil.ShowKindReadme); err != nil {
				return errMsg{err}
			} else if ok {
				if values, ok2, err := helmutil.ReadShowCache(m.params.RepoRoot, repoURL, chartName, version, helmutil.ShowKindValues); err != nil {
					return errMsg{err}
				} else if ok2 {
					return ahDetailMsg{readme: readme, values: values}
				}
			}
		}

		// Per-repoURL isolated Helm env so repo update touches only this repo.
		env := helmutil.EnvForRepoURL(m.params.RepoRoot, repoURL)
		ctx, cancel := context.WithTimeout(contextBG(), 20*time.Second)
		defer cancel()
		// OCI refs can be used directly.
		if strings.HasPrefix(repoURL, "oci://") {
			ref := strings.TrimRight(repoURL, "/") + "/" + chartName
			readme, err := helmutil.ShowReadme(ctx, env, ref, version)
			if err != nil {
				return errMsg{err}
			}
			values, err := helmutil.ShowValues(ctx, env, ref, version)
			if err != nil {
				return errMsg{err}
			}
			_ = helmutil.WriteShowCache(m.params.RepoRoot, repoURL, chartName, version, helmutil.ShowKindReadme, readme)
			_ = helmutil.WriteShowCache(m.params.RepoRoot, repoURL, chartName, version, helmutil.ShowKindValues, values)
			return ahDetailMsg{readme: readme, values: values}
		}

		repoName := helmutil.RepoNameForURL(repoURL)
		if err := helmutil.RepoAdd(ctx, env, repoName, repoURL); err != nil {
			return errMsg{err}
		}
		if force {
			if err := helmutil.RepoUpdate(ctx, env); err != nil {
				return errMsg{err}
			}
		} else {
			if err := helmutil.RepoUpdateIfStale(ctx, env, 24*time.Hour); err != nil {
				return errMsg{err}
			}
		}
		ref := repoName + "/" + chartName
		readme, err := helmutil.ShowReadme(ctx, env, ref, version)
		if err != nil {
			return errMsg{err}
		}
		values, err := helmutil.ShowValues(ctx, env, ref, version)
		if err != nil {
			return errMsg{err}
		}
		_ = helmutil.WriteShowCache(m.params.RepoRoot, repoURL, chartName, version, helmutil.ShowKindReadme, readme)
		_ = helmutil.WriteShowCache(m.params.RepoRoot, repoURL, chartName, version, helmutil.ShowKindValues, values)
		return ahDetailMsg{readme: readme, values: values}
	}
}

func (m AppModel) reloadInstancesCmd() tea.Cmd {
	return func() tea.Msg {
		appsDir := "apps"
		if m.params.Config != nil && m.params.Config.Repo.AppsDir != "" {
			appsDir = m.params.Config.Repo.AppsDir
		}
		insts, err := instances.List(m.params.RepoRoot, appsDir)
		if err != nil {
			return errMsg{err}
		}
		return instancesMsg{items: insts}
	}
}

func (m AppModel) loadChartCmd(inst instances.Instance) tea.Cmd {
	return func() tea.Msg {
		c, err := yamlchart.ReadChart(filepath.Join(inst.Path, "Chart.yaml"))
		if err != nil {
			return errMsg{err}
		}
		return chartMsg{chart: c}
	}
}

func (m AppModel) loadCatalogCmd() tea.Cmd {
	return func() tea.Msg {
		e, err := catalog.LoadLocalCatalogEntries(m.params.RepoRoot)
		if err != nil {
			return errMsg{err}
		}
		return catalogMsg{entries: e}
	}
}

func (m AppModel) ahSearchCmd(query string) tea.Cmd {
	return func() tea.Msg {
		res, err := m.ahClient.SearchHelm(contextBG(), query, 50)
		if err != nil {
			return errMsg{err}
		}
		return ahSearchMsg{results: res}
	}
}

func (m AppModel) ahVersionsCmd(repoID, pkg string) tea.Cmd {
	return func() tea.Msg {
		detail, err := m.ahClient.GetHelmPackage(contextBG(), repoID, pkg)
		if err != nil {
			return errMsg{err}
		}
		return ahVersionsMsg{versions: detail.Versions}
	}
}

func (m AppModel) renderAHDetailBody() string {
	if m.ahSelected == nil {
		return "No selection"
	}
	if m.ahLoading {
		return "Loading chart details via helm…"
	}
	switch m.ahDetailTab {
	case 0:
		if m.ahReadme == "" {
			if m.ahSelectedVersion == "" {
				return "README not loaded yet. Loading versions…"
			}
			return "README not loaded yet. Select a version in Versions tab."
		}
		return m.ahReadme
	case 1:
		if m.ahValues == "" {
			if m.ahSelectedVersion == "" {
				return "Default values not loaded yet. Loading versions…"
			}
			return "Default values not loaded yet. Select a version in Versions tab."
		}
		return m.ahValues
	case 2:
		return "Select a version below (Enter)."
	default:
		return ""
	}
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.instList.SetSize(msg.Width-2, msg.Height-4)
		m.content.Width = msg.Width - 2
		m.content.Height = msg.Height - 6
		m.ahPreview.Width = msg.Width - 2
		m.ahPreview.Height = msg.Height - 10
		m.depsList.SetSize(msg.Width-2, msg.Height-6)
		m.depSource.SetSize(msg.Width-2, msg.Height-6)
		m.catalogList.SetSize(msg.Width-2, msg.Height-6)
		m.ahResults.SetSize(msg.Width-2, msg.Height-6)
		m.ahVersions.SetSize(msg.Width-2, msg.Height-6)
		m.actions.SetSize(min(40, msg.Width-4), min(8, msg.Height-6))
		// Ensure the viewport never ends up with negative size.
		if m.ahPreview.Height < 3 {
			m.ahPreview.Height = 3
		}
		return m, nil
	case instancesMsg:
		m.insts = msg.items
		items := make([]list.Item, 0, len(msg.items))
		for _, inst := range msg.items {
			items = append(items, instanceItem(inst))
		}
		m.instList.SetItems(items)
		return m, nil
	case chartMsg:
		m.chart = &msg.chart
		m.depsList.SetItems(depsToItems(msg.chart.Dependencies))
		return m, nil
	case depAppliedMsg:
		m.chart = &msg.chart
		m.depsList.SetItems(depsToItems(msg.chart.Dependencies))
		m.addingDep = false
		m.depStep = depStepNone
		m.modalErr = ""
		return m, nil
	case catalogMsg:
		m.catalogEntries = msg.entries
		items := make([]list.Item, 0, len(msg.entries))
		for _, e := range msg.entries {
			items = append(items, catalogListItem{E: e})
		}
		m.catalogList.SetItems(items)
		return m, nil
	case ahSearchMsg:
		m.ahResultsData = msg.results
		items := make([]list.Item, 0, len(msg.results))
		for _, r := range msg.results {
			items = append(items, ahResultItem{P: r})
		}
		m.ahResults.SetItems(items)
		m.depStep = depStepAHResults
		return m, nil
	case ahVersionsMsg:
		m.ahVersionsData = msg.versions
		items := make([]list.Item, 0, len(msg.versions))
		for _, v := range msg.versions {
			items = append(items, ahVersionItem(v))
		}
		m.ahVersions.SetItems(items)
		// If we're already in the detail screen, keep it there; otherwise the legacy
		// versions-only step is used.
		if !(m.addingDep && m.depStep == depStepAHDetail) {
			m.depStep = depStepAHVersions
		}
		// In the detail screen, auto-select the highest stable SemVer and load
		// README + values previews.
		if m.addingDep && m.depStep == depStepAHDetail && m.ahSelected != nil {
			if m.ahSelectedVersion == "" && len(msg.versions) > 0 {
				vs := make([]string, 0, len(msg.versions))
				for _, v := range msg.versions {
					vs = append(vs, v.Version)
				}
				best, ok := semverutil.BestStable(vs)
				if !ok {
					best = msg.versions[0].Version
				}
				m.ahSelectedVersion = best
				// Select matching item.
				for i := range msg.versions {
					if msg.versions[i].Version == best {
						m.ahVersions.Select(i)
						break
					}
				}
				m.ahLoading = true
				m.ahPreview.SetContent(m.renderAHDetailBody())
				if m.ahSelected.RepositoryURL != "" {
					return m, m.loadHelmPreviewsCmd(m.ahSelected.RepositoryURL, m.ahSelected.Name, m.ahSelectedVersion)
				}
				m.ahLoading = false
				m.modalErr = "selected chart has no repository URL; cannot run helm show"
			}
		}
		m.ahPreview.SetContent(m.renderAHDetailBody())
		return m, nil
	case ahDetailMsg:
		m.ahReadme = msg.readme
		m.ahValues = msg.values
		m.ahLoading = false
		m.ahPreview.SetContent(m.renderAHDetailBody())
		return m, nil
	case errMsg:
		// Prefer keeping the modal open when errors happen during add-dep flows.
		if m.addingDep {
			m.modalErr = msg.err.Error()
			m.ahLoading = false
			m.ahPreview.SetContent(m.renderAHDetailBody())
			return m, nil
		}
		// Otherwise show error in the instance viewport for now.
		m.screen = ScreenInstance
		if m.selected != nil {
			m.content.SetContent("Error: " + msg.err.Error())
		} else {
			m.content.SetContent("Error: " + msg.err.Error())
		}
		return m, nil
	case tea.KeyMsg:
		// If a text input is focused or a list filter is active, do not treat
		// characters as global shortcuts.
		if m.inputCapturesKeys() {
			break
		}

		if key.Matches(msg, m.keys.Quit) {
			return m, tea.Quit
		}
		if key.Matches(msg, m.keys.Reload) {
			// When inside Artifact Hub detail, reload means "force refresh".
			if m.addingDep && m.depStep == depStepAHDetail && m.ahSelected != nil && m.ahSelectedVersion != "" {
				m.ahForceRefresh = true
				m.ahLoading = true
				m.modalErr = ""
				m.ahPreview.SetContent(m.renderAHDetailBody())
				cmd := m.loadHelmPreviewsCmd(m.ahSelected.RepositoryURL, m.ahSelected.Name, m.ahSelectedVersion)
				m.ahForceRefresh = false
				return m, cmd
			}
			return m, m.reloadInstancesCmd()
		}
		if key.Matches(msg, m.keys.Actions) {
			m.actionsOpen = !m.actionsOpen
			return m, nil
		}
		if key.Matches(msg, m.keys.NewInstance) {
			if m.screen == ScreenDashboard {
				m.creating = true
				m.newName.SetValue("")
				m.newName.Focus()
				return m, nil
			}
		}
		if key.Matches(msg, m.keys.AddDep) {
			if m.screen == ScreenInstance && !m.addingDep {
				m.addingDep = true
				m.depStep = depStepChooseSource
				m.modalErr = ""
				return m, m.loadCatalogCmd()
			}
		}
		if key.Matches(msg, m.keys.Back) {
			if m.creating {
				m.creating = false
				return m, nil
			}
			if m.addingDep {
				m.addingDep = false
				m.depStep = depStepNone
				m.modalErr = ""
				return m, nil
			}
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
					return m, m.loadChartCmd(inst)
				}
			}
		}
		// When the add-dependency wizard is open, left/right should switch the wizard
		// detail tabs (README/Values/Versions), not the instance tabs.
		if m.screen == ScreenInstance && m.addingDep && m.depStep == depStepAHDetail {
			if key.Matches(msg, m.keys.TabLeft) {
				m.ahDetailTab = (m.ahDetailTab - 1 + len(m.ahDetailTabNames)) % len(m.ahDetailTabNames)
				m.ahPreview.SetContent(m.renderAHDetailBody())
				return m, nil
			}
			if key.Matches(msg, m.keys.TabRight) {
				m.ahDetailTab = (m.ahDetailTab + 1) % len(m.ahDetailTabNames)
				m.ahPreview.SetContent(m.renderAHDetailBody())
				return m, nil
			}
		}

		if m.screen == ScreenInstance && !m.addingDep {
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

	if m.actionsOpen {
		var cmd tea.Cmd
		m.actions, cmd = m.actions.Update(msg)
		if km, ok := msg.(tea.KeyMsg); ok && km.Type == tea.KeyEnter {
			it := m.actions.SelectedItem()
			if it != nil {
				s := string(it.(actionItem))
				switch s {
				case actionNewInstance:
					m.actionsOpen = false
					m.creating = true
					m.newName.SetValue("")
					m.newName.Focus()
				case actionReloadInstances:
					m.actionsOpen = false
					return m, m.reloadInstancesCmd()
				case actionForceRefreshAHDetail:
					m.actionsOpen = false
					if m.addingDep && m.depStep == depStepAHDetail && m.ahSelected != nil && m.ahSelectedVersion != "" {
						m.ahForceRefresh = true
						m.ahLoading = true
						m.modalErr = ""
						m.ahPreview.SetContent(m.renderAHDetailBody())
						cmd := m.loadHelmPreviewsCmd(m.ahSelected.RepositoryURL, m.ahSelected.Name, m.ahSelectedVersion)
						m.ahForceRefresh = false
						return m, cmd
					}
				}
			}
		}
		return m, cmd
	}

	// Modal: create instance.
	if m.creating {
		var cmd tea.Cmd
		m.newName, cmd = m.newName.Update(msg)
		if km, ok := msg.(tea.KeyMsg); ok && km.Type == tea.KeyEnter {
			name := strings.TrimSpace(m.newName.Value())
			appsDir := "apps"
			if m.params.Config != nil && m.params.Config.Repo.AppsDir != "" {
				appsDir = m.params.Config.Repo.AppsDir
			}
			inst, err := instances.Create(m.params.RepoRoot, appsDir, name)
			if err != nil {
				return m, func() tea.Msg { return errMsg{err} }
			}
			m.creating = false
			m.selected = &inst
			m.screen = ScreenInstance
			m.activeTab = 0
			m.content.SetContent(renderInstanceOverview(inst))
			return m, tea.Batch(m.reloadInstancesCmd(), m.loadChartCmd(inst))
		}
		return m, cmd
	}

	// Modal: add dependency wizard.
	if m.addingDep {
		switch m.depStep {
		case depStepChooseSource:
			var cmd tea.Cmd
			m.depSource, cmd = m.depSource.Update(msg)
			if km, ok := msg.(tea.KeyMsg); ok && km.Type == tea.KeyEnter {
				it := m.depSource.SelectedItem()
				if it == nil {
					return m, cmd
				}
				s := string(it.(sourceItem))
				switch s {
				case "Predefined catalog":
					m.depStep = depStepCatalog
				case "Artifact Hub":
					m.depStep = depStepAHQuery
					m.ahQuery.SetValue("")
					m.ahQuery.Focus()
				case "Arbitrary":
					m.depStep = depStepArbitrary
					m.arbFocus = 0
					m.arbRepo.Focus()
				}
			}
			return m, cmd
		case depStepCatalog:
			var cmd tea.Cmd
			m.catalogList, cmd = m.catalogList.Update(msg)
			if km, ok := msg.(tea.KeyMsg); ok && km.Type == tea.KeyEnter {
				it := m.catalogList.SelectedItem()
				if it == nil {
					return m, cmd
				}
				ci, ok := it.(catalogListItem)
				if !ok {
					return m, func() tea.Msg { return errMsg{fmt.Errorf("unexpected catalog item type")}} 
				}
				e := ci.E
				return m, m.applyDependencyDraft(yamlchart.Dependency{Name: e.Chart.Name, Repository: e.Chart.Repo, Version: e.Version})
			}
			return m, cmd
		case depStepAHQuery:
			var cmd tea.Cmd
			m.ahQuery, cmd = m.ahQuery.Update(msg)
			if km, ok := msg.(tea.KeyMsg); ok && km.Type == tea.KeyEnter {
				q := strings.TrimSpace(m.ahQuery.Value())
				if q != "" {
					return m, m.ahSearchCmd(q)
				}
			}
			return m, cmd
		case depStepAHResults:
			var cmd tea.Cmd
			m.ahResults, cmd = m.ahResults.Update(msg)
			if km, ok := msg.(tea.KeyMsg); ok && km.Type == tea.KeyEnter {
				it := m.ahResults.SelectedItem()
				if it == nil {
					return m, cmd
				}
				ai, ok := it.(ahResultItem)
				if !ok {
					return m, func() tea.Msg { return errMsg{fmt.Errorf("unexpected Artifact Hub item type")}} 
				}
				sel := ai.P
				m.ahSelected = &sel
				// Enter on a result opens a detail screen.
				m.depStep = depStepAHDetail
				m.modalErr = ""
				m.ahDetailTab = 0
				m.ahSelectedVersion = ""
				m.ahReadme = ""
				m.ahValues = ""
				m.ahLoading = false
				m.ahPreview.SetContent(m.renderAHDetailBody())
				return m, m.ahVersionsCmd(sel.RepositoryKey, sel.Name)
			}
			return m, cmd
		case depStepAHDetail:
			// Versions tab has an interactive list.
			if m.ahDetailTab == 2 {
				var cmd tea.Cmd
				m.ahVersions, cmd = m.ahVersions.Update(msg)
				if km, ok := msg.(tea.KeyMsg); ok {
					if km.Type == tea.KeyEnter {
						if m.ahSelected == nil {
							return m, cmd
						}
						it := m.ahVersions.SelectedItem()
						if it == nil {
							return m, cmd
						}
						vi, ok := it.(ahVersionItem)
						if !ok {
							return m, func() tea.Msg { return errMsg{fmt.Errorf("unexpected version item type")}} 
						}
						v := artifacthub.Version(vi)
						m.ahSelectedVersion = v.Version
						m.ahLoading = true
						m.ahReadme = ""
						m.ahValues = ""
						m.ahPreview.SetContent(m.renderAHDetailBody())
						if m.ahSelected.RepositoryURL == "" {
							m.ahLoading = false
							m.modalErr = "selected chart has no repository URL; cannot run helm show"
							return m, cmd
						}
						return m, tea.Batch(cmd, m.loadHelmPreviewsCmd(m.ahSelected.RepositoryURL, m.ahSelected.Name, v.Version))
					}
					if km.String() == "a" || km.String() == "A" {
						if m.ahSelected != nil && m.ahSelectedVersion != "" {
							return m, m.applyDependencyDraft(yamlchart.Dependency{Name: m.ahSelected.Name, Repository: m.ahSelected.RepositoryURL, Version: m.ahSelectedVersion})
						}
					}
				}
				return m, cmd
			}

			// Non-versions tabs: allow quick add if a version is selected.
			if km, ok := msg.(tea.KeyMsg); ok {
				if km.String() == "a" || km.String() == "A" {
					if m.ahSelected != nil && m.ahSelectedVersion != "" {
						return m, m.applyDependencyDraft(yamlchart.Dependency{Name: m.ahSelected.Name, Repository: m.ahSelected.RepositoryURL, Version: m.ahSelectedVersion})
					}
				}
			}
			return m, nil
		case depStepAHVersions:
			var cmd tea.Cmd
			m.ahVersions, cmd = m.ahVersions.Update(msg)
			if km, ok := msg.(tea.KeyMsg); ok && km.Type == tea.KeyEnter {
				if m.ahSelected == nil {
					return m, cmd
				}
				it := m.ahVersions.SelectedItem()
				if it == nil {
					return m, cmd
				}
				vi, ok := it.(ahVersionItem)
				if !ok {
					return m, func() tea.Msg { return errMsg{fmt.Errorf("unexpected version item type")}} 
				}
				v := artifacthub.Version(vi)
				return m, m.applyDependencyDraft(yamlchart.Dependency{Name: m.ahSelected.Name, Repository: m.ahSelected.RepositoryURL, Version: v.Version})
			}
			return m, cmd
		case depStepArbitrary:
			// Simple focus cycling with tab.
			if km, ok := msg.(tea.KeyMsg); ok {
				if km.Type == tea.KeyTab {
					m.arbFocus = (m.arbFocus + 1) % 4
					m.arbRepo.Blur(); m.arbName.Blur(); m.arbVersion.Blur(); m.arbAlias.Blur()
					switch m.arbFocus {
					case 0: m.arbRepo.Focus()
					case 1: m.arbName.Focus()
					case 2: m.arbVersion.Focus()
					case 3: m.arbAlias.Focus()
					}
				}
				if km.Type == tea.KeyEnter {
					dep := yamlchart.Dependency{Name: strings.TrimSpace(m.arbName.Value()), Repository: strings.TrimSpace(m.arbRepo.Value()), Version: strings.TrimSpace(m.arbVersion.Value()), Alias: strings.TrimSpace(m.arbAlias.Value())}
					return m, m.applyDependencyDraft(dep)
				}
			}
			var cmds []tea.Cmd
			var cmd tea.Cmd
			m.arbRepo, cmd = m.arbRepo.Update(msg); cmds = append(cmds, cmd)
			m.arbName, cmd = m.arbName.Update(msg); cmds = append(cmds, cmd)
			m.arbVersion, cmd = m.arbVersion.Update(msg); cmds = append(cmds, cmd)
			m.arbAlias, cmd = m.arbAlias.Update(msg); cmds = append(cmds, cmd)
			return m, tea.Batch(cmds...)
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
		if m.creating {
			body = lipgloss.NewStyle().Bold(true).Render("New instance") + "\n\n" + m.newName.View() + "\n\n(enter to create, esc to cancel)"
		} else {
			body = m.instList.View() + "\n\n" + lipgloss.NewStyle().Faint(true).Render("m: menu")
		}
	case ScreenInstance:
		if m.addingDep {
			body = renderAddDepView(m)
		} else {
			tabsLine := renderTabs(m.tabNames, m.activeTab)
			body = tabsLine + "\n" + m.content.View() + "\n\n" + lipgloss.NewStyle().Faint(true).Render("a: add dependency")
		}
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

// Implement list.DefaultItem so bubbles/list default delegate can render it.
func (i instanceItem) Title() string { return i.Name }
func (i instanceItem) Description() string {
	if i.Path == "" {
		return ""
	}
	return i.Path
}

func renderInstanceOverview(inst instances.Instance) string {
	return fmt.Sprintf("Instance: %s\nPath: %s\n\n(Detail tabs are stubbed in v0.2 skeleton.)", inst.Name, inst.Path)
}

func renderInstanceTab(inst instances.Instance, tab int) string {
	switch tab {
	case 0:
		return renderInstanceOverview(inst)
	case 1:
		return "Deps tab: (open add dependency wizard with 'a')"
	case 2:
		return "Values tab (stub): will preview values.*.yaml and allow opening $EDITOR for values.instance.yaml."
	case 3:
		return "Presets tab (stub): will preview resolved preset files per dependency."
	default:
		return "unknown tab"
	}
}

type sourceItem string

func (s sourceItem) FilterValue() string { return string(s) }

// Implement list.DefaultItem so bubbles/list default delegate can render it.
func (s sourceItem) Title() string { return string(s) }
func (s sourceItem) Description() string { return "" }

type catalogItem catalog.Entry

// Wrap catalog.Entry (which has a `Description` field) to avoid method/field name collisions.
type catalogListItem struct{ E catalog.Entry }

func (c catalogListItem) Title() string { return c.E.ID }
func (c catalogListItem) Description() string {
	return c.E.Chart.Repo + "@" + c.E.Version
}
func (c catalogListItem) FilterValue() string {
	return c.E.ID + " " + c.E.Chart.Name + " " + c.E.Chart.Repo
}

// Wrap PackageSummary (which has a `Description` field) to avoid method/field name collisions.
type ahResultItem struct{ P artifacthub.PackageSummary }

func (a ahResultItem) Title() string {
	if a.P.DisplayName != "" {
		return a.P.DisplayName
	}
	return a.P.Name
}
func (a ahResultItem) Description() string {
	parts := []string{}
	if a.P.RepositoryName != "" {
		parts = append(parts, a.P.RepositoryName)
	}
	if a.P.LatestVersion != "" {
		parts = append(parts, "latest: "+a.P.LatestVersion)
	}
	if !a.P.LastUpdated.IsZero() {
		parts = append(parts, "updated: "+a.P.LastUpdated.Format("2006-01-02"))
	}
	if a.P.Description != "" {
		parts = append(parts, a.P.Description)
	}
	return strings.Join(parts, " • ")
}
func (a ahResultItem) FilterValue() string { return a.P.Name + " " + a.P.DisplayName + " " + a.P.RepositoryName }

type ahVersionItem artifacthub.Version

func (v ahVersionItem) Title() string { return v.Version }
func (v ahVersionItem) Description() string { return "" }
func (v ahVersionItem) FilterValue() string { return v.Version }

func depsToItems(deps []yamlchart.Dependency) []list.Item {
	items := make([]list.Item, 0, len(deps))
	for _, d := range deps {
		items = append(items, depItem(d))
	}
	return items
}

type depItem yamlchart.Dependency

func (d depItem) Title() string {
	id := yamlchart.DependencyID(yamlchart.Dependency(d))
	return string(id)
}

func (d depItem) Description() string {
	dd := yamlchart.Dependency(d)
	return dd.Repository + " • " + dd.Name + " • " + dd.Version
}

func (d depItem) FilterValue() string { return d.Title() + " " + d.Description() }

func renderAddDepView(m AppModel) string {
	header := lipgloss.NewStyle().Bold(true).Render("Add dependency")

	if m.modalErr != "" {
		errLine := lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("Error: " + m.modalErr)
		header = header + "\n" + errLine
	}

	switch m.depStep {
	case depStepChooseSource:
		return header + "\n\n" + m.depSource.View()
	case depStepCatalog:
		if len(m.catalogEntries) == 0 {
			msg := lipgloss.NewStyle().Faint(true).Render("No local catalog entries. Run `helmdex catalog sync` then retry.")
			return header + "\n\n" + msg
		}
		return header + "\n\n" + m.catalogList.View()
	case depStepAHQuery:
		return header + "\n\n" + "Artifact Hub search" + "\n\n" + m.ahQuery.View() + "\n\n(enter to search)"
	case depStepAHResults:
		return header + "\n\n" + m.ahResults.View() + "\n\n" + lipgloss.NewStyle().Faint(true).Render("enter: open details")
	case depStepAHVersions:
		return header + "\n\n" + m.ahVersions.View()
	case depStepAHDetail:
		body := renderTabs(m.ahDetailTabNames, m.ahDetailTab) + "\n"
		switch m.ahDetailTab {
		case 2:
			body += m.ahVersions.View() + "\n\n" + lipgloss.NewStyle().Faint(true).Render("enter: load README/values • a: add")
		default:
			body += m.ahPreview.View() + "\n\n" + lipgloss.NewStyle().Faint(true).Render("a: add")
		}
		return header + "\n\n" + body
	case depStepArbitrary:
		return header + "\n\n" + m.arbRepo.View() + "\n" + m.arbName.View() + "\n" + m.arbVersion.View() + "\n" + m.arbAlias.View() + "\n\n(tab to move, enter to add)"
	default:
		return header + "\n\n" + "(unknown step)"
	}
}

func (m AppModel) inputCapturesKeys() bool {
	// Any focused textinput should receive keystrokes.
	if m.creating && m.newName.Focused() {
		return true
	}
	if m.addingDep {
		if m.depStep == depStepAHQuery && m.ahQuery.Focused() {
			return true
		}
		if m.depStep == depStepArbitrary {
			if m.arbRepo.Focused() || m.arbName.Focused() || m.arbVersion.Focused() || m.arbAlias.Focused() {
				return true
			}
		}
		// If any list is filtering, it should capture typing.
		if m.catalogList.FilterState() == list.Filtering {
			return true
		}
		if m.ahResults.FilterState() == list.Filtering {
			return true
		}
		if m.depSource.FilterState() == list.Filtering {
			return true
		}
	}
	if m.instList.FilterState() == list.Filtering {
		return true
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (m AppModel) applyDependencyDraft(dep yamlchart.Dependency) tea.Cmd {
	return func() tea.Msg {
		if m.selected == nil {
			return errMsg{fmt.Errorf("no instance selected")}
		}
		chartPath := filepath.Join(m.selected.Path, "Chart.yaml")
		c, err := yamlchart.ReadChart(chartPath)
		if err != nil {
			return errMsg{err}
		}
		if err := c.UpsertDependency(dep); err != nil {
			return errMsg{err}
		}
		if err := yamlchart.WriteChart(chartPath, c); err != nil {
			return errMsg{err}
		}
		// Ensure values.yaml regenerated.
		_ = os.MkdirAll(m.selected.Path, 0o755)
		return depAppliedMsg{chart: c}
	}
}

// contextBG avoids importing context in many places; v0.2 uses Background for Artifact Hub calls.
func contextBG() context.Context { return context.Background() }
