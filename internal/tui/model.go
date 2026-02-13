package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"helmdex/internal/artifacthub"
	"helmdex/internal/catalog"
	"helmdex/internal/helmutil"
	"helmdex/internal/instances"
	"helmdex/internal/presets"
	"helmdex/internal/semverutil"
	"helmdex/internal/values"
	"helmdex/internal/yamlchart"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v3"
)

type AppModel struct {
	params Params

	screen ScreenID

	// dashboard
	instList list.Model
	insts    []instances.Instance

	// overlays
	helpOpen bool

	paletteOpen bool
	palette     paletteModel

	statusErr   string
	statusErrAt time.Time

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

	// presets resolution (computed on demand for the Presets tab)
	presetErr string

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

	// global loading indicator (status bar spinner)
	busy      int
	busyLabel string
	spin      spinner.Model

	// dependency version editor (Deps tab)
	depEditOpen         bool
	depEditDep          yamlchart.Dependency
	depEditMode         depEditMode
	depEditLoading      bool
	depEditVersions     list.Model
	depEditVersionsData []string
	depEditVersionInput textinput.Model

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

type depEditMode int

const (
	depEditModeList depEditMode = iota
	depEditModeManual
)

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
	Regen  key.Binding
	TabLeft  key.Binding
	TabRight key.Binding
	NewInstance key.Binding
	AddDep key.Binding
	Actions key.Binding // command palette
	Help    key.Binding
	EditValues key.Binding
	Apply  key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Actions, k.Help, k.Open, k.AddDep, k.EditValues, k.Apply, k.Regen, k.TabLeft, k.TabRight, k.Reload, k.Back, k.Quit}
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
		Regen:  key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "regen values")),
		TabLeft:  key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "prev tab")),
		TabRight: key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "next tab")),
		NewInstance: key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new instance")),
		AddDep: key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add dep")),
		EditValues: key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit values")),
		Apply: key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "apply")),
		Actions: key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "commands")),
		Help:    key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
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

	depVers := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	depVers.Title = "Versions"
	depVers.SetFilteringEnabled(true)
	depVers.SetShowHelp(false)

	newName := textinput.New()
	newName.Placeholder = "instance name"
	newName.Prompt = "> "

	q := textinput.New()
	q.Placeholder = "search charts (e.g. postgresql)"
	q.Prompt = "? "

	depVerInput := textinput.New()
	depVerInput.Placeholder = "exact version"
	depVerInput.Prompt = "version> "

	arbRepo := textinput.New(); arbRepo.Placeholder = "repo URL (https://... or oci://...)"; arbRepo.Prompt = "repo> "
	arbName := textinput.New(); arbName.Placeholder = "chart name"; arbName.Prompt = "name> "
	arbVersion := textinput.New(); arbVersion.Placeholder = "exact version"; arbVersion.Prompt = "version> "
	arbAlias := textinput.New(); arbAlias.Placeholder = "alias (optional)"; arbAlias.Prompt = "alias> "

	tabNames := []string{"Overview", "Deps", "Values", "Presets"}
	ahDetailTabNames := []string{"README", "Values", "Versions"}
	vp := viewport.New(0, 0)
	ahvp := viewport.New(0, 0)

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Faint(true)

	m := AppModel{
		params: p,
		screen: p.StartScreen,
		instList: l,
		depsList: deps,
		depSource: src,
		catalogList: catList,
		ahClient: artifacthub.NewClient(),
		ahQuery: q,
		ahResults: ahRes,
		ahVersions: ahVers,
		depEditVersions: depVers,
		depEditVersionInput: depVerInput,
		ahDetailTabNames: ahDetailTabNames,
		ahPreview: ahvp,
		spin: sp,
		newName: newName,
		arbRepo: arbRepo,
		arbName: arbName,
		arbVersion: arbVersion,
		arbAlias: arbAlias,
		activeTab: 0,
		tabNames:  tabNames,
		content: vp,
		palette: newPaletteModel(),
		keys: keys,
	}

	return m
}

func (m AppModel) Init() tea.Cmd {
	return tea.Batch(m.beginBusy("Reloading"), m.reloadInstancesCmd())
}

type errMsg struct{ err error }

type regenDoneMsg struct{}

type appliedMsg struct{}

type instancesMsg struct{ items []instances.Instance }

type chartMsg struct{ chart yamlchart.Chart }

type catalogMsg struct{ entries []catalog.Entry }

type ahSearchMsg struct{ results []artifacthub.PackageSummary }

type ahVersionsMsg struct{ versions []artifacthub.Version }

type ahDetailMsg struct{ readme, values string }

type depVersionsMsg struct {
	ID       yamlchart.DepID
	Versions []string
}

// noopMsg is used by background cmds to signal "success but no UI change",
// so the busy indicator can stop cleanly.
type noopMsg struct{}

// depAppliedMsg indicates a dependency draft was applied (Chart.yaml written)
// and the modal can be closed.
type depAppliedMsg struct{ chart yamlchart.Chart }

type depAppliedAndAppliedMsg struct{ chart yamlchart.Chart }

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
		ref := repoName + "/" + chartName
		// Best-effort show with minimal side effects.
		// If user requested force refresh, run repo update explicitly.
		if force {
			if err := helmutil.RepoUpdate(ctx, env); err != nil {
				// If update was killed, do not keep the UI stuck in LOADING; fall back to show attempt.
				if helmutil.IsRepoUpdateWorthRetrying(err) {
					return errMsg{err}
				}
			}
		}
		readme, err := helmutil.ShowReadmeBestEffort(ctx, env, ref, version, 24*time.Hour)
		if err != nil {
			return errMsg{err}
		}
		values, err := helmutil.ShowValuesBestEffort(ctx, env, ref, version, 24*time.Hour)
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
	// Help overlay has highest priority.
	if km, ok := msg.(tea.KeyMsg); ok && m.helpOpen {
		if km.Type == tea.KeyEsc || key.Matches(km, m.keys.Help) {
			m.helpOpen = false
			return m, nil
		}
		// Consume all input while help is open.
		return m, nil
	}

	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		if m.busy > 0 {
			return m, cmd
		}
		return m, nil
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
		m.depEditVersions.SetSize(max(10, msg.Width-6), max(5, msg.Height-12))
		m.palette.SetSize(min(70, msg.Width-4), min(14, msg.Height-6))
		// Ensure the viewport never ends up with negative size.
		if m.ahPreview.Height < 3 {
			m.ahPreview.Height = 3
		}
		return m, nil
	case instancesMsg:
		m.endBusy()
		m.insts = msg.items
		items := make([]list.Item, 0, len(msg.items))
		for _, inst := range msg.items {
			items = append(items, instanceItem(inst))
		}
		m.instList.SetItems(items)
		return m, nil
	case chartMsg:
		m.endBusy()
		m.chart = &msg.chart
		m.depsList.SetItems(depsToItems(msg.chart.Dependencies))
		m.refreshInstanceView()
		return m, nil
	case depAppliedMsg:
		m.endBusy()
		m.chart = &msg.chart
		m.depsList.SetItems(depsToItems(msg.chart.Dependencies))
		m.addingDep = false
		m.depEditOpen = false
		m.depStep = depStepNone
		m.modalErr = ""
		m.statusErr = ""
		m.refreshInstanceView()
		return m, nil
	case depAppliedAndAppliedMsg:
		m.endBusy()
		m.chart = &msg.chart
		m.depsList.SetItems(depsToItems(msg.chart.Dependencies))
		m.addingDep = false
		m.depEditOpen = false
		m.depStep = depStepNone
		m.modalErr = ""
		m.statusErr = ""
		m.refreshInstanceView()
		return m, nil
	case regenDoneMsg:
		m.endBusy()
		m.refreshInstanceView()
		return m, nil
	case appliedMsg:
		m.endBusy()
		m.refreshInstanceView()
		return m, nil
	case catalogMsg:
		m.endBusy()
		m.catalogEntries = msg.entries
		items := make([]list.Item, 0, len(msg.entries))
		for _, e := range msg.entries {
			items = append(items, catalogListItem{E: e})
		}
		m.catalogList.SetItems(items)
		return m, nil
	case ahSearchMsg:
		m.endBusy()
		m.ahResultsData = msg.results
		items := make([]list.Item, 0, len(msg.results))
		for _, r := range msg.results {
			items = append(items, ahResultItem{P: r})
		}
		m.ahResults.SetItems(items)
		m.depStep = depStepAHResults
		return m, nil
	case ahVersionsMsg:
		m.endBusy()
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
					return m, tea.Batch(m.beginBusy("Fetching chart"), m.loadHelmPreviewsCmd(m.ahSelected.RepositoryURL, m.ahSelected.Name, m.ahSelectedVersion))
				}
				m.ahLoading = false
				m.modalErr = "selected chart has no repository URL; cannot run helm show"
			}
		}
		m.ahPreview.SetContent(m.renderAHDetailBody())
		return m, nil
	case ahDetailMsg:
		m.endBusy()
		m.ahReadme = msg.readme
		m.ahValues = msg.values
		m.ahLoading = false
		m.ahPreview.SetContent(m.renderAHDetailBody())
		return m, nil
	case depVersionsMsg:
		m.endBusy()
		if !m.depEditOpen {
			return m, nil
		}
		if yamlchart.DependencyID(m.depEditDep) != msg.ID {
			return m, nil
		}
		m.depEditLoading = false
		m.depEditVersionsData = msg.Versions
		items := make([]list.Item, 0, len(msg.Versions))
		for _, v := range msg.Versions {
			items = append(items, versionItem(v))
		}
		m.depEditVersions.SetItems(items)
		// Try to keep selection on current version.
		m.depEditVersions.Select(0)
		for i := range msg.Versions {
			if strings.TrimSpace(msg.Versions[i]) == strings.TrimSpace(m.depEditDep.Version) {
				m.depEditVersions.Select(i)
				break
			}
		}
		return m, nil
	case noopMsg:
		m.endBusy()
		return m, nil
	case errMsg:
		m.endBusy()
		m.statusErr = msg.err.Error()
		m.statusErrAt = time.Now()
		// Prefer keeping the modal open when errors happen during add-dep flows.
		if m.addingDep {
			m.modalErr = msg.err.Error()
			m.ahLoading = false
			m.ahPreview.SetContent(m.renderAHDetailBody())
			return m, nil
		}
		if m.depEditOpen {
			m.modalErr = msg.err.Error()
			m.depEditLoading = false
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
		// Dep version editor modal has priority over global shortcuts.
		if m.depEditOpen {
			return m.depEditUpdate(msg)
		}
		// Command palette has priority over global shortcuts.
		if m.paletteOpen {
			return m.paletteUpdate(msg)
		}

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
				return m, tea.Batch(m.beginBusy("Refreshing"), cmd)
			}
			return m, tea.Batch(m.beginBusy("Reloading"), m.reloadInstancesCmd())
		}
		if key.Matches(msg, m.keys.Regen) {
			if m.screen == ScreenInstance && !m.addingDep {
				return m, tea.Batch(m.beginBusy("Regenerating values"), m.regenMergedValuesCmd())
			}
		}
		if key.Matches(msg, m.keys.EditValues) {
			if m.screen == ScreenInstance && !m.addingDep {
				return m, m.editInstanceValuesCmd()
			}
		}
		if key.Matches(msg, m.keys.Apply) {
			if m.screen == ScreenInstance && !m.addingDep {
				return m, tea.Batch(m.beginBusy("Applying"), m.applyInstanceCmd(false))
			}
		}
		// Deps tab actions.
		if m.screen == ScreenInstance && !m.addingDep && m.activeTab == 1 {
			// Delete dependency.
			if msg.String() == "d" || msg.String() == "D" {
				return m, tea.Batch(m.beginBusy("Updating"), m.removeSelectedDepCmd())
			}
			// Change version.
			if msg.String() == "v" || msg.String() == "V" {
				return m.openDepEditSelected()
			}
			// Upgrade to latest.
			if msg.String() == "u" || msg.String() == "U" {
				return m.upgradeSelectedDepCmd()
			}
		}
		if key.Matches(msg, m.keys.Actions) {
			m.paletteOpen = true
			m.palette.Open(m)
			return m, nil
		}
		if key.Matches(msg, m.keys.Help) {
			m.helpOpen = true
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
				return m, tea.Batch(m.beginBusy("Loading catalog"), m.loadCatalogCmd())
			}
		}
		if key.Matches(msg, m.keys.Back) {
			// First: if any filter is applied, clear it.
			if m.clearAnyAppliedFilter() {
				return m, nil
			}
			if m.creating {
				m.creating = false
				return m, nil
			}
			if m.addingDep {
				// Step-wise back inside the wizard.
				switch m.depStep {
				case depStepChooseSource:
					m.addingDep = false
					m.depStep = depStepNone
					m.modalErr = ""
					return m, nil
				case depStepCatalog, depStepAHQuery, depStepArbitrary:
					m.depStep = depStepChooseSource
					m.modalErr = ""
					return m, nil
				case depStepAHResults:
					m.depStep = depStepAHQuery
					m.ahQuery.Focus()
					m.modalErr = ""
					return m, nil
				case depStepAHVersions:
					m.depStep = depStepAHResults
					m.modalErr = ""
					return m, nil
				case depStepAHDetail:
					m.depStep = depStepAHResults
					m.modalErr = ""
					return m, nil
				default:
					m.addingDep = false
					m.depStep = depStepNone
					m.modalErr = ""
					return m, nil
				}
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
					m.refreshInstanceView()
					return m, tea.Batch(m.beginBusy("Loading chart"), m.loadChartCmd(inst))
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
				m.refreshInstanceView()
				return m, nil
			}
			if key.Matches(msg, m.keys.TabRight) {
				m.activeTab = (m.activeTab + 1) % len(m.tabNames)
				m.refreshInstanceView()
				return m, nil
			}
		}
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
			return m, tea.Batch(
				m.beginBusy("Reloading"),
				m.reloadInstancesCmd(),
				m.beginBusy("Loading chart"),
				m.loadChartCmd(inst),
			)
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
							return m, m.applyDependencyAndApplyInstanceCmd(yamlchart.Dependency{Name: m.ahSelected.Name, Repository: m.ahSelected.RepositoryURL, Version: m.ahSelectedVersion})
						}
					}
				}
				return m, cmd
			}

			// Non-versions tabs: allow quick add if a version is selected.
			if km, ok := msg.(tea.KeyMsg); ok {
				if km.String() == "a" || km.String() == "A" {
					if m.ahSelected != nil && m.ahSelectedVersion != "" {
						return m, m.applyDependencyAndApplyInstanceCmd(yamlchart.Dependency{Name: m.ahSelected.Name, Repository: m.ahSelected.RepositoryURL, Version: m.ahSelectedVersion})
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
		// Deps tab uses its own list.
		if m.activeTab == 1 && !m.addingDep {
			var cmd tea.Cmd
			m.depsList, cmd = m.depsList.Update(msg)
			return m, cmd
		}
		var cmd tea.Cmd
		m.content, cmd = m.content.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m AppModel) paletteUpdate(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Close.
	if msg.Type == tea.KeyEsc {
		m.paletteOpen = false
		return m, nil
	}
	if msg.Type == tea.KeyCtrlC {
		return m, tea.Quit
	}
	cmd, didEnter := m.palette.Update(msg)
	if didEnter {
		it, ok := m.palette.selected()
		if ok {
			switch it.ID {
			case palQuit:
				return m, tea.Quit
			case palReload:
				m.paletteOpen = false
				return m, tea.Batch(m.beginBusy("Reloading"), m.reloadInstancesCmd())
			case palNewInstance:
				m.paletteOpen = false
				m.creating = true
				m.newName.SetValue("")
				m.newName.Focus()
				return m, nil
			case palBack:
				m.paletteOpen = false
				if m.screen == ScreenInstance {
					m.screen = ScreenDashboard
					m.selected = nil
				}
				return m, nil
			case palAddDep:
				m.paletteOpen = false
				if m.screen == ScreenInstance && !m.addingDep {
					m.addingDep = true
					m.depStep = depStepChooseSource
					m.modalErr = ""
					return m, m.loadCatalogCmd()
				}
				return m, nil
			case palRegenValues:
				m.paletteOpen = false
				if m.screen == ScreenInstance {
					return m, tea.Batch(m.beginBusy("Regenerating values"), m.regenMergedValuesCmd())
				}
				return m, nil
			case palForceRefresh:
				m.paletteOpen = false
				if m.addingDep && m.depStep == depStepAHDetail && m.ahSelected != nil && m.ahSelectedVersion != "" {
					m.ahForceRefresh = true
					m.ahLoading = true
					m.modalErr = ""
					m.ahPreview.SetContent(m.renderAHDetailBody())
					c := m.loadHelmPreviewsCmd(m.ahSelected.RepositoryURL, m.ahSelected.Name, m.ahSelectedVersion)
					m.ahForceRefresh = false
					return m, tea.Batch(m.beginBusy("Refreshing"), c)
				}
				return m, nil
			}
		}
	}
	return m, cmd
}

func (m AppModel) isAnyFilterActive() bool {
	if m.instList.FilterState() != list.Unfiltered {
		return true
	}
	if m.catalogList.FilterState() != list.Unfiltered {
		return true
	}
	if m.depSource.FilterState() != list.Unfiltered {
		return true
	}
	return false
}

func (m AppModel) View() string {
	base := lipgloss.NewStyle().Padding(1, 1)

	header := lipgloss.NewStyle().Bold(true).Render("helmdex")
	if m.params.RepoRoot != "" {
		header += "  " + lipgloss.NewStyle().Faint(true).Render(m.params.RepoRoot)
	}

	var body string
	if m.helpOpen {
		body = renderHelpOverlay(m)
	} else if m.paletteOpen {
		body = renderWithModal(m, m.currentBodyView(), m.palette.View())
	} else if m.depEditOpen {
		body = renderWithModal(m, m.currentBodyView(), renderDepEditModal(m))
	} else {
		body = m.currentBodyView()
	}

	contextHelp := lipgloss.NewStyle().Faint(true).Render(m.contextHelpLine())
	status := renderStatusBar(m)

	return base.Render(strings.TrimRight(header+"\n\n"+body+"\n\n"+contextHelp+"\n"+status, "\n"))
}

func (m AppModel) currentBodyView() string {
	switch m.screen {
	case ScreenDashboard:
		if m.creating {
			return lipgloss.NewStyle().Bold(true).Render("New instance") + "\n\n" + m.newName.View() + "\n\n(enter to create, esc to cancel)"
		}
		return m.instList.View()
	case ScreenInstance:
		if m.addingDep {
			return renderAddDepView(m)
		}
		tabsLine := renderTabs(m.tabNames, m.activeTab)
		if m.activeTab == 1 {
			return tabsLine + "\n" + m.depsList.View() + "\n\n" + lipgloss.NewStyle().Faint(true).Render("d: remove selected")
		}
		return tabsLine + "\n" + m.content.View()
	default:
		return "unknown screen"
	}
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

func (m AppModel) contextHelpLine() string {
	if m.helpOpen {
		return "esc/? close help"
	}
	if m.paletteOpen {
		return "type to search • ↑/↓ select • enter run • esc close"
	}
	if m.creating {
		return "enter create • esc cancel"
	}
	if m.screen == ScreenDashboard {
		return "/ filter • enter open • n new • m commands • q quit"
	}
	if m.screen == ScreenInstance {
		if m.addingDep {
			return "esc close • enter select • ←/→ tabs • a add"
		}
		if m.activeTab == 1 {
			return "←/→ tabs • d remove • v version • u upgrade • a add dep • m commands • esc back • q quit"
		}
		return "←/→ tabs • a add dep • e edit values • p apply • r regen values • m commands • esc back • q quit"
	}
	return shortHelp(m.keys)
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
	return fmt.Sprintf("Instance: %s\nPath: %s", inst.Name, inst.Path)
}

func renderInstanceTab(inst instances.Instance, tab int) string {
	switch tab {
	case 0:
		return renderInstanceOverview(inst)
	case 1:
		return "Dependencies (press 'a' to add)"
	case 2:
		return "Values"
	case 3:
		return "Presets"
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

type versionItem string

func (v versionItem) Title() string       { return string(v) }
func (v versionItem) Description() string { return "" }
func (v versionItem) FilterValue() string { return string(v) }

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
	if m.paletteOpen && m.palette.QueryFocused() {
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
	if m.depEditOpen {
		if m.depEditMode == depEditModeManual && m.depEditVersionInput.Focused() {
			return true
		}
		if m.depEditMode == depEditModeList && m.depEditVersions.FilterState() == list.Filtering {
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
	if m.depsList.FilterState() == list.Filtering {
		return true
	}
	return false
}

func (m *AppModel) beginBusy(label string) tea.Cmd {
	m.busy++
	if strings.TrimSpace(label) != "" {
		m.busyLabel = label
	}
	if m.busy == 1 {
		return m.spin.Tick
	}
	return nil
}

func (m *AppModel) endBusy() {
	if m.busy > 0 {
		m.busy--
	}
	if m.busy == 0 {
		m.busyLabel = ""
	}
}

func (m *AppModel) clearAnyAppliedFilter() bool {
	cleared := false
	if m.instList.FilterState() == list.FilterApplied {
		m.instList.ResetFilter()
		cleared = true
	}
	if m.catalogList.FilterState() == list.FilterApplied {
		m.catalogList.ResetFilter()
		cleared = true
	}
	if m.depSource.FilterState() == list.FilterApplied {
		m.depSource.ResetFilter()
		cleared = true
	}
	return cleared
}

func (m AppModel) depEditUpdate(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEsc {
		m.depEditOpen = false
		m.depEditLoading = false
		m.modalErr = ""
		m.depEditDep = yamlchart.Dependency{}
		m.depEditVersionInput.Blur()
		return m, nil
	}

	if m.depEditMode == depEditModeManual {
		var cmd tea.Cmd
		m.depEditVersionInput, cmd = m.depEditVersionInput.Update(msg)
		if msg.Type == tea.KeyEnter {
			v := strings.TrimSpace(m.depEditVersionInput.Value())
			if v == "" {
				m.modalErr = "version is required"
				return m, nil
			}
			dep := m.depEditDep
			dep.Version = v
			m.depEditOpen = false
			m.depEditVersionInput.Blur()
			return m, tea.Batch(m.beginBusy("Applying"), m.applyDependencyAndApplyInstanceCmd(dep))
		}
		return m, cmd
	}

	var cmd tea.Cmd
	m.depEditVersions, cmd = m.depEditVersions.Update(msg)
	if msg.Type == tea.KeyEnter {
		it := m.depEditVersions.SelectedItem()
		if it == nil {
			return m, cmd
		}
		vi, ok := it.(versionItem)
		if !ok {
			return m, func() tea.Msg { return errMsg{fmt.Errorf("unexpected version item type")} }
		}
		dep := m.depEditDep
		dep.Version = string(vi)
		m.depEditOpen = false
		return m, tea.Batch(m.beginBusy("Applying"), m.applyDependencyAndApplyInstanceCmd(dep))
	}
	return m, cmd
}

func (m AppModel) openDepEditSelected() (tea.Model, tea.Cmd) {
	it := m.depsList.SelectedItem()
	if it == nil {
		return m, nil
	}
	di, ok := it.(depItem)
	if !ok {
		return m, func() tea.Msg { return errMsg{fmt.Errorf("unexpected dependency item type")} }
	}
	dep := yamlchart.Dependency(di)

	m.depEditOpen = true
	m.depEditDep = dep
	m.depEditLoading = false
	m.modalErr = ""
	m.depEditVersionsData = nil
	m.depEditVersions.SetItems(nil)
	m.depEditVersions.Title = fmt.Sprintf("Versions (%s)", yamlchart.DependencyID(dep))

	// OCI: cannot query versions; use manual exact version input.
	if strings.HasPrefix(dep.Repository, "oci://") {
		m.depEditMode = depEditModeManual
		m.depEditVersionInput.SetValue(dep.Version)
		m.depEditVersionInput.Focus()
		return m, nil
	}

	m.depEditMode = depEditModeList
	m.depEditLoading = true
	return m, tea.Batch(m.beginBusy("Loading versions"), m.loadDepVersionsCmd(dep))
}

func (m AppModel) loadDepVersionsCmd(dep yamlchart.Dependency) tea.Cmd {
	return func() tea.Msg {
		// Allow enough time for a first `helm search repo` attempt, and (if needed)
		// one stale-aware `helm repo update` + retry.
		ctx, cancel := context.WithTimeout(contextBG(), 60*time.Second)
		defer cancel()
		vs, err := helmutil.RepoChartVersions(ctx, m.params.RepoRoot, dep.Repository, dep.Name, 24*time.Hour)
		if err != nil {
			return errMsg{err}
		}
		return depVersionsMsg{ID: yamlchart.DependencyID(dep), Versions: vs}
	}
}

func (m AppModel) upgradeSelectedDepCmd() (tea.Model, tea.Cmd) {
	it := m.depsList.SelectedItem()
	if it == nil {
		return m, nil
	}
	di, ok := it.(depItem)
	if !ok {
		return m, func() tea.Msg { return errMsg{fmt.Errorf("unexpected dependency item type")} }
	}
	dep := yamlchart.Dependency(di)
	return m, tea.Batch(m.beginBusy("Upgrading"), m.upgradeDepToLatestCmd(dep))
}

func (m AppModel) upgradeDepToLatestCmd(dep yamlchart.Dependency) tea.Cmd {
	return func() tea.Msg {
		if strings.HasPrefix(dep.Repository, "oci://") {
			return errMsg{fmt.Errorf("cannot auto-upgrade OCI dependency %s; use v to set exact version", yamlchart.DependencyID(dep))}
		}
		ctx, cancel := context.WithTimeout(contextBG(), 75*time.Second)
		defer cancel()
		vs, err := helmutil.RepoChartVersions(ctx, m.params.RepoRoot, dep.Repository, dep.Name, 24*time.Hour)
		if err != nil {
			return errMsg{err}
		}
		best, ok := semverutil.BestStable(vs)
		if !ok {
			return errMsg{fmt.Errorf("no stable SemVer versions found for %s", yamlchart.DependencyID(dep))}
		}
		if strings.TrimSpace(best) == strings.TrimSpace(dep.Version) {
			return noopMsg{}
		}
		dep.Version = best
		return m.applyDependencyAndApplyInstanceCmd(dep)()
	}
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

// applyDependencyAndApplyInstanceCmd writes Chart.yaml and then immediately
// applies the instance (relock + presets import + values regen) so Chart.lock
// is generated right away.
func (m AppModel) applyDependencyAndApplyInstanceCmd(dep yamlchart.Dependency) tea.Cmd {
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

		// Apply pipeline.
		if _, err := instances.RelockIfDepsChanged(contextBG(), m.params.RepoRoot, m.selected.Path); err != nil {
			return errMsg{err}
		}
		if m.params.Config != nil {
			_, err = presets.Import(presets.ImportParams{RepoRoot: m.params.RepoRoot, InstancePath: m.selected.Path, Config: *m.params.Config, Dependencies: c.Dependencies})
			if err != nil {
				return errMsg{err}
			}
		}
		if err := values.GenerateMergedValues(m.selected.Path); err != nil {
			return errMsg{err}
		}
		return depAppliedAndAppliedMsg{chart: c}
	}
}

func (m AppModel) regenMergedValuesCmd() tea.Cmd {
	return func() tea.Msg {
		if m.selected == nil {
			return errMsg{fmt.Errorf("no instance selected")}
		}
		if err := values.GenerateMergedValues(m.selected.Path); err != nil {
			return errMsg{err}
		}
		return regenDoneMsg{}
	}
}

func (m AppModel) editInstanceValuesCmd() tea.Cmd {
	return func() tea.Msg {
		if m.selected == nil {
			return errMsg{fmt.Errorf("no instance selected")}
		}
		editor := os.Getenv("EDITOR")
		if strings.TrimSpace(editor) == "" {
			editor = "vi"
		}
		path := filepath.Join(m.selected.Path, "values.instance.yaml")
		cmd := exec.Command(editor, path)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return errMsg{fmt.Errorf("editor failed: %w", err)}
		}
		return nil
	}
}

func (m AppModel) applyInstanceCmd(forceRelock bool) tea.Cmd {
	return func() tea.Msg {
		if m.selected == nil {
			return errMsg{fmt.Errorf("no instance selected")}
		}
		// Optional relock.
		if forceRelock {
			if err := instances.RelockDependencies(contextBG(), m.params.RepoRoot, m.selected.Path); err != nil {
				return errMsg{err}
			}
		} else {
			if _, err := instances.RelockIfDepsChanged(contextBG(), m.params.RepoRoot, m.selected.Path); err != nil {
				return errMsg{err}
			}
		}
		c, err := yamlchart.ReadChart(filepath.Join(m.selected.Path, "Chart.yaml"))
		if err != nil {
			return errMsg{err}
		}
		if m.params.Config != nil {
			_, err = presets.Import(presets.ImportParams{RepoRoot: m.params.RepoRoot, InstancePath: m.selected.Path, Config: *m.params.Config, Dependencies: c.Dependencies})
			if err != nil {
				return errMsg{err}
			}
		}
		if err := values.GenerateMergedValues(m.selected.Path); err != nil {
			return errMsg{err}
		}
		return appliedMsg{}
	}
}

func (m *AppModel) refreshInstanceView() {
	if m.selected == nil {
		return
	}
	switch m.activeTab {
	case 0:
		m.content.SetContent(m.renderOverviewTab())
	case 2:
		m.content.SetContent(m.renderValuesTab())
	case 3:
		m.content.SetContent(m.renderPresetsTab())
	default:
		m.content.SetContent(renderInstanceTab(*m.selected, m.activeTab))
	}
}

func (m AppModel) renderOverviewTab() string {
	inst := *m.selected
	lines := []string{renderInstanceOverview(inst), ""}
	// Chart summary.
	if m.chart != nil {
		lines = append(lines, fmt.Sprintf("Chart: %s (%s)\nVersion: %s", m.chart.Name, m.chart.APIVersion, m.chart.Version))
		lines = append(lines, fmt.Sprintf("Dependencies: %d", len(m.chart.Dependencies)))
		for _, d := range m.chart.Dependencies {
			lines = append(lines, fmt.Sprintf("- %s: %s @ %s", yamlchart.DependencyID(d), d.Version, d.Repository))
		}
		lines = append(lines, "")
	}
	// Local set files.
	setFiles, _ := filepath.Glob(filepath.Join(inst.Path, "values.set.*.yaml"))
	if len(setFiles) == 0 {
		lines = append(lines, "Sets: (none)")
	} else {
		s := []string{}
		for _, f := range setFiles {
			s = append(s, filepath.Base(f))
		}
		sort.Strings(s)
		lines = append(lines, "Sets:")
		for _, f := range s {
			lines = append(lines, "- "+f)
		}
	}
	lines = append(lines, "")
	// Sync meta.
	if m.params.Config != nil && len(m.params.Config.Sources) > 0 {
		lines = append(lines, "Sources:")
		for _, src := range m.params.Config.Sources {
			metaPath := filepath.Join(m.params.RepoRoot, ".helmdex", "cache", src.Name, ".helmdex-meta.yaml")
			commit := readResolvedCommit(metaPath)
			if commit == "" {
				commit = "(not synced)"
			}
			lines = append(lines, fmt.Sprintf("- %s: %s", src.Name, commit))
		}
	}
	return strings.Join(lines, "\n")
}

func readResolvedCommit(metaPath string) string {
	b, err := os.ReadFile(metaPath)
	if err != nil {
		return ""
	}
	var m map[string]any
	if err := yaml.Unmarshal(b, &m); err != nil {
		return ""
	}
	if v, ok := m["resolvedCommit"].(string); ok {
		if len(v) > 12 {
			return v[:12]
		}
		return v
	}
	return ""
}

func (m AppModel) renderValuesTab() string {
	if m.selected == nil {
		return ""
	}
	inst := *m.selected
	paths := []string{
		"values.default.yaml",
		"values.platform.yaml",
		"values.instance.yaml",
		"values.yaml",
	}
	setFiles, _ := filepath.Glob(filepath.Join(inst.Path, "values.set.*.yaml"))
	sort.Strings(setFiles)
	lines := []string{"Values files:", ""}
	for _, rel := range paths {
		p := filepath.Join(inst.Path, rel)
		if _, err := os.Stat(p); err == nil {
			lines = append(lines, "- "+rel)
		}
	}
	for _, p := range setFiles {
		lines = append(lines, "- "+filepath.Base(p))
	}
	lines = append(lines, "", "Actions:", "- e: edit values.instance.yaml", "- r: regenerate values.yaml")
	return strings.Join(lines, "\n")
}

func (m AppModel) renderPresetsTab() string {
	if m.selected == nil {
		return ""
	}
	if m.params.Config == nil {
		return "No config loaded; cannot resolve presets."
	}
	if m.chart == nil {
		return "Chart not loaded yet."
	}
	res, err := presets.Resolve(m.params.RepoRoot, *m.params.Config, m.chart.Dependencies)
	if err != nil {
		return "Preset resolution error: " + err.Error()
	}
	lines := []string{"Resolved presets:", ""}
	ids := make([]string, 0, len(res.ByID))
	for id := range res.ByID {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)
	for _, raw := range ids {
		rd := res.ByID[yamlchart.DepID(raw)]
		lines = append(lines, fmt.Sprintf("%s (%s)", raw, rd.Dependency.Version))
		if rd.DefaultPath != "" {
			lines = append(lines, "  default:  "+rd.DefaultPath)
		}
		if rd.PlatformPath != "" {
			lines = append(lines, "  platform: "+rd.PlatformPath)
		}
		// Sets
		setNames := make([]string, 0, len(rd.SetPaths))
		for s := range rd.SetPaths {
			setNames = append(setNames, s)
		}
		sort.Strings(setNames)
		for _, s := range setNames {
			lines = append(lines, "  set "+s+": "+rd.SetPaths[s])
		}
		lines = append(lines, "")
	}
	lines = append(lines, "Actions:", "- p: apply (import preset layers + regenerate values.yaml)")
	return strings.Join(lines, "\n")
}

func (m AppModel) removeSelectedDepCmd() tea.Cmd {
	return func() tea.Msg {
		if m.selected == nil {
			return errMsg{fmt.Errorf("no instance selected")}
		}
		it := m.depsList.SelectedItem()
		if it == nil {
			return errMsg{fmt.Errorf("no dependency selected")}
		}
		di, ok := it.(depItem)
		if !ok {
			return errMsg{fmt.Errorf("unexpected dependency item type")}
		}
		id := yamlchart.DepID(di.Title())
		chartPath := filepath.Join(m.selected.Path, "Chart.yaml")
		c, err := yamlchart.ReadChart(chartPath)
		if err != nil {
			return errMsg{err}
		}
		if ok := c.RemoveDependencyByID(id); !ok {
			return errMsg{fmt.Errorf("dependency %q not found", id)}
		}
		if err := yamlchart.WriteChart(chartPath, c); err != nil {
			return errMsg{err}
		}
		return depAppliedMsg{chart: c}
	}
}

// contextBG avoids importing context in many places; v0.2 uses Background for Artifact Hub calls.
func contextBG() context.Context { return context.Background() }
