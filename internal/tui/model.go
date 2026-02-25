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
	"helmdex/internal/config"
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
	statusOK    string

	// create instance
	creating bool
	newName  textinput.Model

	// instance manage (Instance tab)
	instanceManageOpen   bool
	instanceManageMode   instanceManageMode
	instanceManageName   textinput.Model
	instanceManageConfirm bool

	// instance detail
	selected  *instances.Instance
	activeTab int
	tabNames  []string
	content   viewport.Model

	// values tab (file list + preview modal)
	valuesList        list.Model
	valuesPreviewOpen bool
	valuesPreviewPath string
	valuesPreview     viewport.Model

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

	// blocking apply overlay (for long-running add+apply flows)
	applyOpen           bool
	applyCancelConfirm  bool
	applyCancelRequested bool
	applyCancel         context.CancelFunc
	applyID             int

	// catalog picker
	catalogList    list.Model
	catalogEntries []catalog.EntryWithSource
	// catalog wizard UX helpers
	catalogWizardAutoSyncTried bool
	catalogWizardSyncing       bool

	// catalog detail (sets selection)
	catalogDetailEntry *catalog.EntryWithSource
	catalogSetList     list.Model
	catalogSetsLoading bool

	// catalog collision resolution (same dependency name already exists)
	catalogCollisionDep      yamlchart.Dependency
	catalogCollisionExisting yamlchart.Dependency
	catalogCollisionSets     []string
	catalogCollisionChoice   collisionChoice
	catalogCollisionAlias    textinput.Model

	// artifacthub picker
	ahClient          *artifacthub.Client
	ahQuery           textinput.Model
	ahResults         list.Model
	ahResultsData     []artifacthub.PackageSummary
	ahVersions        list.Model
	ahVersionsData    []artifacthub.Version
	ahSelected        *artifacthub.PackageSummary
	ahSelectedVersion string
	ahDetailTab       int
	ahDetailTabNames  []string
	ahReadme          string
	ahValues          string
	ahLoading         bool
	ahPreview         viewport.Model
	ahForceRefresh    bool

	// global loading indicator (status bar spinner)
	busy      int
	busyLabel string
	spin      spinner.Model

	// dependency version editor (Deps tab)
	depEditOpen bool
	depEditDep  yamlchart.Dependency
	depEditMode depEditMode
	// depEditLoading indicates a background refresh is running; the list may still
	// be usable if we have cached versions.
	depEditLoading      bool
	depEditVersions     list.Model
	depEditVersionsData []string
	depEditVersionInput textinput.Model

	// dependency actions menu (Deps tab)
	depActionsOpen bool
	depActionsDep  yamlchart.Dependency
	depActionsSource   depSourceMeta
	depActionsSourceOK bool
	depActionsList list.Model

	// dependency detail modal (Deps tab)
	depDetailOpen     bool
	depDetailDep      yamlchart.Dependency
	depDetailSource   depSourceMeta
	depDetailSourceOK bool
	depDetailTab      int
	depDetailTabNames []string
	depDetailTabKinds []depDetailTabKind
	depDetailLoading  bool
	depDetailMode     depEditMode // reuse enum: list vs manual
	depDetailAliasInput     textinput.Model
	depDetailDeleteConfirm  bool
	// depDetailSetsLoading is specific to the Sets tab.
	depDetailSetsLoading bool
	depDetailSets        list.Model
	// depDetailVersionsLoading is specific to the Versions tab, so other tabs can
	// remain interactive while versions refreshes.
	depDetailVersionsLoading bool
	depDetailVersions        list.Model
	depDetailVersionsData    []string
	depDetailVersionInput    textinput.Model // OCI/manual fallback
	depDetailReadme          string
	depDetailDefaultValues   string
	depDetailSchemaRaw       string
	depDetailPreview         viewport.Model
	depDetailPendingVersion  string
	depConfigure             depConfigureModel

	// versions refresh (disk cache + periodic background refresh)
	versionsWatched  map[string]versionsWatch
	versionsInFlight map[string]bool

	// arbitrary
	arbRepo    textinput.Model
	arbName    textinput.Model
	arbVersion textinput.Model
	arbAlias   textinput.Model
	arbFocus   int

	width  int
	height int

	// lastWindowTitle is used to avoid emitting redundant window-title updates.
	lastWindowTitle string

	// skipWindowTitleOnce is set for returns where we don't want to batch window
	// title updates with the command (notably: quit commands, so tests and Bubble
	// Tea behavior remain predictable).
	skipWindowTitleOnce bool

	keys keyMap

	// sources config modal
	sourcesOpen bool
	sourcesName textinput.Model
	sourcesGit  textinput.Model
	sourcesRef  textinput.Model
	sourcesPlat textinput.Model
	sourcesFocus int
	sourcesErr  string
}

func (m *AppModel) setStatusErr(msg string) {
	msg = strings.TrimSpace(msg)
	m.statusErr = msg
	if msg != "" {
		// Errors take precedence over OK status.
		m.statusOK = ""
	}
}

func (m *AppModel) clearStatusErr() { m.statusErr = "" }

func (m *AppModel) setStatusOK(msg string) {
	msg = strings.TrimSpace(msg)
	m.statusOK = msg
	if msg != "" {
		// A successful operation clears any previous error.
		m.statusErr = ""
	}
}

func (m AppModel) noModalOpen() bool {
	// Keep this conservative: any overlay/modal-like state counts as a modal.
	return !m.helpOpen &&
		!m.paletteOpen &&
		!m.sourcesOpen &&
		!m.instanceManageOpen &&
		!m.depActionsOpen &&
		!m.depDetailOpen &&
		!m.depEditOpen &&
		!m.valuesPreviewOpen &&
		!m.applyOpen
}

func hasCatalogEnabledSources(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	for _, src := range cfg.Sources {
		if src.Catalog.Enabled {
			return true
		}
	}
	return false
}

func (m *AppModel) openSourcesModal() {
	m.paletteOpen = false
	m.sourcesOpen = true
	m.sourcesErr = ""
	m.sourcesFocus = 0
	// Prefill from config when available (first source).
	if m.params.Config != nil {
		m.sourcesPlat.SetValue(m.params.Config.Platform.Name)
		if len(m.params.Config.Sources) > 0 {
			m.sourcesName.SetValue(m.params.Config.Sources[0].Name)
			m.sourcesGit.SetValue(m.params.Config.Sources[0].Git.URL)
			m.sourcesRef.SetValue(m.params.Config.Sources[0].Git.Ref)
		} else {
			m.sourcesName.SetValue("")
			m.sourcesGit.SetValue("")
			m.sourcesRef.SetValue("")
		}
	} else {
		m.sourcesName.SetValue("")
		m.sourcesGit.SetValue("")
		m.sourcesRef.SetValue("")
		m.sourcesPlat.SetValue("")
	}
	m.sourcesName.Focus()
	m.sourcesGit.Blur()
	m.sourcesRef.Blur()
	m.sourcesPlat.Blur()
}

// Instance tabs (ScreenInstance).
const (
	InstanceTabDeps = iota
	InstanceTabValues
	InstanceTabInstance
)

func instanceTabNames() []string {
	// Centralized tab order definition.
	return []string{
		withIcon(iconDeps, "Dependencies"),
		withIcon(iconValues, "Values"),
		withIcon(iconSettings, "Instance"),
	}
}

type instanceManageMode int

const (
	instanceManageRename instanceManageMode = iota
	instanceManageDelete
)

// depDetailTabs returns the dynamic tabs list for the dependency detail modal.
// Catalog-backed deps get Sets as the first tab.
func depDetailTabs(source depSourceMeta, ok bool) (names []string, kinds []depDetailTabKind) {
	// Base (non-catalog) order.
	kinds = []depDetailTabKind{depDetailTabValues, depDetailTabDependency, depDetailTabDefault, depDetailTabReadme, depDetailTabVersions}
	if ok && source.Kind == depSourceCatalog {
		kinds = append([]depDetailTabKind{depDetailTabSets}, kinds...)
	}

	names = make([]string, 0, len(kinds))
	for _, k := range kinds {
		switch k {
		case depDetailTabSets:
			names = append(names, withIcon(iconPresets, "Sets"))
		case depDetailTabValues:
			names = append(names, withIcon(iconSchema, "Configure"))
		case depDetailTabDependency:
			names = append(names, withIcon(iconDeps, "Settings"))
		case depDetailTabDefault:
			names = append(names, withIcon(iconAHValues, "Default"))
		case depDetailTabReadme:
			names = append(names, withIcon(iconReadme, "README"))
		case depDetailTabVersions:
			names = append(names, withIcon(iconVersions, "Versions"))
		default:
			names = append(names, "")
		}
	}
	return names, kinds
}

type depEditMode int

const (
	depEditModeList depEditMode = iota
	depEditModeManual
)

// depDetailTabKind is the semantic tab identity for the dependency detail modal.
// We keep tab order dynamic, so logic should not rely on fixed numeric indices.
type depDetailTabKind int

const (
	depDetailTabSets depDetailTabKind = iota
	depDetailTabValues
	depDetailTabDependency
	depDetailTabDefault
	depDetailTabReadme
	depDetailTabVersions
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

type depActionItem string

func (a depActionItem) Title() string       { return string(a) }
func (a depActionItem) Description() string { return "" }
func (a depActionItem) FilterValue() string { return string(a) }

const (
	depActionSyncPresets  depActionItem = "Sync presets"
	depActionChangeVer    depActionItem = "Change version"
	depActionUpgrade      depActionItem = "Upgrade to latest"
	depActionRemove       depActionItem = "Remove"
)

type depWizardStep int

const (
	depStepNone depWizardStep = iota
	depStepChooseSource
	depStepCatalog
	depStepCatalogDetail
	depStepCatalogCollision
	depStepAHQuery
	depStepAHResults
	depStepAHVersions
	depStepAHDetail
	depStepArbitrary
)

type applyDoneMsg struct {
	applyID int
	chart   *yamlchart.Chart
	err     error
}

type applyCancelDoneMsg struct {
	applyID int
}

type collisionChoice int

const (
	collisionChoiceAlias collisionChoice = iota
	collisionChoiceOverride
	collisionChoiceCancel
)

type catalogSetsMsg struct {
	entry catalog.EntryWithSource
	sets  []setChoice
	err   error
}

type depDetailSetsMsg struct {
	ID   yamlchart.DepID
	Sets []setChoice
	Err  error
}

type setChoice struct {
	Name    string
	Default bool
	On      bool
}

type setChoiceItem struct{ C setChoice }

func (s setChoiceItem) Title() string {
	box := "[ ]"
	if s.C.On {
		box = "[x]"
	}
	if s.C.Default {
		return box + " " + s.C.Name + " " + styleMuted.Render("(default)")
	}
	return box + " " + s.C.Name
}
func (s setChoiceItem) Description() string { return "" }
func (s setChoiceItem) FilterValue() string { return s.C.Name }

// setChoiceForDepItem is used in the dependency detail Sets tab.
// It reuses the same underlying setChoice struct but keeps it separate so
// we can evolve rendering/interaction independently from the catalog wizard.
type setChoiceForDepItem struct{ C setChoice }

func (s setChoiceForDepItem) Title() string {
	box := "[ ]"
	if s.C.On {
		box = "[x]"
	}
	if s.C.Default {
		return box + " " + s.C.Name + " " + styleMuted.Render("(default)")
	}
	return box + " " + s.C.Name
}
func (s setChoiceForDepItem) Description() string { return "" }
func (s setChoiceForDepItem) FilterValue() string { return s.C.Name }

type keyMap struct {
	Quit        key.Binding
	Back        key.Binding
	Open        key.Binding
	Reload      key.Binding
	Regen       key.Binding
	TabLeft     key.Binding
	TabRight    key.Binding
	NewInstance key.Binding
	AddDep      key.Binding
	Actions     key.Binding // command palette
	Help        key.Binding
	EditValues  key.Binding
	Apply       key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Actions, k.Help, k.Open, k.AddDep, k.EditValues, k.Apply, k.Regen, k.TabLeft, k.TabRight, k.Reload, k.Back, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Open, k.Reload}, {k.Back, k.Quit}}
}

func NewAppModel(p Params) AppModel {
	keys := keyMap{
		Quit:        key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		Back:        key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Open:        key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		Reload:      key.NewBinding(key.WithKeys("ctrl+r", "f5"), key.WithHelp("ctrl+r", "reload")),
		Regen:       key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "regen values")),
		TabLeft:     key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "prev tab")),
		TabRight:    key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "next tab")),
		NewInstance: key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new instance")),
		AddDep:      key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add dep")),
		EditValues:  key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit values")),
		Apply:       key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "apply")),
		Actions:     key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "commands")),
		Help:        key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	}

	l := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Instances"
	l.SetShowHelp(false)
	l.SetFilteringEnabled(true)

	deps := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	// Title disabled: we render a consistent per-tab heading row in the instance view.
	// Keeping the list title would look like the tab bar changes when switching to Deps.
	deps.Title = ""
	deps.SetShowTitle(false)
	deps.SetShowHelp(false)

	vals := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	vals.Title = ""
	vals.SetShowTitle(false)
	vals.SetShowHelp(false)
	vals.SetFilteringEnabled(true)

	catSets := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	catSets.Title = ""
	catSets.SetShowTitle(false)
	catSets.SetShowHelp(false)
	catSets.SetFilteringEnabled(false)

	depSets := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	depSets.Title = ""
	depSets.SetShowTitle(false)
	depSets.SetShowHelp(false)
	depSets.SetFilteringEnabled(false)

	src := list.New([]list.Item{sourceItem("Predefined catalog"), sourceItem("Artifact Hub"), sourceItem("Arbitrary")}, list.NewDefaultDelegate(), 0, 0)
	src.Title = withIcon(iconWizard, "Select source")
	src.SetShowHelp(false)

	catList := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	catList.Title = withIcon(iconCatalog, "Catalog")
	catList.SetFilteringEnabled(true)
	catList.SetShowHelp(false)

	ahRes := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	ahRes.Title = withIcon(iconAH, "Artifact Hub results")
	// Filtering here makes Enter ambiguous (it can apply filter instead of selecting).
	// We rely on the dedicated query input instead.
	ahRes.SetFilteringEnabled(false)
	ahRes.SetShowHelp(false)

	ahVers := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	ahVers.Title = withIcon(iconVersions, "Select version")
	ahVers.SetShowHelp(false)

	depVers := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	depVers.Title = withIcon(iconVersions, "Versions")
	depVers.SetFilteringEnabled(true)
	depVers.SetShowHelp(false)

	depActions := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	depActions.Title = withIcon(iconCmd, "Dependency actions")
	depActions.SetShowHelp(false)
	depActions.SetFilteringEnabled(false)

	newName := textinput.New()
	newName.Placeholder = "instance name"
	newName.Prompt = "> "

	instManageName := textinput.New()
	instManageName.Placeholder = "new instance name"
	instManageName.Prompt = "name> "

	srcName := textinput.New()
	srcName.Placeholder = "source name (e.g. example)"
	srcName.Prompt = "name> "

	srcGit := textinput.New()
	srcGit.Placeholder = "git URL/path (e.g. /tmp/... or https://...)"
	srcGit.Prompt = "git> "

	srcRef := textinput.New()
	srcRef.Placeholder = "git ref (optional, e.g. main, v1.2.3, HEAD~1)"
	srcRef.Prompt = "ref> "

	srcPlat := textinput.New()
	srcPlat.Placeholder = "platform name (e.g. eks)"
	srcPlat.Prompt = "platform> "

	collAlias := textinput.New()
	collAlias.Placeholder = "alias (required)"
	collAlias.Prompt = "alias> "

	q := textinput.New()
	q.Placeholder = "search charts (e.g. postgresql)"
	q.Prompt = "? "

	depVerInput := textinput.New()
	depVerInput.Placeholder = "exact version"
	depVerInput.Prompt = "version> "

	depDetailVersions := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	depDetailVersions.Title = ""
	depDetailVersions.SetShowTitle(false)
	depDetailVersions.SetFilteringEnabled(true)
	depDetailVersions.SetShowHelp(false)

	depDetailVerInput := textinput.New()
	depDetailVerInput.Placeholder = "exact version"
	depDetailVerInput.Prompt = "version> "

	depDetailAlias := textinput.New()
	depDetailAlias.Placeholder = "alias (optional)"
	depDetailAlias.Prompt = "alias> "

	arbRepo := textinput.New()
	arbRepo.Placeholder = "repo URL (https://... or oci://...)"
	arbRepo.Prompt = "repo> "
	arbName := textinput.New()
	arbName.Placeholder = "chart name"
	arbName.Prompt = "name> "
	arbVersion := textinput.New()
	arbVersion.Placeholder = "exact version"
	arbVersion.Prompt = "version> "
	arbAlias := textinput.New()
	arbAlias.Placeholder = "alias (optional)"
	arbAlias.Prompt = "alias> "

	tabNames := instanceTabNames()
	ahDetailTabNames := []string{
		withIcon(iconReadme, "README"),
		withIcon(iconAHValues, "Configure"),
		withIcon(iconVersions, "Versions"),
	}
	depDetailTabNames, depDetailTabKinds := depDetailTabs(depSourceMeta{}, false)
	vp := viewport.New(0, 0)
	ahvp := viewport.New(0, 0)
	depDetailVP := viewport.New(0, 0)
	valsPrev := viewport.New(0, 0)
	valsPrev.SetContent("No file selected")

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Faint(true)

	m := AppModel{
		params:                p,
		screen:                p.StartScreen,
		instList:              l,
		depsList:              deps,
		depSource:             src,
		catalogList:           catList,
		ahClient:              artifacthub.NewClient(),
		ahQuery:               q,
		ahResults:             ahRes,
		ahVersions:            ahVers,
		depEditVersions:       depVers,
		depActionsList:        depActions,
		depEditVersionInput:   depVerInput,
		ahDetailTabNames:      ahDetailTabNames,
		ahPreview:             ahvp,
		depDetailTabNames:     depDetailTabNames,
		depDetailTabKinds:     depDetailTabKinds,
		depDetailVersions:     depDetailVersions,
		depDetailVersionInput: depDetailVerInput,
		depDetailAliasInput:   depDetailAlias,
		depDetailPreview:      depDetailVP,
		spin:                  sp,
		newName:               newName,
		instanceManageName:    instManageName,
		arbRepo:               arbRepo,
		arbName:               arbName,
		arbVersion:            arbVersion,
		arbAlias:              arbAlias,
		activeTab:             0,
		tabNames:              tabNames,
		content:               vp,
		valuesList:            vals,
		valuesPreview:         valsPrev,
		catalogSetList:        catSets,
		depDetailSets:         depSets,
		palette:               newPaletteModel(),
		keys:                  keys,
		versionsWatched:       map[string]versionsWatch{},
		versionsInFlight:      map[string]bool{},
		sourcesName:           srcName,
		sourcesGit:            srcGit,
		sourcesRef:            srcRef,
		sourcesPlat:           srcPlat,
		sourcesFocus:          0,
		catalogCollisionAlias: collAlias,
		catalogCollisionChoice: collisionChoiceAlias,
	}

	// Initialize the window title cache so the first Update() doesn't re-emit the
	// same title we set in Init().
	if windowTitleEnabled() {
		m.lastWindowTitle = buildWindowTitle(m)
	}

	return m
}

func (m AppModel) Init() tea.Cmd {
	cmds := []tea.Cmd{
		m.beginBusy("Reloading"),
		m.reloadInstancesCmd(),
		tea.Tick(versionsRefreshInterval, func(t time.Time) tea.Msg { return versionsRefreshTickMsg{now: t} }),
	}
	if windowTitleEnabled() {
		cmds = append(cmds, tea.SetWindowTitle(m.lastWindowTitle))
	}
	return tea.Batch(cmds...)
}

func selectedSetNames(items []list.Item) []string {
	out := []string{}
	for _, it := range items {
		si, ok := it.(setChoiceItem)
		if !ok {
			continue
		}
		if si.C.On {
			out = append(out, si.C.Name)
		}
	}
	sort.Strings(out)
	return out
}

func uniqueStrings(in []string) []string {
	if len(in) < 2 {
		return in
	}
	out := make([]string, 0, len(in))
	prev := ""
	for i, s := range in {
		if i == 0 || s != prev {
			out = append(out, s)
		}
		prev = s
	}
	return out
}

type errMsg struct{ err error }

type regenDoneMsg struct{}

// editValuesDoneMsg is emitted after the editor for values.instance.yaml exits.
// We use it to trigger an automatic regenerate of merged values.yaml.
type editValuesDoneMsg struct{}

type appliedMsg struct{}

type instanceRenamedMsg struct{ inst instances.Instance }

type instanceDeletedMsg struct{ name string }

type instanceRenameRequest struct{ newName string }

type instanceDeleteRequest struct{}

type instancesMsg struct{ items []instances.Instance }

type chartMsg struct{ chart yamlchart.Chart }

	type catalogMsg struct{ entries []catalog.EntryWithSource }

type catalogSyncDoneMsg struct{ err error }

type depPresetsSyncDoneMsg struct {
	dep yamlchart.Dependency
	err error
}

type sourcesSavedMsg struct {
	cfg *config.Config
	err error
}

type ahSearchMsg struct{ results []artifacthub.PackageSummary }

type ahVersionsMsg struct{ versions []artifacthub.Version }

type ahDetailMsg struct{ readme, values string }

func (m AppModel) openDepActionsSelected() (tea.Model, tea.Cmd) {
	it := m.depsList.SelectedItem()
	if it == nil {
		return m, nil
	}
	di, ok := it.(depItem)
	if !ok {
		return m, func() tea.Msg { return errMsg{fmt.Errorf("unexpected dependency item type")} }
	}
	dep := di.Dep
	// Load dep source metadata for display.
	if m.selected != nil {
		if meta, ok := readDepSourceMeta(m.params.RepoRoot, m.selected.Name, yamlchart.DependencyID(dep)); ok {
			m.depActionsSource = meta
			m.depActionsSourceOK = true
		} else {
			m.depActionsSource = depSourceMeta{}
			m.depActionsSourceOK = false
		}
	}

	m.depActionsOpen = true
	m.depActionsDep = dep
	m.modalErr = ""
	// Ensure the list has a usable size even if we haven't received a
	// tea.WindowSizeMsg yet.
	if m.width > 0 && m.height > 0 {
		m.depActionsList.SetSize(max(10, m.width-6), max(5, m.height-12))
	}
	m.depActionsList.SetItems([]list.Item{
		depActionItem(depActionSyncPresets),
		depActionItem(depActionChangeVer),
		depActionItem(depActionUpgrade),
		depActionItem(depActionRemove),
	})
	m.depActionsList.Select(0)
	return m, nil
}

func (m AppModel) depActionsUpdate(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Close.
	if msg.Type == tea.KeyEsc {
		m.depActionsOpen = false
		m.depActionsDep = yamlchart.Dependency{}
		m.modalErr = ""
		return m, nil
	}

	var cmd tea.Cmd
	m.depActionsList, cmd = m.depActionsList.Update(msg)
	if msg.Type != tea.KeyEnter {
		return m, cmd
	}
	it := m.depActionsList.SelectedItem()
	if it == nil {
		return m, cmd
	}
	ai, ok := it.(depActionItem)
	if !ok {
		return m, func() tea.Msg { return errMsg{fmt.Errorf("unexpected dep action item type")} }
	}

		// Close menu before launching the chosen action.
		m.depActionsOpen = false

		switch ai {
		case depActionItem(depActionRemove):
			// When actions are invoked from the actions menu, use the same cleanup pipeline
			// as the dependency detail delete.
			m.depDetailDep = m.depActionsDep
			return m, tea.Batch(m.beginBusy("Updating"), m.deleteDepFromDetailCmd())
		case depActionItem(depActionUpgrade):
			return m.upgradeSelectedDepCmd()
		case depActionItem(depActionChangeVer):
			return m.openDepEditSelected()
		case depActionItem(depActionSyncPresets):
			return m, tea.Batch(m.beginBusy("Syncing"), m.syncSelectedDepPresetsCmd())
		default:
			return m, cmd
		}
	}

func (m AppModel) loadCatalogSetsCmd(e catalog.EntryWithSource) tea.Cmd {
	return func() tea.Msg {
		if m.params.Config == nil {
			return catalogSetsMsg{entry: e, sets: nil, err: fmt.Errorf("no config loaded; cannot resolve preset sets")}
		}
		// Resolve against the synced preset cache using presets.Resolve.
		dep := yamlchart.Dependency{Name: e.Entry.Chart.Name, Repository: e.Entry.Chart.Repo, Version: e.Entry.Version}
		res, err := presets.Resolve(m.params.RepoRoot, *m.params.Config, []yamlchart.Dependency{dep})
		if err != nil {
			return catalogSetsMsg{entry: e, sets: nil, err: err}
		}
		rd, ok := res.ByID[yamlchart.DependencyID(dep)]
		if !ok {
			return catalogSetsMsg{entry: e, sets: []setChoice{}, err: nil}
		}
		setNames := make([]string, 0, len(rd.SetPaths))
		for s := range rd.SetPaths {
			setNames = append(setNames, s)
		}
		sort.Strings(setNames)
		defaults := map[string]struct{}{}
		for _, s := range e.Entry.DefaultSets {
			s = strings.TrimSpace(s)
			if s != "" {
				defaults[s] = struct{}{}
			}
		}
		out := make([]setChoice, 0, len(setNames))
		for _, s := range setNames {
			_, isDef := defaults[s]
			out = append(out, setChoice{Name: s, Default: isDef, On: isDef})
		}
		return catalogSetsMsg{entry: e, sets: out, err: nil}
	}
}

func (m AppModel) loadDepDetailSetsCmd(dep yamlchart.Dependency) tea.Cmd {
	return func() tea.Msg {
		if m.params.Config == nil {
			return depDetailSetsMsg{ID: yamlchart.DependencyID(dep), Sets: nil, Err: fmt.Errorf("no config loaded; cannot resolve preset sets")}
		}
		if m.selected == nil {
			return depDetailSetsMsg{ID: yamlchart.DependencyID(dep), Sets: nil, Err: fmt.Errorf("no instance selected")}
		}
		id := yamlchart.DependencyID(dep)
		res, err := presets.Resolve(m.params.RepoRoot, *m.params.Config, []yamlchart.Dependency{dep})
		if err != nil {
			return depDetailSetsMsg{ID: id, Sets: nil, Err: err}
		}
		rd, ok := res.ByID[id]
		if !ok {
			return depDetailSetsMsg{ID: id, Sets: []setChoice{}, Err: nil}
		}
		setNames := make([]string, 0, len(rd.SetPaths))
		for s := range rd.SetPaths {
			setNames = append(setNames, s)
		}
		sort.Strings(setNames)

		selected, _ := readSelectedDepSetsForDep(m.selected.Path, string(id))
		selectedSet := map[string]struct{}{}
		for _, s := range selected {
			selectedSet[s] = struct{}{}
		}

		out := make([]setChoice, 0, len(setNames))
		for _, s := range setNames {
			_, on := selectedSet[s]
			out = append(out, setChoice{Name: s, Default: false, On: on})
		}
		return depDetailSetsMsg{ID: id, Sets: out, Err: nil}
	}
}

func readSelectedDepSetsForDep(instancePath string, depID string) ([]string, error) {
	depID = strings.TrimSpace(depID)
	if depID == "" {
		return []string{}, nil
	}
	glob := filepath.Join(instancePath, fmt.Sprintf("values.dep-set.%s--*.yaml", depID))
	files, err := filepath.Glob(glob)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return []string{}, nil
	}
	sets := make([]string, 0, len(files))
	for _, f := range files {
		base := filepath.Base(f)
		name := strings.TrimSuffix(strings.TrimPrefix(base, "values.dep-set."), ".yaml")
		parts := strings.SplitN(name, "--", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.TrimSpace(parts[0]) != depID {
			continue
		}
		setName := strings.TrimSpace(parts[1])
		if setName == "" {
			continue
		}
		sets = append(sets, setName)
	}
	sort.Strings(sets)
	sets = uniqueStrings(sets)
	return sets, nil
}

func (m AppModel) applyCatalogDependencyWithSetsCmd(dep yamlchart.Dependency, sets []string) tea.Cmd {
	return func() tea.Msg {
		// Backward-compat: run as a normal apply without collision prompting.
		ctx, cancel := context.WithCancel(contextBG())
		defer cancel()
		return m.applyCatalogDependencyWithSetsCmdCtx(ctx, 0, dep, sets, applyOptions{Override: true, DeleteMarkersForDepID: false})()
	}
}

type applyOptions struct {
	// Override controls whether we allow updating an existing dependency with the same Name.
	Override bool
	// DeleteMarkersForDepID deletes existing per-dependency set marker files for the final depID.
	DeleteMarkersForDepID bool
}

func (m *AppModel) startApplyCmd(dep yamlchart.Dependency, sets []string, opt applyOptions) tea.Cmd {
	// Start a cancellable, blocking apply.
	m.applyID++
	applyID := m.applyID
	ctx, cancel := context.WithCancel(contextBG())
	m.applyCancel = cancel
	m.applyOpen = true
	m.applyCancelConfirm = false
	m.applyCancelRequested = false
	m.modalErr = ""

	// Keep the busy spinner going and show a centered blocking overlay.
	cmd := m.applyCatalogDependencyWithSetsCmdCtx(ctx, applyID, dep, sets, opt)
	return tea.Batch(m.beginBusy("Applying"), cmd)
}

func (m AppModel) applyCatalogDependencyWithSetsCmdCtx(ctx context.Context, applyID int, dep yamlchart.Dependency, sets []string, opt applyOptions) tea.Cmd {
	return func() tea.Msg {
		if m.selected == nil {
			return applyDoneMsg{applyID: applyID, chart: nil, err: fmt.Errorf("no instance selected")}
		}
		chartPath := filepath.Join(m.selected.Path, "Chart.yaml")
		// Keep a copy so we can roll back if apply fails.
		origChartYAML, _ := os.ReadFile(chartPath)
		lockPath := filepath.Join(m.selected.Path, "Chart.lock")
		origLockYAML, _ := os.ReadFile(lockPath)
		hadOrigLock := origLockYAML != nil
		rollback := func() {
			if origChartYAML != nil {
				_ = os.WriteFile(chartPath, origChartYAML, 0o644)
			}
			if hadOrigLock {
				_ = os.WriteFile(lockPath, origLockYAML, 0o644)
			} else {
				_ = os.Remove(lockPath)
			}
		}

		c, err := yamlchart.ReadChart(chartPath)
		if err != nil {
			return applyDoneMsg{applyID: applyID, chart: nil, err: err}
		}
		// Collision handling: disallow adding the same dependency name twice only when
		// the new dependency has no alias (depID == name). If an alias is provided,
		// Helm supports it and we upsert by depID.
		if strings.TrimSpace(dep.Alias) == "" {
			for _, d := range c.Dependencies {
				if d.Name == dep.Name {
					if !opt.Override {
						return applyDoneMsg{applyID: applyID, chart: nil, err: fmt.Errorf("dependency %q already exists", dep.Name)}
					}
					break
				}
			}
		}
		if err := c.UpsertDependency(dep); err != nil {
			return applyDoneMsg{applyID: applyID, chart: nil, err: err}
		}
		if err := yamlchart.WriteChart(chartPath, c); err != nil {
			return applyDoneMsg{applyID: applyID, chart: nil, err: err}
		}

		depID := yamlchart.DependencyID(dep)
		if opt.DeleteMarkersForDepID {
			_ = deleteDepSetMarkers(m.selected.Path, depID)
		}
		for _, setName := range sets {
			setName = strings.TrimSpace(setName)
			if setName == "" {
				continue
			}
			p := filepath.Join(m.selected.Path, fmt.Sprintf("values.dep-set.%s--%s.yaml", depID, setName))
			if _, err := os.Stat(p); err == nil {
				continue
			}
			_ = os.WriteFile(p, []byte("{}\n"), 0o644)
		}

		// Apply pipeline.
		if _, err := instances.RelockIfDepsChanged(ctx, m.params.RepoRoot, m.selected.Path); err != nil {
			rollback()
			return applyDoneMsg{applyID: applyID, chart: nil, err: err}
		}
		if m.params.Config != nil {
			_, err = presets.Import(presets.ImportParams{RepoRoot: m.params.RepoRoot, InstancePath: m.selected.Path, Config: *m.params.Config, Dependencies: c.Dependencies})
			if err != nil {
				rollback()
				return applyDoneMsg{applyID: applyID, chart: nil, err: err}
			}
		}
		if err := values.GenerateMergedValues(m.selected.Path); err != nil {
			rollback()
			return applyDoneMsg{applyID: applyID, chart: nil, err: err}
		}
		return applyDoneMsg{applyID: applyID, chart: &c, err: nil}
	}
}

func deleteDepSetMarkers(instancePath string, depID yamlchart.DepID) error {
	glob := filepath.Join(instancePath, fmt.Sprintf("values.dep-set.%s--*.yaml", depID))
	files, err := filepath.Glob(glob)
	if err != nil {
		return err
	}
	for _, p := range files {
		_ = os.Remove(p)
	}
	return nil
}

func writeDepSetMarker(instancePath string, depID yamlchart.DepID, setName string) error {
	setName = strings.TrimSpace(setName)
	if setName == "" {
		return nil
	}
	p := filepath.Join(instancePath, fmt.Sprintf("values.dep-set.%s--%s.yaml", depID, setName))
	return os.WriteFile(p, []byte("{}\n"), 0o644)
}

func deleteDepSetMarker(instancePath string, depID yamlchart.DepID, setName string) error {
	setName = strings.TrimSpace(setName)
	if setName == "" {
		return nil
	}
	p := filepath.Join(instancePath, fmt.Sprintf("values.dep-set.%s--%s.yaml", depID, setName))
	_ = os.Remove(p)
	return nil
}

func removeOrphanDepSetMarkers(instancePath string, depID yamlchart.DepID, allowedSets map[string]struct{}) error {
	glob := filepath.Join(instancePath, fmt.Sprintf("values.dep-set.%s--*.yaml", depID))
	files, err := filepath.Glob(glob)
	if err != nil {
		return err
	}
	for _, f := range files {
		base := filepath.Base(f)
		name := strings.TrimSuffix(strings.TrimPrefix(base, "values.dep-set."), ".yaml")
		parts := strings.SplitN(name, "--", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.TrimSpace(parts[0]) != string(depID) {
			continue
		}
		setName := strings.TrimSpace(parts[1])
		if setName == "" {
			continue
		}
		if allowedSets != nil {
			if _, ok := allowedSets[setName]; !ok {
				_ = os.Remove(f)
			}
		}
	}
	return nil
}

func (m AppModel) applyDepDetailSetsCmd() tea.Cmd {
	return func() tea.Msg {
		if m.selected == nil {
			return errMsg{fmt.Errorf("no instance selected")}
		}
		if m.params.Config == nil {
			return errMsg{fmt.Errorf("no config loaded")}
		}
		dep := m.depDetailDep
		depID := yamlchart.DependencyID(dep)

		// Resolve available sets from preset cache.
		res, err := presets.Resolve(m.params.RepoRoot, *m.params.Config, []yamlchart.Dependency{dep})
		if err != nil {
			return errMsg{err}
		}
		rd, ok := res.ByID[depID]
		allowed := map[string]struct{}{}
		if ok {
			for s := range rd.SetPaths {
				allowed[s] = struct{}{}
			}
		}

		// Remove orphan markers (selected sets that no longer exist in cache).
		_ = removeOrphanDepSetMarkers(m.selected.Path, depID, allowed)

		// Apply current UI selection by writing/removing markers.
		items := m.depDetailSets.Items()
		for _, it := range items {
			si, ok := it.(setChoiceForDepItem)
			if !ok {
				continue
			}
			if si.C.On {
				_ = writeDepSetMarker(m.selected.Path, depID, si.C.Name)
			} else {
				_ = deleteDepSetMarker(m.selected.Path, depID, si.C.Name)
			}
		}

		// Re-import presets and regenerate merged values.
		c, err := yamlchart.ReadChart(filepath.Join(m.selected.Path, "Chart.yaml"))
		if err != nil {
			return errMsg{err}
		}
		if _, err := presets.Import(presets.ImportParams{RepoRoot: m.params.RepoRoot, InstancePath: m.selected.Path, Config: *m.params.Config, Dependencies: c.Dependencies}); err != nil {
			return errMsg{err}
		}
		if err := values.GenerateMergedValues(m.selected.Path); err != nil {
			return errMsg{err}
		}
		return appliedMsg{}
	}
}

func (m AppModel) syncSelectedDepPresetsCmd() tea.Cmd {
	return func() tea.Msg {
		if m.selected == nil {
			return depPresetsSyncDoneMsg{dep: m.depActionsDep, err: fmt.Errorf("no instance selected")}
		}
		if m.params.Config == nil {
			return depPresetsSyncDoneMsg{dep: m.depActionsDep, err: fmt.Errorf("no config loaded")}
		}
		dep := m.depActionsDep
		depID := yamlchart.DependencyID(dep)

		// Sync only sources that have presets enabled. (We can't reliably map a dep
		// back to an individual source without a catalog -> sourceName index.)
		s := catalog.NewSyncer(m.params.RepoRoot)
		_, err := s.SyncFiltered(contextBG(), *m.params.Config, func(src config.Source) bool {
			return src.Presets.Enabled
		})
		if err != nil {
			return depPresetsSyncDoneMsg{dep: dep, err: err}
		}

		// Resolve allowed sets after sync and remove orphan markers.
		res, err := presets.Resolve(m.params.RepoRoot, *m.params.Config, []yamlchart.Dependency{dep})
		if err != nil {
			return depPresetsSyncDoneMsg{dep: dep, err: err}
		}
		allowed := map[string]struct{}{}
		if rd, ok := res.ByID[depID]; ok {
			for s := range rd.SetPaths {
				allowed[s] = struct{}{}
			}
		}
		_ = removeOrphanDepSetMarkers(m.selected.Path, depID, allowed)

		// Re-import presets and regenerate merged values.
		c, err := yamlchart.ReadChart(filepath.Join(m.selected.Path, "Chart.yaml"))
		if err != nil {
			return depPresetsSyncDoneMsg{dep: dep, err: err}
		}
		if _, err := presets.Import(presets.ImportParams{RepoRoot: m.params.RepoRoot, InstancePath: m.selected.Path, Config: *m.params.Config, Dependencies: c.Dependencies}); err != nil {
			return depPresetsSyncDoneMsg{dep: dep, err: err}
		}
		if err := values.GenerateMergedValues(m.selected.Path); err != nil {
			return depPresetsSyncDoneMsg{dep: dep, err: err}
		}
		return depPresetsSyncDoneMsg{dep: dep, err: nil}
	}
}

// (catalogSetsMsg is declared near dep wizard types; keep only one definition)

// depDetailPreviewsMsg carries readme + default values previews for the selected dependency.
type depDetailPreviewsMsg struct {
	ID            yamlchart.DepID
	readme        string
	defaultValues string
	schema        string
}

// depDetailVersionsMsg carries the available versions list for the selected dependency.
type depDetailVersionsMsg struct {
	ID       yamlchart.DepID
	Versions []string
}

type depVersionsMsg struct {
	ID       yamlchart.DepID
	Versions []string
}

type versionsTarget int

const (
	versionsTargetDepEdit versionsTarget = iota
	versionsTargetDepDetail
	versionsTargetBackground
)

type versionsRefreshResultMsg struct {
	key       string
	repoURL   string
	chartName string
	depID     yamlchart.DepID
	target    versionsTarget
	versions  []string
	err       error
}

type versionsRefreshTickMsg struct{ now time.Time }

type versionsWatch struct {
	repoURL     string
	chartName   string
	lastRefresh time.Time
}

const (
	versionsCacheTTL        = 15 * time.Minute
	versionsRefreshInterval = 15 * time.Minute
)

type valuesPreviewLoadedMsg struct {
	path    string
	content string
}

// staleWhileOpen avoids re-opening errors while the modal is active.
func (m *AppModel) handleVersionsRefreshResult(msg versionsRefreshResultMsg) tea.Cmd {
	// Mark single-flight complete.
	if msg.key != "" {
		delete(m.versionsInFlight, msg.key)
	}

	// Update watched refresh timestamp on success.
	if msg.err == nil {
		if w, ok := m.versionsWatched[msg.key]; ok {
			w.lastRefresh = time.Now().UTC()
			m.versionsWatched[msg.key] = w
		}
	}

	// Apply to UI when still relevant.
	switch msg.target {
	case versionsTargetDepEdit:
		m.depEditLoading = false
		if !m.depEditOpen {
			return nil
		}
		if msg.depID != yamlchart.DependencyID(m.depEditDep) {
			return nil
		}
		if msg.err == nil {
			m.setVersionsList(msg.versions, versionsTargetDepEdit)
		}
		return nil
	case versionsTargetDepDetail:
		m.depDetailVersionsLoading = false
		if !m.depDetailOpen {
			return nil
		}
		if msg.depID != yamlchart.DependencyID(m.depDetailDep) {
			return nil
		}
		if msg.err == nil {
			m.setVersionsList(msg.versions, versionsTargetDepDetail)
		}
		return nil
	default:
		// Background refresh: no direct UI.
		return nil
	}
}

// noopMsg is used by background cmds to signal "success but no UI change",
// so the busy indicator can stop cleanly.
type noopMsg struct{}

// depAppliedMsg indicates a dependency draft was applied (Chart.yaml written)
// and the modal can be closed.
type depAppliedMsg struct{ chart yamlchart.Chart }

type depAppliedAndAppliedMsg struct{ chart yamlchart.Chart }

func (m *AppModel) setVersionsList(items []string, which versionsTarget) {
	data := items
	listModel := &m.depEditVersions
	curVer := ""
	if which == versionsTargetDepDetail {
		listModel = &m.depDetailVersions
		curVer = m.depDetailDep.Version
		m.depDetailVersionsData = data
	} else {
		curVer = m.depEditDep.Version
		m.depEditVersionsData = data
	}
	its := make([]list.Item, 0, len(data))
	for _, v := range data {
		its = append(its, versionItem(v))
	}
	listModel.SetItems(its)
	// Keep selection on current version when possible.
	listModel.Select(0)
	for i := range data {
		if strings.TrimSpace(data[i]) == strings.TrimSpace(curVer) {
			listModel.Select(i)
			break
		}
	}
}

func versionsKey(repoURL, chartName string) string {
	return helmutil.VersionsCacheKey(repoURL, chartName)
}

func (m *AppModel) watchVersions(dep yamlchart.Dependency) {
	if strings.TrimSpace(dep.Repository) == "" || strings.TrimSpace(dep.Name) == "" {
		return
	}
	if strings.HasPrefix(dep.Repository, "oci://") {
		return
	}
	key := versionsKey(dep.Repository, dep.Name)
	if _, ok := m.versionsWatched[key]; ok {
		return
	}
	last := time.Time{}
	if _, fetchedAt, ok, err := helmutil.ReadVersionsCache(m.params.RepoRoot, dep.Repository, dep.Name); err == nil && ok {
		last = fetchedAt
	}
	m.versionsWatched[key] = versionsWatch{repoURL: dep.Repository, chartName: dep.Name, lastRefresh: last}
}

func (m *AppModel) refreshVersionsCmd(dep yamlchart.Dependency, target versionsTarget) tea.Cmd {
	return func() tea.Msg {
		key := versionsKey(dep.Repository, dep.Name)
		ctx, cancel := context.WithTimeout(contextBG(), 60*time.Second)
		defer cancel()
		vs, err := helmutil.RepoChartVersions(ctx, m.params.RepoRoot, dep.Repository, dep.Name, 24*time.Hour)
		if err == nil {
			_, _ = helmutil.WriteVersionsCache(m.params.RepoRoot, dep.Repository, dep.Name, vs)
		}
		return versionsRefreshResultMsg{
			key:       key,
			repoURL:   dep.Repository,
			chartName: dep.Name,
			depID:     yamlchart.DependencyID(dep),
			target:    target,
			versions:  vs,
			err:       err,
		}
	}
}

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

		// Offline: read from cached chart archive (.tgz) if present.
		if tgzPath, ok := helmutil.FindCachedChartArchive(m.params.RepoRoot, repoURL, chartName, version); ok {
			readme, values, err := helmutil.ReadChartArchiveFiles(tgzPath)
			if err != nil {
				return errMsg{err}
			}
			if strings.TrimSpace(readme) != "" {
				_ = helmutil.WriteShowCache(m.params.RepoRoot, repoURL, chartName, version, helmutil.ShowKindReadme, readme)
			}
			if strings.TrimSpace(values) != "" {
				_ = helmutil.WriteShowCache(m.params.RepoRoot, repoURL, chartName, version, helmutil.ShowKindValues, values)
			}
			if strings.TrimSpace(readme) != "" || strings.TrimSpace(values) != "" {
				return ahDetailMsg{readme: readme, values: values}
			}
		}

		// Per-repoURL isolated Helm env so repo update touches only this repo.
		env := helmutil.EnvForRepoURL(m.params.RepoRoot, repoURL)
		ctx, cancel := context.WithTimeout(contextBG(), 60*time.Second)
		defer cancel()
		// OCI refs can be used directly.
		if strings.HasPrefix(repoURL, "oci://") {
			ref, err := helmutil.OCIChartRef(repoURL, chartName)
			if err != nil {
				return errMsg{err}
			}
			// Try pulling archive first (reduces network calls and avoids show timeouts).
			if tgzPath, err := helmutil.PullChartArchive(ctx, env, repoURL, chartName, version); err == nil {
				if readme, values, err2 := helmutil.ReadChartArchiveFiles(tgzPath); err2 == nil {
					_ = helmutil.WriteShowCache(m.params.RepoRoot, repoURL, chartName, version, helmutil.ShowKindReadme, readme)
					_ = helmutil.WriteShowCache(m.params.RepoRoot, repoURL, chartName, version, helmutil.ShowKindValues, values)
					return ahDetailMsg{readme: readme, values: values}
				}
			}
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
		// If archive isn't present, try pulling it first (prefer offline extraction).
		if tgzPath, err := helmutil.PullChartArchive(ctx, env, repoURL, chartName, version); err == nil {
			if readme, values, err2 := helmutil.ReadChartArchiveFiles(tgzPath); err2 == nil {
				_ = helmutil.WriteShowCache(m.params.RepoRoot, repoURL, chartName, version, helmutil.ShowKindReadme, readme)
				_ = helmutil.WriteShowCache(m.params.RepoRoot, repoURL, chartName, version, helmutil.ShowKindValues, values)
				return ahDetailMsg{readme: readme, values: values}
			}
		}
		// Best-effort show with minimal side effects.
		// If user requested force refresh, run repo update explicitly.
		if force {
			if err := helmutil.RepoUpdateNames(ctx, env, repoName); err != nil {
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

func (m AppModel) loadValuesPreviewCmd(relPath string) tea.Cmd {
	return func() tea.Msg {
		if m.selected == nil {
			return valuesPreviewLoadedMsg{path: relPath, content: "No instance selected"}
		}
		p := filepath.Join(m.selected.Path, relPath)
		b, err := os.ReadFile(p)
		if err != nil {
			return errMsg{fmt.Errorf("could not read %s: %w", relPath, err)}
		}
		if len(b) == 0 {
			return valuesPreviewLoadedMsg{path: relPath, content: "(empty file)"}
		}
		content := string(b)
		content = maybeHighlightYAMLForDisplay(relPath, content)
		return valuesPreviewLoadedMsg{path: relPath, content: content}
	}
}

func (m AppModel) loadDepDetailPreviewsCmd(dep yamlchart.Dependency) tea.Cmd {
	return func() tea.Msg {
		id := yamlchart.DependencyID(dep)
		ctx, cancel := context.WithTimeout(contextBG(), 60*time.Second)
		defer cancel()

		// 0) Instance-local vendor dir (if present): charts/<name>/values.yaml, charts/<name>/README.md, charts/<name>/values.schema.json.
		// This is the most reliable and requires zero network.
		if m.selected != nil {
			readme, values, schema, ok, err := readInstanceVendoredChartFiles(m.selected.Path, dep)
			if err != nil {
				return errMsg{err}
			}
			if ok {
				if strings.TrimSpace(readme) != "" {
					_ = helmutil.WriteShowCache(m.params.RepoRoot, dep.Repository, dep.Name, dep.Version, helmutil.ShowKindReadme, readme)
				}
				if strings.TrimSpace(values) != "" {
					_ = helmutil.WriteShowCache(m.params.RepoRoot, dep.Repository, dep.Name, dep.Version, helmutil.ShowKindValues, values)
				}
				if strings.TrimSpace(schema) != "" {
					_ = helmutil.WriteShowCache(m.params.RepoRoot, dep.Repository, dep.Name, dep.Version, helmutil.ShowKindSchema, schema)
				}
				return depDetailPreviewsMsg{ID: id, readme: readme, defaultValues: values, schema: schema}
			}
		}

		// 1) Offline: read from cached chart archive (.tgz) if present.
		if tgzPath, ok := helmutil.FindCachedChartArchive(m.params.RepoRoot, dep.Repository, dep.Name, dep.Version); ok {
			readme, values, schema, err := helmutil.ReadChartArchiveFilesWithSchema(tgzPath)
			if err != nil {
				return errMsg{err}
			}
			if strings.TrimSpace(readme) != "" {
				_ = helmutil.WriteShowCache(m.params.RepoRoot, dep.Repository, dep.Name, dep.Version, helmutil.ShowKindReadme, readme)
			}
			if strings.TrimSpace(values) != "" {
				_ = helmutil.WriteShowCache(m.params.RepoRoot, dep.Repository, dep.Name, dep.Version, helmutil.ShowKindValues, values)
			}
			if strings.TrimSpace(schema) != "" {
				_ = helmutil.WriteShowCache(m.params.RepoRoot, dep.Repository, dep.Name, dep.Version, helmutil.ShowKindSchema, schema)
			}
			if strings.TrimSpace(readme) != "" || strings.TrimSpace(values) != "" || strings.TrimSpace(schema) != "" {
				return depDetailPreviewsMsg{ID: id, readme: readme, defaultValues: values, schema: schema}
			}
		}

		// 2) helmdex show cache (can be stale across versions of extraction logic, so keep after tgz reads).
		readme, okReadme, err := helmutil.ReadShowCache(m.params.RepoRoot, dep.Repository, dep.Name, dep.Version, helmutil.ShowKindReadme)
		if err != nil {
			return errMsg{err}
		}
		values, okValues, err := helmutil.ReadShowCache(m.params.RepoRoot, dep.Repository, dep.Name, dep.Version, helmutil.ShowKindValues)
		if err != nil {
			return errMsg{err}
		}
		schema, okSchema, err := helmutil.ReadShowCache(m.params.RepoRoot, dep.Repository, dep.Name, dep.Version, helmutil.ShowKindSchema)
		if err != nil {
			return errMsg{err}
		}
		if okReadme || okValues || okSchema {
			return depDetailPreviewsMsg{ID: id, readme: readme, defaultValues: values, schema: schema}
		}

		// 3) If missing: pull chart archive and read from it.
		env := helmutil.EnvForRepoURL(m.params.RepoRoot, dep.Repository)
		if tgzPath, err := helmutil.PullChartArchive(ctx, env, dep.Repository, dep.Name, dep.Version); err == nil {
			readme, values, schema, err2 := helmutil.ReadChartArchiveFilesWithSchema(tgzPath)
			if err2 == nil {
				if strings.TrimSpace(readme) != "" {
					_ = helmutil.WriteShowCache(m.params.RepoRoot, dep.Repository, dep.Name, dep.Version, helmutil.ShowKindReadme, readme)
				}
				if strings.TrimSpace(values) != "" {
					_ = helmutil.WriteShowCache(m.params.RepoRoot, dep.Repository, dep.Name, dep.Version, helmutil.ShowKindValues, values)
				}
				if strings.TrimSpace(schema) != "" {
					_ = helmutil.WriteShowCache(m.params.RepoRoot, dep.Repository, dep.Name, dep.Version, helmutil.ShowKindSchema, schema)
				}
				return depDetailPreviewsMsg{ID: id, readme: readme, defaultValues: values, schema: schema}
			}
		} else {
			// Preserve pull error for the final error message if we also fail helm show.
		}

		// 4) Last resort: helm show.
		if strings.HasPrefix(dep.Repository, "oci://") {
			ref, err := helmutil.OCIChartRef(dep.Repository, dep.Name)
			if err != nil {
				return errMsg{err}
			}
			readme, err := helmutil.ShowReadme(ctx, env, ref, dep.Version)
			if err != nil {
				return errMsg{err}
			}
			values, err := helmutil.ShowValues(ctx, env, ref, dep.Version)
			if err != nil {
				return errMsg{err}
			}
			_ = helmutil.WriteShowCache(m.params.RepoRoot, dep.Repository, dep.Name, dep.Version, helmutil.ShowKindReadme, readme)
			_ = helmutil.WriteShowCache(m.params.RepoRoot, dep.Repository, dep.Name, dep.Version, helmutil.ShowKindValues, values)
			// Schema is not available via `helm show values`; prefer tgz extraction.
			return depDetailPreviewsMsg{ID: id, readme: readme, defaultValues: values, schema: ""}
		}
		repoName := helmutil.RepoNameForURL(dep.Repository)
		_ = helmutil.RepoAdd(ctx, env, repoName, dep.Repository)
		ref := repoName + "/" + dep.Name
		readme, err = helmutil.ShowReadmeBestEffort(ctx, env, ref, dep.Version, 24*time.Hour)
		if err != nil {
			return errMsg{err}
		}
		values, err = helmutil.ShowValuesBestEffort(ctx, env, ref, dep.Version, 24*time.Hour)
		if err != nil {
			return errMsg{err}
		}
		_ = helmutil.WriteShowCache(m.params.RepoRoot, dep.Repository, dep.Name, dep.Version, helmutil.ShowKindReadme, readme)
		_ = helmutil.WriteShowCache(m.params.RepoRoot, dep.Repository, dep.Name, dep.Version, helmutil.ShowKindValues, values)
		return depDetailPreviewsMsg{ID: id, readme: readme, defaultValues: values, schema: ""}
	}
}

func (m AppModel) loadDepDetailVersionsCmd(dep yamlchart.Dependency) tea.Cmd {
	return func() tea.Msg {
		id := yamlchart.DependencyID(dep)
		if strings.HasPrefix(dep.Repository, "oci://") {
			return depDetailVersionsMsg{ID: id, Versions: nil}
		}

		// Cache-first: return cached versions immediately when present.
		if vs, _, ok, err := helmutil.ReadVersionsCache(m.params.RepoRoot, dep.Repository, dep.Name); err == nil && ok {
			return depDetailVersionsMsg{ID: id, Versions: vs}
		}
		// No cache: trigger a normal load; UI will show loader.
		ctx, cancel := context.WithTimeout(contextBG(), 60*time.Second)
		defer cancel()
		vs, err := helmutil.RepoChartVersions(ctx, m.params.RepoRoot, dep.Repository, dep.Name, 24*time.Hour)
		if err != nil {
			return errMsg{err}
		}
		_, _ = helmutil.WriteVersionsCache(m.params.RepoRoot, dep.Repository, dep.Name, vs)
		return depDetailVersionsMsg{ID: id, Versions: vs}
	}
}

func readInstanceVendoredChartFiles(instancePath string, dep yamlchart.Dependency) (readme string, values string, schema string, ok bool, err error) {
	// Helm vendor layout: <instance>/charts/<dep.Name>/values.yaml
	base := filepath.Join(instancePath, "charts", dep.Name)
	st, err := os.Stat(base)
	if err != nil || !st.IsDir() {
		return "", "", "", false, nil
	}
	// Prefer values.yaml, values.schema.json and README.md from the vendored chart dir.
	readmePath := filepath.Join(base, "README.md")
	valuesPath := filepath.Join(base, "values.yaml")
	schemaPath := filepath.Join(base, "values.schema.json")

	if b, err := os.ReadFile(readmePath); err == nil {
		readme = string(b)
	}
	if b, err := os.ReadFile(valuesPath); err == nil {
		values = string(b)
	}
	if b, err := os.ReadFile(schemaPath); err == nil {
		schema = string(b)
	}
	if strings.TrimSpace(readme) == "" && strings.TrimSpace(values) == "" && strings.TrimSpace(schema) == "" {
		return "", "", "", false, nil
	}
	return readme, values, schema, true, nil
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
		e, err := catalog.LoadLocalCatalogEntriesWithSource(m.params.RepoRoot)
		if err != nil {
			return errMsg{err}
		}
		return catalogMsg{entries: e}
	}
}

func (m AppModel) catalogSyncCmd() tea.Cmd {
	return func() tea.Msg {
		if m.params.Config == nil {
			return catalogSyncDoneMsg{err: fmt.Errorf("no config loaded; cannot sync catalog")}
		}
		s := catalog.NewSyncer(m.params.RepoRoot)
		_, err := s.Sync(contextBG(), *m.params.Config)
		return catalogSyncDoneMsg{err: err}
	}
}

func (m AppModel) saveSourcesCmd(name, gitURL, gitRef, platform string) tea.Cmd {
	return func() tea.Msg {
		name = strings.TrimSpace(name)
		gitURL = strings.TrimSpace(gitURL)
		gitRef = strings.TrimSpace(gitRef)
		platform = strings.TrimSpace(platform)
		if name == "" {
			return sourcesSavedMsg{err: fmt.Errorf("source name is required")}
		}
		if gitURL == "" {
			return sourcesSavedMsg{err: fmt.Errorf("git url/path is required")}
		}
		if platform == "" {
			return sourcesSavedMsg{err: fmt.Errorf("platform name is required (needed for presets/sets)")}
		}

		// UX: allow local filesystem sources (plain directories without `.git`).
		// Sync supports these by copying the directory into `.helmdex/cache/<source>`.
		//
		// We still validate that obvious local paths exist and are directories.
		// For remote git URLs, we do not preflight existence.
		looksLikeRemoteURL := strings.Contains(gitURL, "://") || strings.HasPrefix(gitURL, "git@")
		looksLikeLocalPath := filepath.IsAbs(gitURL) || strings.HasPrefix(gitURL, ".") || strings.Contains(gitURL, string(os.PathSeparator))
		if !looksLikeRemoteURL && looksLikeLocalPath {
			st, err := os.Stat(gitURL)
			if err != nil {
				return sourcesSavedMsg{err: fmt.Errorf("git path %q does not exist", gitURL)}
			}
			if !st.IsDir() {
				return sourcesSavedMsg{err: fmt.Errorf("git path %q is not a directory", gitURL)}
			}
			// Filesystem sources ignore git refs/commits; clear ref to prevent confusion.
			if _, err := os.Stat(filepath.Join(gitURL, ".git")); err != nil {
				gitRef = ""
			}
		}

		cfg := config.DefaultConfig()
		if m.params.Config != nil {
			cfg = *m.params.Config
		}
		cfg.Platform.Name = platform

		// For now, a source configured from TUI always enables both catalog + presets.
		// This is the minimum needed for catalog entries + set discovery.
		src := config.Source{
			Name: name,
			Git:  config.GitRef{URL: gitURL, Ref: gitRef},
			Presets: config.PresetsConfig{
				Enabled:    true,
				ChartsPath: "charts",
			},
			Catalog: config.CatalogConfig{
				Enabled: true,
				Path:    "catalog.yaml",
			},
		}

		updated := false
		for i := range cfg.Sources {
			if cfg.Sources[i].Name == name {
				cfg.Sources[i] = src
				updated = true
				break
			}
		}
		if !updated {
			cfg.Sources = append(cfg.Sources, src)
		}

		if err := cfg.Validate(); err != nil {
			return sourcesSavedMsg{err: err}
		}
		if err := config.WriteFile(m.params.ConfigPath, cfg); err != nil {
			return sourcesSavedMsg{err: err}
		}
		loaded, err := config.LoadFile(m.params.ConfigPath)
		if err != nil {
			return sourcesSavedMsg{err: err}
		}
		return sourcesSavedMsg{cfg: &loaded, err: nil}
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
		return renderMarkdownForDisplay(m.ahPreview.Width, m.ahReadme)
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
	model, cmd := m.updateInner(msg)
	return wrapWithWindowTitle(model, cmd)
}

func (m AppModel) updateInner(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Clear transient OK feedback on the next user input.
	if _, ok := msg.(tea.KeyMsg); ok {
		m.statusOK = ""
	}

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
		// Capture pre-resize scroll offsets so we can preserve position where possible
		// when we re-render markdown for a new width.
		oldAHOff := m.ahPreview.YOffset
		oldDepOff := m.depDetailPreview.YOffset
		oldAHW := m.ahPreview.Width
		oldDepW := m.depDetailPreview.Width

		m.width, m.height = msg.Width, msg.Height
		// Layout has: header + breadcrumb + body + context help + footer status,
		// all wrapped in a padded base style. Keep list/viewports slightly smaller
		// to avoid clipping in smaller terminals.
		m.instList.SetSize(msg.Width-2, msg.Height-5)
		m.content.Width = msg.Width - 2
		// Instance view has a tab bar above the viewport.
		// Reduce height so the body fits within the terminal without clipping.
		m.content.Height = msg.Height - 8
		m.ahPreview.Width = msg.Width - 2
		m.ahPreview.Height = msg.Height - 11
		// Deps tab also has the shared tab bar.
		m.depsList.SetSize(msg.Width-2, msg.Height-8)
		m.valuesList.SetSize(msg.Width-2, msg.Height-8)
		m.depSource.SetSize(msg.Width-2, msg.Height-7)
		m.catalogList.SetSize(msg.Width-2, msg.Height-7)
		m.catalogSetList.SetSize(msg.Width-2, msg.Height-12)
		m.ahResults.SetSize(msg.Width-2, msg.Height-7)
		m.ahVersions.SetSize(msg.Width-2, msg.Height-7)
		m.depEditVersions.SetSize(max(10, msg.Width-6), max(5, msg.Height-12))
		m.depActionsList.SetSize(max(10, msg.Width-6), max(5, msg.Height-12))
		m.palette.SetSize(min(70, msg.Width-4), min(14, msg.Height-6))
		m.depDetailPreview.Width = max(10, msg.Width-6)
		// Dep detail is rendered as a full-body modal, but the overall app View()
		// still includes the global header/breadcrumb and footer lines.
		// Additionally, the modal itself has a header, tab bar, footer, border and padding.
		// If the preview viewport is too tall, the terminal will scroll and the modal
		// top border (and tabs/header) appear cut.
		modalMaxH := max(8, msg.Height-10)
		// Conservative "chrome" height inside the dep detail modal (border+padding + header/tabs/footer + blank separators).
		depDetailChromeH := 12
		m.depDetailPreview.Height = max(5, modalMaxH-depDetailChromeH)
		m.depDetailSets.SetSize(m.depDetailPreview.Width, max(5, m.depDetailPreview.Height-2))
		// Dep detail modal has a header + tab bar + footer around its body.
		// Keep the versions list height aligned with the preview viewport height
		// (minus a couple of lines for the versions hint line) so the modal header
		// stays visible and the output does not exceed the terminal height.
		m.depDetailVersions.SetSize(m.depDetailPreview.Width, max(5, m.depDetailPreview.Height-2))
		m.valuesPreview.Width = max(10, msg.Width-6)
		m.valuesPreview.Height = max(5, msg.Height-14)
		// Ensure the viewport never ends up with negative size.
		if m.ahPreview.Height < 3 {
			m.ahPreview.Height = 3
		}

		// Re-render markdown previews to reflow with the new width.
		// Only do this when we have content; otherwise we'd replace helpful
		// placeholder messages.
		if m.ahReadme != "" && (oldAHW != m.ahPreview.Width) {
			m.ahPreview.SetContent(m.renderAHDetailBody())
			m.ahPreview.SetYOffset(oldAHOff)
		}
		if m.depDetailReadme != "" && (oldDepW != m.depDetailPreview.Width) {
			m.depDetailPreview.SetContent(m.renderDepDetailBody())
			m.depDetailPreview.SetYOffset(oldDepOff)
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
	case instanceRenameRequest:
		m.endBusy()
		if m.selected == nil {
			return m, nil
		}
		oldName := strings.TrimSpace(m.selected.Name)
		newName := strings.TrimSpace(msg.newName)
		appsDir := appsDirFromConfig(m.params.Config)
		inst, err := instances.Rename(m.params.RepoRoot, appsDir, oldName, newName)
		if err != nil {
			return m, func() tea.Msg { return errMsg{err} }
		}
		_ = renameDepMetaInstanceDir(m.params.RepoRoot, oldName, newName)
		m.selected = &inst
		// Refresh list + chart.
		return m, tea.Batch(m.beginBusy("Reloading"), m.reloadInstancesCmd(), m.beginBusy("Loading chart"), m.loadChartCmd(inst))
	case instanceDeleteRequest:
		m.endBusy()
		if m.selected == nil {
			return m, nil
		}
		name := strings.TrimSpace(m.selected.Name)
		appsDir := appsDirFromConfig(m.params.Config)
		if err := instances.Remove(m.params.RepoRoot, appsDir, name); err != nil {
			return m, func() tea.Msg { return errMsg{err} }
		}
		_ = deleteDepMetaInstanceDir(m.params.RepoRoot, name)
		m.screen = ScreenDashboard
		m.selected = nil
		return m, tea.Batch(m.beginBusy("Reloading"), m.reloadInstancesCmd())
	case chartMsg:
		m.endBusy()
		m.chart = &msg.chart
		m.depsList.SetItems(m.depsToItems(msg.chart.Dependencies))
		m.refreshInstanceView()
		return m, nil
	case depAppliedMsg:
		m.endBusy()
		m.chart = &msg.chart
		m.depsList.SetItems(m.depsToItems(msg.chart.Dependencies))
		m.addingDep = false
		m.catalogWizardAutoSyncTried = false
		m.catalogWizardSyncing = false
		m.depEditOpen = false
		m.depDetailOpen = false
		m.depStep = depStepNone
		m.modalErr = ""
		m.depConfigure = depConfigureModel{}
		m.clearStatusErr()
		m.setStatusOK("Dependency applied")
		m.refreshInstanceView()
		return m, nil
	case depAppliedAndAppliedMsg:
		m.endBusy()
		m.chart = &msg.chart
		m.depsList.SetItems(m.depsToItems(msg.chart.Dependencies))
		m.addingDep = false
		m.catalogWizardAutoSyncTried = false
		m.catalogWizardSyncing = false
		m.depEditOpen = false
		if m.depDetailOpen {
			// Keep modal open and refresh its data using the updated chart.
			for _, d := range msg.chart.Dependencies {
				if d.Name == m.depDetailDep.Name {
					m.depDetailDep = d
					break
				}
			}
			m.depDetailLoading = true
			m.modalErr = ""
			m.depDetailPreview.SetContent(m.renderDepDetailBody())
			cmds := []tea.Cmd{m.beginBusy("Loading"), m.loadDepDetailPreviewsCmd(m.depDetailDep)}
			if !strings.HasPrefix(m.depDetailDep.Repository, "oci://") {
				cmds = append(cmds, m.loadDepDetailVersionsCmd(m.depDetailDep))
			}
			m.depStep = depStepNone
			m.clearStatusErr()
			m.setStatusOK("Dependency applied")
			m.refreshInstanceView()
			return m, tea.Batch(cmds...)
		}
		m.depStep = depStepNone
		m.modalErr = ""
		m.clearStatusErr()
		m.setStatusOK("Dependency applied")
		m.refreshInstanceView()
		return m, nil
	case catalogSyncDoneMsg:
		m.endBusy()
		m.catalogWizardSyncing = false
		if msg.err != nil {
			// Prefer showing the error inside the modal when sync was triggered from the wizard.
			if m.addingDep {
				m.modalErr = msg.err.Error()
			} else {
				m.setStatusErr(msg.err.Error())
			}
			return m, nil
		}
		m.clearStatusErr()
		m.setStatusOK("Catalog synced")
		if m.addingDep {
			m.modalErr = ""
		}
		// Refresh catalog list in case we're in the add-dep wizard.
		return m, tea.Batch(m.beginBusy("Loading catalog"), m.loadCatalogCmd())
	case depPresetsSyncDoneMsg:
		m.endBusy()
		if msg.err != nil {
			m.setStatusErr(msg.err.Error())
			return m, nil
		}
		m.clearStatusErr()
		m.setStatusOK("Presets synced")
		m.refreshValuesList()
		m.refreshInstanceView()
		// If the dep detail modal is open for this dependency, reload sets.
		if m.depDetailOpen && yamlchart.DependencyID(m.depDetailDep) == yamlchart.DependencyID(msg.dep) {
			m.depDetailSetsLoading = true
			m.depDetailPreview.SetContent(m.renderDepDetailBody())
			return m, tea.Batch(m.beginBusy("Loading sets"), m.loadDepDetailSetsCmd(msg.dep))
		}
		return m, nil
	case catalogSetsMsg:
		m.endBusy()
		m.catalogSetsLoading = false
		if msg.err != nil {
			m.modalErr = msg.err.Error()
			m.catalogSetList.SetItems(nil)
			return m, nil
		}
		// Ensure we're still on the matching entry.
		if m.catalogDetailEntry == nil || m.catalogDetailEntry.Entry.ID != msg.entry.Entry.ID {
			return m, nil
		}
		items := make([]list.Item, 0, len(msg.sets))
		for _, c := range msg.sets {
			items = append(items, setChoiceItem{C: c})
		}
		m.catalogSetList.SetItems(items)
		m.catalogSetList.Select(0)
		return m, nil
	case depDetailSetsMsg:
		m.endBusy()
		m.depDetailSetsLoading = false
		if msg.Err != nil {
			// Non-fatal: show error and keep the rest of the modal usable.
			m.modalErr = msg.Err.Error()
			m.depDetailSets.SetItems(nil)
			m.depDetailPreview.SetContent(m.renderDepDetailBody())
			return m, nil
		}
		// Ignore stale loads.
		if !m.depDetailOpen || yamlchart.DependencyID(m.depDetailDep) != msg.ID {
			return m, nil
		}
		m.modalErr = ""
		items := make([]list.Item, 0, len(msg.Sets))
		for _, c := range msg.Sets {
			items = append(items, setChoiceForDepItem{C: c})
		}
		m.depDetailSets.SetItems(items)
		m.depDetailSets.Select(0)
		m.depDetailPreview.SetContent(m.renderDepDetailBody())
		return m, nil
	case depVersionValidatedMsg:
		m.endBusy()
		// Keep modal open; close only after apply succeeds.
		return m, tea.Batch(m.beginBusy("Applying"), m.applyDependencyAndApplyInstanceCmd(msg.dep))
	case depAliasAppliedMsg:
		m.endBusy()
		m.chart = &msg.chart
		m.depsList.SetItems(m.depsToItems(msg.chart.Dependencies))
		// Keep dep detail modal pointed at the updated dependency (alias affects depID).
		if m.depDetailOpen {
			m.depDetailDep = msg.dep
			m.depDetailAliasInput.SetValue(strings.TrimSpace(msg.dep.Alias))
			m.depDetailAliasInput.Blur()
		}
		m.modalErr = ""
		m.depDetailDeleteConfirm = false
		m.clearStatusErr()
		m.setStatusOK("Saved")
		m.refreshValuesList()
		m.depDetailPreview.SetContent(m.renderDepDetailBody())
		return m, nil
	case regenDoneMsg:
		m.endBusy()
		m.clearStatusErr()
		m.setStatusOK("Values regenerated")
		m.refreshValuesList()
		if m.valuesPreviewOpen {
			return m, m.loadValuesPreviewCmd(m.valuesPreviewPath)
		}
		m.refreshInstanceView()
		return m, nil
	case valuesPreviewLoadedMsg:
		// Ignore stale preview loads.
		if !m.valuesPreviewOpen || msg.path != m.valuesPreviewPath {
			return m, nil
		}
		m.valuesPreview.SetContent(msg.content)
		return m, nil
	case editValuesDoneMsg:
		// After editing instance values, always regenerate merged values.yaml.
		if m.screen == ScreenInstance && !m.addingDep {
			return m, tea.Batch(m.beginBusy("Regenerating values"), m.regenMergedValuesCmd())
		}
		return m, nil
	case appliedMsg:
		m.endBusy()
		m.clearStatusErr()
		m.setStatusOK("Applied")
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
	case applyDoneMsg:
		// Ignore stale apply results.
		if msg.applyID != m.applyID {
			return m, nil
		}
		m.endBusy()
		m.applyOpen = false
		m.applyCancelConfirm = false
		m.applyCancelRequested = false
		if m.applyCancel != nil {
			m.applyCancel()
			m.applyCancel = nil
		}
		if msg.err != nil {
			m.modalErr = msg.err.Error()
			m.setStatusErr(msg.err.Error())
			return m, nil
		}
		if msg.chart != nil {
			m.chart = msg.chart
			m.depsList.SetItems(m.depsToItems(msg.chart.Dependencies))
		}
		m.addingDep = false
		m.catalogWizardAutoSyncTried = false
		m.catalogWizardSyncing = false
		m.depStep = depStepNone
		m.modalErr = ""
		m.clearStatusErr()
		m.setStatusOK("Dependency applied")
		m.refreshInstanceView()
		return m, nil
	case sourcesSavedMsg:
		m.endBusy()
		if msg.err != nil {
			m.sourcesErr = msg.err.Error()
			return m, nil
		}
		m.sourcesErr = ""
		m.sourcesOpen = false
		m.sourcesName.Blur()
		m.sourcesGit.Blur()
		m.sourcesPlat.Blur()
		m.params.Config = msg.cfg
		// UX: if the user is in the add-dep wizard and just configured a source,
		// sync immediately so the catalog becomes usable without leaving the wizard.
		if m.addingDep {
			if hasCatalogEnabledSources(m.params.Config) {
				m.catalogWizardAutoSyncTried = true
				m.catalogWizardSyncing = true
				m.modalErr = ""
				return m, tea.Batch(m.beginBusy("Syncing catalog"), m.catalogSyncCmd())
			}
		}
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
		m.ahValues = highlightYAMLForDisplay(msg.values)
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
		m.setVersionsList(msg.Versions, versionsTargetDepEdit)
		return m, nil
	case depDetailPreviewsMsg:
		m.endBusy()
		if !m.depDetailOpen {
			return m, nil
		}
		if yamlchart.DependencyID(m.depDetailDep) != msg.ID {
			return m, nil
		}
		m.depDetailReadme = msg.readme
		m.depDetailDefaultValues = highlightYAMLForDisplay(msg.defaultValues)
		m.depDetailSchemaRaw = msg.schema

		// Initialize Configure form model.
		if m.selected != nil {
			m.depConfigure.Reset(string(msg.ID), m.selected.Path)
			// Load existing overrides from values.instance.yaml under depID.
			existing := readDepOverrideFromInstance(m.selected.Path, string(msg.ID))
			m.depConfigure.Load(msg.schema, existing)
		}
		m.depDetailLoading = false
		m.depDetailPreview.SetContent(m.renderDepDetailBody())
		return m, nil
	case depDetailVersionsMsg:
		m.endBusy()
		if !m.depDetailOpen {
			return m, nil
		}
		if yamlchart.DependencyID(m.depDetailDep) != msg.ID {
			return m, nil
		}
		m.setVersionsList(msg.Versions, versionsTargetDepDetail)
		m.depDetailVersionsLoading = false
		m.depDetailPreview.SetContent(m.renderDepDetailBody())
		return m, nil
	case versionsRefreshResultMsg:
		// Background refresh result: update cache + update the open modal when still relevant.
		_ = m.handleVersionsRefreshResult(msg)
		return m, nil
	case versionsRefreshTickMsg:
		// Periodic background refresh for watched deps.
		cmds := []tea.Cmd{}
		for key, w := range m.versionsWatched {
			if m.versionsInFlight[key] {
				continue
			}
			if !helmutil.VersionsCacheStale(w.lastRefresh, versionsCacheTTL, msg.now.UTC()) {
				continue
			}
			m.versionsInFlight[key] = true
			dep := yamlchart.Dependency{Repository: w.repoURL, Name: w.chartName}
			cmds = append(cmds, m.refreshVersionsCmd(dep, versionsTargetBackground))
		}
		// Re-arm tick.
		cmds = append(cmds, tea.Tick(versionsRefreshInterval, func(t time.Time) tea.Msg { return versionsRefreshTickMsg{now: t} }))
		return m, tea.Batch(cmds...)
	case noopMsg:
		m.endBusy()
		return m, nil
	case errMsg:
		m.endBusy()
		m.setStatusErr(msg.err.Error())
		if m.depDetailOpen {
			m.modalErr = msg.err.Error()
			m.depDetailLoading = false
			m.depDetailPreview.SetContent(m.renderDepDetailBody())
			return m, nil
		}
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
		// Otherwise: keep user on the current screen; surface the error via footer.
		return m, nil
	case tea.KeyMsg:
		// Apply overlay has highest priority and blocks all other input.
		if m.applyOpen {
			// Cancel confirmation flow.
			if m.applyCancelConfirm {
				if msg.String() == "y" || msg.String() == "Y" {
					m.applyCancelConfirm = false
					m.applyCancelRequested = true
					if m.applyCancel != nil {
						m.applyCancel()
					}
					return m, nil
				}
				if msg.String() == "n" || msg.String() == "N" || msg.Type == tea.KeyEsc {
					m.applyCancelConfirm = false
					return m, nil
				}
				return m, nil
			}
			// Esc requests cancel.
			if msg.Type == tea.KeyEsc {
				m.applyCancelConfirm = true
				return m, nil
			}
			// Ignore all other keys while applying.
			return m, nil
		}

		// Dep detail modal has the highest priority while open.
		if m.depDetailOpen {
			return m.depDetailUpdate(msg)
		}
		// Dep actions menu modal has priority over other input.
		if m.depActionsOpen {
			return m.depActionsUpdate(msg)
		}
		// Dep version editor modal has priority over global shortcuts.
		if m.depEditOpen {
			return m.depEditUpdate(msg)
		}
		// Command palette has priority over global shortcuts.
		if m.paletteOpen {
			return m.paletteUpdate(msg)
		}
		// Sources modal has priority over global shortcuts.
		if m.sourcesOpen {
			return m.sourcesUpdate(msg)
		}
		// Values preview modal has priority over global shortcuts.
		if m.valuesPreviewOpen {
			if msg.Type == tea.KeyEsc {
				m.valuesPreviewOpen = false
				m.valuesPreviewPath = ""
				m.valuesPreview.SetContent("")
				return m, nil
			}
			var cmd tea.Cmd
			m.valuesPreview, cmd = m.valuesPreview.Update(msg)
			return m, cmd
		}

		// Always honor hard quit keys, even when a text input is focused.
		// Otherwise the user can get stuck in inputs (e.g. Artifact Hub query).
		if msg.Type == tea.KeyCtrlC || msg.String() == "ctrl+c" {
			m.skipWindowTitleOnce = true
			return m, tea.Quit
		}
		// Bubble Tea may or may not decode Ctrl+D as a dedicated key type depending
		// on terminal/reader; matching by string keeps it robust.
		if msg.Type == tea.KeyCtrlD || msg.String() == "ctrl+d" {
			m.skipWindowTitleOnce = true
			return m, tea.Quit
		}

		// Always handle Esc/back before deferring to focused inputs.
		// Esc should first clear any active list filtering, then navigate back.
		if key.Matches(msg, m.keys.Back) || msg.Type == tea.KeyEsc {
			// First: if any filter is active, clear it.
			if m.clearAnyActiveFilter() {
				return m, nil
			}
			// Next: when no modal is open, Esc dismisses a persistent error.
			if msg.Type == tea.KeyEsc && m.noModalOpen() && strings.TrimSpace(m.statusErr) != "" {
				m.clearStatusErr()
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
					m.catalogWizardAutoSyncTried = false
					m.catalogWizardSyncing = false
					m.depStep = depStepNone
					m.modalErr = ""
					return m, nil
		case depStepCatalog, depStepCatalogDetail, depStepCatalogCollision, depStepAHQuery, depStepArbitrary:
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
					m.catalogWizardAutoSyncTried = false
					m.catalogWizardSyncing = false
					m.depStep = depStepNone
					m.modalErr = ""
					return m, nil
				}
			}
			if m.instanceManageOpen {
				m.instanceManageOpen = false
				m.instanceManageConfirm = false
				m.instanceManageName.Blur()
				return m, nil
			}
			if m.screen == ScreenInstance {
				m.screen = ScreenDashboard
				m.selected = nil
				return m, nil
			}
		}

		// Wizard helpers (only when the add-dep wizard is open and we're in Catalog step).
		// This allows recovery from "no catalog" without having to open the command palette.
		if m.addingDep && m.depStep == depStepCatalog && len(m.catalogEntries) == 0 {
			switch msg.String() {
			case "c", "C":
				m.openSourcesModal()
				return m, nil
			case "s", "S":
				if !m.catalogWizardSyncing && hasCatalogEnabledSources(m.params.Config) {
					m.catalogWizardAutoSyncTried = true
					m.catalogWizardSyncing = true
					m.modalErr = ""
					return m, tea.Batch(m.beginBusy("Syncing catalog"), m.catalogSyncCmd())
				}
			}
		}

		// If a text input is focused or a list filter is active, do not treat
		// characters as global shortcuts.
		if m.inputCapturesKeys() {
			break
		}

		// Instance tab actions.
		if m.screen == ScreenInstance && !m.addingDep && m.activeTab == InstanceTabInstance {
			if msg.String() == "r" || msg.String() == "R" {
				m.instanceManageOpen = true
				m.instanceManageMode = instanceManageRename
				m.instanceManageConfirm = false
				cur := ""
				if m.selected != nil {
					cur = m.selected.Name
				}
				m.instanceManageName.SetValue(cur)
				m.instanceManageName.Focus()
				return m, nil
			}
			if msg.String() == "d" || msg.String() == "D" {
				m.instanceManageOpen = true
				m.instanceManageMode = instanceManageDelete
				m.instanceManageConfirm = false
				m.instanceManageName.Blur()
				return m, nil
			}
		}

		if key.Matches(msg, m.keys.Quit) {
			m.skipWindowTitleOnce = true
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
				// Values tab: only allow editing values.instance.yaml.
				if m.activeTab == InstanceTabValues {
					if it := m.valuesList.SelectedItem(); it != nil {
						if vf, ok := it.(valuesFileItem); ok && string(vf) == "values.instance.yaml" {
							return m, m.editInstanceValuesCmd()
						}
					}
					m.setStatusErr("Select values.instance.yaml in the list to edit")
					return m, nil
				}
				return m, m.editInstanceValuesCmd()
			}
		}
		if key.Matches(msg, m.keys.Apply) {
			if m.screen == ScreenInstance && !m.addingDep {
				return m, tea.Batch(m.beginBusy("Applying"), m.applyInstanceCmd(false))
			}
		}
		// Deps tab actions.
		if m.screen == ScreenInstance && !m.addingDep && m.activeTab == InstanceTabDeps {
			// Actions menu.
			if msg.String() == "x" || msg.String() == "X" {
				return m.openDepActionsSelected()
			}
			// Delete dependency.
			if msg.String() == "d" || msg.String() == "D" {
				// Route through the same cleanup pipeline as the Dependency tab delete.
				it := m.depsList.SelectedItem()
				if it == nil {
					return m, nil
				}
				di, ok := it.(depItem)
				if !ok {
					return m, func() tea.Msg { return errMsg{fmt.Errorf("unexpected dependency item type")} }
				}
				m.depDetailDep = di.Dep
				return m, tea.Batch(m.beginBusy("Updating"), m.deleteDepFromDetailCmd())
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
				m.catalogWizardAutoSyncTried = false
				m.catalogWizardSyncing = false
				m.depStep = depStepChooseSource
				m.modalErr = ""
				return m, tea.Batch(m.beginBusy("Loading catalog"), m.loadCatalogCmd())
			}
		}
		// Back/Esc is handled above, before input capture, so it works inside text inputs.
		if key.Matches(msg, m.keys.Open) {
			if m.screen == ScreenDashboard {
				if it, ok := m.instList.SelectedItem().(instanceItem); ok {
					inst := instances.Instance(it)
					m.selected = &inst
						m.screen = ScreenInstance
						m.activeTab = 0 // Dependencies is first tab
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
				if m.activeTab == InstanceTabValues {
					m.refreshValuesList()
				}
				m.refreshInstanceView()
				return m, nil
			}
			if key.Matches(msg, m.keys.TabRight) {
				m.activeTab = (m.activeTab + 1) % len(m.tabNames)
				if m.activeTab == InstanceTabValues {
					m.refreshValuesList()
				}
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
			m.activeTab = 0 // Dependencies is first tab
			return m, tea.Batch(
				m.beginBusy("Reloading"),
				m.reloadInstancesCmd(),
				m.beginBusy("Loading chart"),
				m.loadChartCmd(inst),
			)
		}
		return m, cmd
	}

	// Modal: instance manage (rename/delete).
	if m.instanceManageOpen {
		// Rename flow: textinput.
		if m.instanceManageMode == instanceManageRename {
			var cmd tea.Cmd
			m.instanceManageName, cmd = m.instanceManageName.Update(msg)
			if km, ok := msg.(tea.KeyMsg); ok && km.Type == tea.KeyEnter {
				name := strings.TrimSpace(m.instanceManageName.Value())
				m.instanceManageOpen = false
				m.instanceManageName.Blur()
				return m, tea.Batch(m.beginBusy("Renaming"), func() tea.Msg { return instanceRenameRequest{newName: name} })
			}
			return m, cmd
		}

		// Delete flow: confirmation.
		if m.instanceManageMode == instanceManageDelete {
			if km, ok := msg.(tea.KeyMsg); ok {
				if km.String() == "y" || km.String() == "Y" {
					m.instanceManageOpen = false
					return m, tea.Batch(m.beginBusy("Deleting"), func() tea.Msg { return instanceDeleteRequest{} })
				}
				if km.String() == "n" || km.String() == "N" {
					m.instanceManageOpen = false
					return m, nil
				}
			}
			return m, nil
		}
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
					// If there are no local catalog entries yet, attempt a one-time sync
					// to populate `.helmdex/catalog/*.yaml` from configured sources.
					if len(m.catalogEntries) == 0 && !m.catalogWizardAutoSyncTried {
						m.catalogWizardAutoSyncTried = true
						if hasCatalogEnabledSources(m.params.Config) {
							m.catalogWizardSyncing = true
							m.modalErr = ""
							return m, tea.Batch(cmd, m.beginBusy("Syncing catalog"), m.catalogSyncCmd())
						}
					}
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
					return m, func() tea.Msg { return errMsg{fmt.Errorf("unexpected catalog item type")} }
				}
				e := ci.E
				m.catalogDetailEntry = &e
				m.catalogSetsLoading = true
				m.catalogSetList.SetItems(nil)
				m.depStep = depStepCatalogDetail
				return m, tea.Batch(cmd, m.loadCatalogSetsCmd(e))
			}
			return m, cmd
		case depStepCatalogDetail:
			var cmd tea.Cmd
			m.catalogSetList, cmd = m.catalogSetList.Update(msg)
			if km, ok := msg.(tea.KeyMsg); ok {
				// Space toggles the current set.
				if km.Type == tea.KeySpace {
					if it := m.catalogSetList.SelectedItem(); it != nil {
						if si, ok := it.(setChoiceItem); ok {
							si.C.On = !si.C.On
							items := m.catalogSetList.Items()
							idx := m.catalogSetList.Index()
							if idx >= 0 && idx < len(items) {
								items[idx] = si
								m.catalogSetList.SetItems(items)
								m.catalogSetList.Select(idx)
							}
						}
					}
				}
				// 'D' toggles all defaults. Reserve lowercase 'd' for destructive actions.
				if km.String() == "D" {
					items := m.catalogSetList.Items()
					anyOff := false
					for _, it := range items {
						if si, ok := it.(setChoiceItem); ok && si.C.Default && !si.C.On {
							anyOff = true
							break
						}
					}
					for i := range items {
						if si, ok := items[i].(setChoiceItem); ok && si.C.Default {
							si.C.On = anyOff
							items[i] = si
						}
					}
					idx := m.catalogSetList.Index()
					m.catalogSetList.SetItems(items)
					m.catalogSetList.Select(idx)
				}
				// Enter confirms and applies.
				if km.Type == tea.KeyEnter {
					if m.catalogDetailEntry == nil {
						return m, cmd
					}
					dep := yamlchart.Dependency{Name: m.catalogDetailEntry.Entry.Chart.Name, Repository: m.catalogDetailEntry.Entry.Chart.Repo, Version: m.catalogDetailEntry.Entry.Version}
					setNames := selectedSetNames(m.catalogSetList.Items())
					// Persist dep source metadata.
					_ = m.writeSelectedDepSourceMeta(dep, depSourceMeta{Kind: depSourceCatalog, CatalogID: m.catalogDetailEntry.Entry.ID, CatalogSource: m.catalogDetailEntry.SourceName})
					// Collision check: same dependency name already exists.
					if m.chart != nil {
						for _, ex := range m.chart.Dependencies {
							if ex.Name == dep.Name {
								m.catalogCollisionDep = dep
								m.catalogCollisionExisting = ex
								m.catalogCollisionSets = setNames
								m.catalogCollisionChoice = collisionChoiceAlias
								m.catalogCollisionAlias.SetValue("")
								m.catalogCollisionAlias.Focus()
								m.depStep = depStepCatalogCollision
								return m, cmd
							}
						}
					}
					return m, tea.Batch(cmd, m.startApplyCmd(dep, setNames, applyOptions{Override: true, DeleteMarkersForDepID: false}))
				}
			}
			return m, cmd
		case depStepCatalogCollision:
			// Alias input needs updates.
			var cmd tea.Cmd
			m.catalogCollisionAlias, cmd = m.catalogCollisionAlias.Update(msg)
			if km, ok := msg.(tea.KeyMsg); ok {
				if km.Type == tea.KeyUp {
					m.catalogCollisionChoice = (m.catalogCollisionChoice - 1 + 3) % 3
					if m.catalogCollisionChoice == collisionChoiceAlias {
						m.catalogCollisionAlias.Focus()
					} else {
						m.catalogCollisionAlias.Blur()
					}
					return m, cmd
				}
				if km.Type == tea.KeyDown {
					m.catalogCollisionChoice = (m.catalogCollisionChoice + 1) % 3
					if m.catalogCollisionChoice == collisionChoiceAlias {
						m.catalogCollisionAlias.Focus()
					} else {
						m.catalogCollisionAlias.Blur()
					}
					return m, cmd
				}
				if km.Type == tea.KeyEnter {
					switch m.catalogCollisionChoice {
					case collisionChoiceCancel:
						m.depStep = depStepCatalogDetail
						m.catalogCollisionAlias.Blur()
						return m, cmd
					case collisionChoiceOverride:
						dep := m.catalogCollisionDep
						_ = m.writeSelectedDepSourceMeta(dep, depSourceMeta{Kind: depSourceCatalog, CatalogID: "", CatalogSource: ""})
						// Override: keep same depID (name if no alias) and delete markers for that depID.
						return m, tea.Batch(cmd, m.startApplyCmd(dep, m.catalogCollisionSets, applyOptions{Override: true, DeleteMarkersForDepID: true}))
					case collisionChoiceAlias:
						alias := strings.TrimSpace(m.catalogCollisionAlias.Value())
						if alias == "" {
							m.modalErr = "alias is required"
							return m, cmd
						}
						dep := m.catalogCollisionDep
						dep.Alias = alias
						_ = m.writeSelectedDepSourceMeta(dep, depSourceMeta{Kind: depSourceCatalog, CatalogID: "", CatalogSource: ""})
						return m, tea.Batch(cmd, m.startApplyCmd(dep, m.catalogCollisionSets, applyOptions{Override: false, DeleteMarkersForDepID: false}))
					}
				}
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
					return m, func() tea.Msg { return errMsg{fmt.Errorf("unexpected Artifact Hub item type")} }
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
							return m, func() tea.Msg { return errMsg{fmt.Errorf("unexpected version item type")} }
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
							dep := yamlchart.Dependency{Name: m.ahSelected.Name, Repository: m.ahSelected.RepositoryURL, Version: m.ahSelectedVersion}
							_ = m.writeSelectedDepSourceMeta(dep, depSourceMeta{Kind: depSourceArtifactHub})
							return m, m.applyDependencyAndApplyInstanceCmd(dep)
						}
						m.modalErr = "select a version (Enter) before adding"
						return m, cmd
					}
				}
				return m, cmd
			}

			// Non-versions tabs: allow quick add if a version is selected.
			if km, ok := msg.(tea.KeyMsg); ok {
				if km.String() == "a" || km.String() == "A" {
					if m.ahSelected != nil && m.ahSelectedVersion != "" {
						dep := yamlchart.Dependency{Name: m.ahSelected.Name, Repository: m.ahSelected.RepositoryURL, Version: m.ahSelectedVersion}
						_ = m.writeSelectedDepSourceMeta(dep, depSourceMeta{Kind: depSourceArtifactHub})
						return m, m.applyDependencyAndApplyInstanceCmd(dep)
					}
					m.modalErr = "select a version in the Versions tab first"
					return m, nil
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
					return m, func() tea.Msg { return errMsg{fmt.Errorf("unexpected version item type")} }
				}
			v := artifacthub.Version(vi)
			dep := yamlchart.Dependency{Name: m.ahSelected.Name, Repository: m.ahSelected.RepositoryURL, Version: v.Version}
			_ = m.writeSelectedDepSourceMeta(dep, depSourceMeta{Kind: depSourceArtifactHub})
			return m, m.applyDependencyDraft(dep)
		}
			return m, cmd
		case depStepArbitrary:
			// Simple focus cycling with tab.
			if km, ok := msg.(tea.KeyMsg); ok {
				if km.Type == tea.KeyTab {
					m.arbFocus = (m.arbFocus + 1) % 4
					m.arbRepo.Blur()
					m.arbName.Blur()
					m.arbVersion.Blur()
					m.arbAlias.Blur()
					switch m.arbFocus {
					case 0:
						m.arbRepo.Focus()
					case 1:
						m.arbName.Focus()
					case 2:
						m.arbVersion.Focus()
					case 3:
						m.arbAlias.Focus()
					}
				}
				if km.Type == tea.KeyEnter {
				dep := yamlchart.Dependency{Name: strings.TrimSpace(m.arbName.Value()), Repository: strings.TrimSpace(m.arbRepo.Value()), Version: strings.TrimSpace(m.arbVersion.Value()), Alias: strings.TrimSpace(m.arbAlias.Value())}
				_ = m.writeSelectedDepSourceMeta(dep, depSourceMeta{Kind: depSourceArbitrary})
				return m, m.applyDependencyDraft(dep)
			}
			}
			var cmds []tea.Cmd
			var cmd tea.Cmd
			m.arbRepo, cmd = m.arbRepo.Update(msg)
			cmds = append(cmds, cmd)
			m.arbName, cmd = m.arbName.Update(msg)
			cmds = append(cmds, cmd)
			m.arbVersion, cmd = m.arbVersion.Update(msg)
			cmds = append(cmds, cmd)
			m.arbAlias, cmd = m.arbAlias.Update(msg)
			cmds = append(cmds, cmd)
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
		if m.activeTab == InstanceTabDeps && !m.addingDep {
			var cmd tea.Cmd
			m.depsList, cmd = m.depsList.Update(msg)
			if km, ok := msg.(tea.KeyMsg); ok && km.Type == tea.KeyEnter {
				// When the deps list is filtering, Enter applies the filter.
				if m.depsList.FilterState() != list.Filtering {
					return m.openDepDetailSelected()
				}
			}
			return m, cmd
		}
		// Values tab uses its own list.
		if m.activeTab == InstanceTabValues && !m.addingDep {
			var cmd tea.Cmd
			m.valuesList, cmd = m.valuesList.Update(msg)
			if km, ok := msg.(tea.KeyMsg); ok && km.Type == tea.KeyEnter {
				if m.valuesList.FilterState() != list.Filtering {
					it := m.valuesList.SelectedItem()
					if vf, ok := it.(valuesFileItem); ok {
						m.valuesPreviewOpen = true
						m.valuesPreviewPath = string(vf)
						m.valuesPreview.SetContent(styleMuted.Render("Loading…"))
						return m, tea.Batch(cmd, m.loadValuesPreviewCmd(m.valuesPreviewPath))
					}
				}
			}
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
		m.skipWindowTitleOnce = true
		return m, tea.Quit
	}
	cmd, didEnter := m.palette.Update(msg)
	if didEnter {
		it, ok := m.palette.selected()
		if ok {
			switch it.ID {
				case palQuit:
					m.skipWindowTitleOnce = true
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
			case palSources:
				m.openSourcesModal()
				return m, nil
			case palCatalogSync:
				m.paletteOpen = false
				return m, tea.Batch(m.beginBusy("Syncing catalog"), m.catalogSyncCmd())
			}
		}
	}
	return m, cmd
}

func (m AppModel) sourcesUpdate(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Close.
	if msg.Type == tea.KeyEsc {
		m.sourcesOpen = false
		m.sourcesErr = ""
		m.sourcesName.Blur()
		m.sourcesGit.Blur()
		m.sourcesRef.Blur()
		m.sourcesPlat.Blur()
		return m, nil
	}
	if msg.Type == tea.KeyCtrlC || msg.String() == "ctrl+c" {
		return m, tea.Quit
	}

	// Focus cycling.
	if msg.Type == tea.KeyTab {
		m.sourcesFocus = (m.sourcesFocus + 1) % 4
		m.sourcesName.Blur()
		m.sourcesGit.Blur()
		m.sourcesRef.Blur()
		m.sourcesPlat.Blur()
		switch m.sourcesFocus {
		case 0:
			m.sourcesName.Focus()
		case 1:
			m.sourcesGit.Focus()
		case 2:
			m.sourcesRef.Focus()
		case 3:
			m.sourcesPlat.Focus()
		}
		return m, nil
	}
	if msg.Type == tea.KeyShiftTab {
		m.sourcesFocus = (m.sourcesFocus - 1 + 4) % 4
		m.sourcesName.Blur()
		m.sourcesGit.Blur()
		m.sourcesRef.Blur()
		m.sourcesPlat.Blur()
		switch m.sourcesFocus {
		case 0:
			m.sourcesName.Focus()
		case 1:
			m.sourcesGit.Focus()
		case 2:
			m.sourcesRef.Focus()
		case 3:
			m.sourcesPlat.Focus()
		}
		return m, nil
	}

	// Save.
	if msg.Type == tea.KeyEnter {
		name := m.sourcesName.Value()
		gitURL := m.sourcesGit.Value()
		gitRef := m.sourcesRef.Value()
		platform := m.sourcesPlat.Value()
		m.sourcesErr = ""
		return m, tea.Batch(m.beginBusy("Saving"), m.saveSourcesCmd(name, gitURL, gitRef, platform))
	}

	var cmds []tea.Cmd
	var cmd tea.Cmd
	m.sourcesName, cmd = m.sourcesName.Update(msg)
	cmds = append(cmds, cmd)
	m.sourcesGit, cmd = m.sourcesGit.Update(msg)
	cmds = append(cmds, cmd)
	m.sourcesRef, cmd = m.sourcesRef.Update(msg)
	cmds = append(cmds, cmd)
	m.sourcesPlat, cmd = m.sourcesPlat.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func (m AppModel) isAnyFilterActive() bool {
	// Note: list.FilterState distinguishes between:
	// - Unfiltered
	// - Filtering (user typing)
	// - FilterApplied
	// The footer indicator should be true for both Filtering and FilterApplied.
	if m.instList.FilterState() != list.Unfiltered {
		return true
	}
	if m.catalogList.FilterState() != list.Unfiltered {
		return true
	}
	if m.depSource.FilterState() != list.Unfiltered {
		return true
	}
	if m.depsList.FilterState() != list.Unfiltered {
		return true
	}
	if m.valuesList.FilterState() != list.Unfiltered {
		return true
	}
	// Version lists in modals.
	if m.depEditVersions.FilterState() != list.Unfiltered {
		return true
	}
	if m.depDetailVersions.FilterState() != list.Unfiltered {
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

	// Persistent navigation context (instance breadcrumbs) under the header.
	breadcrumb := renderBreadcrumbBar(m)

	var body string
	if m.helpOpen {
		// Help fully replaces the body.
		body = renderHelpOverlay(m)
	} else if m.paletteOpen {
		// Palette is a full-body modal so it can't be pushed off-screen by the body.
		body = m.palette.View()
	} else if m.sourcesOpen {
		// Sources modal is full-body for the same reason as the palette.
		body = m.renderSourcesModal()
	} else if m.instanceManageOpen {
		body = m.renderInstanceManageModal()
	} else if m.depActionsOpen {
		// Dep actions is a full-body modal.
		body = renderDepActionsModal(m)
	} else if m.depDetailOpen {
		// Dependency detail modal should be full-screen (like dep version editor).
		body = renderDepDetailModal(m)
	} else if m.depEditOpen {
		// The dep version editor should be unmissable. Rendering it as a stacked
		// block above a long instance view can scroll it off-screen in AltScreen.
		// Render it as the full body instead.
		body = renderDepEditModal(m)
	} else if m.valuesPreviewOpen {
		// Values preview is a full-screen modal.
		body = renderValuesPreviewModal(m)
	} else if m.applyOpen {
		body = m.renderApplyOverlay()
	} else {
		body = m.currentBodyView()
	}

	contextHelp := lipgloss.NewStyle().Faint(true).Render(m.contextHelpLine())
	status := renderFooterStatusLine(m)

	return base.Render(strings.TrimRight(header+"\n"+breadcrumb+"\n\n"+body+"\n\n"+contextHelp+"\n"+status, "\n"))
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
		prefix := tabsLine + "\n\n"
		if m.activeTab == InstanceTabDeps {
			return prefix + m.depsList.View()
		}
		if m.activeTab == InstanceTabValues {
			return prefix + m.valuesList.View()
		}
		if m.activeTab == InstanceTabInstance {
			lines := []string{}
			lines = append(lines, lipgloss.NewStyle().Bold(true).Render(withIcon(iconSettings, "Instance")))
			if m.selected != nil {
				lines = append(lines, styleMuted.Render("Name: "+m.selected.Name))
				lines = append(lines, styleMuted.Render("Path: "+m.selected.Path))
			}
			lines = append(lines, "")
			lines = append(lines, styleMuted.Render("r: rename • d: delete"))
			return prefix + strings.Join(lines, "\n")
		}
		return prefix + m.content.View()
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

func (m AppModel) renderSourcesModal() string {
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2)
	box = box.Height(modalMaxHeight(m))
	lines := []string{}
	lines = append(lines, lipgloss.NewStyle().Bold(true).Render(withIcon(iconCmd, "Configure sources")))
	if strings.TrimSpace(m.sourcesErr) != "" {
		lines = append(lines, styleErrStrong.Render(withIcon(iconErr, "Error:")+" "+m.sourcesErr))
	}
	lines = append(lines, "")
	lines = append(lines, m.sourcesName.View())
	lines = append(lines, m.sourcesGit.View())
	lines = append(lines, m.sourcesRef.View())
	lines = append(lines, m.sourcesPlat.View())
	lines = append(lines, "")
	lines = append(lines, styleMuted.Render("tab: next field • enter: save • esc: close"))
	return box.Render(strings.Join(lines, "\n"))
}

func (m AppModel) renderInstanceManageModal() string {
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2)
	box = box.Height(modalMaxHeight(m))
	if !m.instanceManageOpen {
		return ""
	}
	instName := ""
	if m.selected != nil {
		instName = m.selected.Name
	}

	switch m.instanceManageMode {
	case instanceManageRename:
		header := lipgloss.NewStyle().Bold(true).Render(withIcon(iconRename, "Rename instance"))
		if strings.TrimSpace(instName) != "" {
			header += "\n" + styleMuted.Render("Current: "+instName)
		}
		body := m.instanceManageName.View() + "\n\n" + styleMuted.Render("enter rename • esc cancel")
		return box.Render(header + "\n\n" + body)
	case instanceManageDelete:
		header := lipgloss.NewStyle().Bold(true).Render(withIcon(iconTrash, "Delete instance"))
		if strings.TrimSpace(instName) != "" {
			header += "\n" + styleMuted.Render(instName)
		}
		body := styleErrStrong.Render("This will delete the instance directory and its depmeta.") + "\n\n" + styleMuted.Render("y: delete • n: cancel • esc: cancel")
		return box.Render(header + "\n\n" + body)
	default:
		return box.Render(styleMuted.Render("Unknown instance action"))
	}
}

func (m AppModel) renderApplyOverlay() string {
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2)
	box = box.Height(modalMaxHeight(m))
	lines := []string{}
	lines = append(lines, lipgloss.NewStyle().Bold(true).Render(withIcon(iconBusy, "Applying")))
	if strings.TrimSpace(m.busyLabel) != "" {
		lines = append(lines, styleMuted.Render(m.spin.View()+" "+m.busyLabel))
	} else {
		lines = append(lines, styleMuted.Render(m.spin.View()+" Working…"))
	}
	lines = append(lines, "")
	if m.applyCancelConfirm {
		lines = append(lines, styleErrStrong.Render("Cancel apply?"))
		lines = append(lines, styleMuted.Render("This is best-effort; it may still finish in the background."))
		lines = append(lines, "")
		lines = append(lines, styleMuted.Render("y: cancel • n: continue"))
	} else if m.applyCancelRequested {
		lines = append(lines, styleMuted.Render("Cancelling…"))
	} else {
		lines = append(lines, styleMuted.Render("esc: cancel"))
	}
	return box.Render(strings.Join(lines, "\n"))
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
	if m.sourcesOpen {
		return "tab: next field • shift+tab: prev field • enter: save • esc: close"
	}
	if m.creating {
		return "enter create • esc cancel"
	}
	if m.instanceManageOpen {
		if m.instanceManageMode == instanceManageRename {
			return "enter rename • esc cancel"
		}
		return "y delete • n cancel • esc cancel"
	}
	if m.screen == ScreenDashboard {
		return "/ filter • enter open • n new • m commands • q quit"
	}
	if m.screen == ScreenInstance {
		if m.addingDep {
			// Add-dependency wizard is step-aware so the help matches what Enter does.
			switch m.depStep {
			case depStepChooseSource:
				return "esc back • ↑/↓ select • enter: next"
			case depStepCatalog:
				if len(m.catalogEntries) == 0 {
					if m.catalogWizardSyncing {
						return "esc back"
					}
					return "s: sync catalog • c: configure sources • esc back"
				}
				return "/ filter • ↑/↓ select • enter: next • esc back"
		case depStepCatalogDetail:
			if m.catalogSetsLoading {
				return "esc back"
			}
			if len(m.catalogSetList.Items()) == 0 {
				return "enter: add+apply • esc back"
			}
			return "space toggle • D toggle defaults • enter: add+apply • esc back"
			case depStepCatalogCollision:
				return "↑/↓ select • enter confirm • esc back"
			case depStepAHQuery:
				return "type query • enter: search • esc back"
			case depStepAHResults:
				return "↑/↓ select • enter: details • esc back"
			case depStepAHVersions:
				return "↑/↓ select • enter: draft • esc back"
			case depStepAHDetail:
				// Detail tabs: Enter loads README/values in Versions tab.
				if m.ahDetailTab == 2 {
					// Filtering is intentionally disabled in the versions list here.
					return "←/→ tabs • ↑/↓ select • enter: load details • a add • esc back"
				}
				return "←/→ tabs • a add • esc back"
			case depStepArbitrary:
				return "tab next field • enter: draft • esc back"
			default:
				return "esc back"
			}
		}
	if m.depDetailOpen {
		// Dependency detail modal: help is tab-aware so Enter is accurate.
		activeKind := depDetailTabValues
		if m.depDetailTab >= 0 && m.depDetailTab < len(m.depDetailTabKinds) {
			activeKind = m.depDetailTabKinds[m.depDetailTab]
		}
		versionsTab := len(m.depDetailTabNames) - 1
		if m.depDetailTab == versionsTab {
			switch m.depDetailMode {
			case depEditModeManual:
				return "←/→ tabs • enter: apply version • esc close"
			default:
				return "←/→ tabs • / filter • enter: apply version • esc close"
			}
		}
		switch activeKind {
		case depDetailTabDependency:
			if m.depDetailDeleteConfirm {
				return "y delete • n cancel • esc cancel"
			}
			if m.depDetailAliasInput.Focused() {
				return "enter: apply alias • esc cancel"
			}
			return "←/→ tabs • enter: edit alias • d remove • esc close"
		case depDetailTabSets:
			return "←/→ tabs • space toggle • enter: apply • esc close"
		case depDetailTabValues:
			// Configure tab.
			if m.depConfigure.editing {
				return "enter: apply edit • esc cancel edit"
			}
			return "←/→ tabs • ↑/↓ move • enter edit/toggle • s save • esc close"
		default:
			return "←/→ tabs • esc close"
		}
	}
	if m.depActionsOpen {
		return "esc close • ↑/↓ select • enter run"
	}
	if m.activeTab == InstanceTabDeps {
		return "←/→ tabs • x actions • d remove • v version • u upgrade • a add dep • m commands • esc back • q quit"
	}
	if m.activeTab == InstanceTabInstance {
		return "←/→ tabs • r rename • d delete • esc back • q quit"
	}
		return "←/→ tabs • a add dep • e edit values • p apply • r regen values • m commands • esc back • q quit"
	}
	return shortHelp(m.keys)
}

type instanceItem instances.Instance

func (i instanceItem) FilterValue() string { return i.Name }

// Implement list.DefaultItem so bubbles/list default delegate can render it.
func (i instanceItem) Title() string { return withIcon(iconInstance, i.Name) }
func (i instanceItem) Description() string {
	if i.Path == "" {
		return ""
	}
	return i.Path
}

type sourceItem string

func (s sourceItem) FilterValue() string { return string(s) }

// Implement list.DefaultItem so bubbles/list default delegate can render it.
func (s sourceItem) Title() string {
	// Keep filter value plain; decorate title only.
	switch string(s) {
	case "Predefined catalog":
		return withIcon(iconCatalog, string(s))
	case "Artifact Hub":
		return withIcon(iconAH, string(s))
	case "Arbitrary":
		return withIcon(iconCustom, string(s))
	default:
		return string(s)
	}
}
func (s sourceItem) Description() string { return "" }

type catalogItem catalog.Entry

// Wrap catalog.Entry (which has a `Description` field) to avoid method/field name collisions.
type catalogListItem struct{ E catalog.EntryWithSource }

func (c catalogListItem) Title() string { return withIcon(iconCatalog, c.E.Entry.ID) }
func (c catalogListItem) Description() string {
	return c.E.Entry.Chart.Repo + "@" + c.E.Entry.Version
}
func (c catalogListItem) FilterValue() string {
	return c.E.Entry.ID + " " + c.E.Entry.Chart.Name + " " + c.E.Entry.Chart.Repo
}

// Wrap PackageSummary (which has a `Description` field) to avoid method/field name collisions.
type ahResultItem struct{ P artifacthub.PackageSummary }

func (a ahResultItem) Title() string {
	if a.P.DisplayName != "" {
		return withIcon(iconAH, a.P.DisplayName)
	}
	return withIcon(iconAH, a.P.Name)
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
func (a ahResultItem) FilterValue() string {
	return a.P.Name + " " + a.P.DisplayName + " " + a.P.RepositoryName
}

type ahVersionItem artifacthub.Version

func (v ahVersionItem) Title() string       { return withIcon(iconVersions, v.Version) }
func (v ahVersionItem) Description() string { return "" }
func (v ahVersionItem) FilterValue() string { return v.Version }

func (m AppModel) depsToItems(deps []yamlchart.Dependency) []list.Item {
	items := make([]list.Item, 0, len(deps))
	for _, d := range deps {
		var meta depSourceMeta
		metaOK := false
		if m.selected != nil && strings.TrimSpace(m.selected.Name) != "" {
			id := yamlchart.DependencyID(d)
			if m2, ok := readDepSourceMeta(m.params.RepoRoot, m.selected.Name, id); ok {
				meta = m2
				metaOK = true
			}
		}
		items = append(items, depItem{Dep: d, Source: meta, SourceOK: metaOK})
	}
	return items
}

type depItem struct {
	Dep      yamlchart.Dependency
	Source   depSourceMeta
	SourceOK bool
}

func (d depItem) Title() string {
	dd := d.Dep
	id := string(yamlchart.DependencyID(dd))
	name := strings.TrimSpace(dd.Name)
	alias := strings.TrimSpace(dd.Alias)
	ver := strings.TrimSpace(dd.Version)

	// Title is the scan-friendly identity line: depID + name/alias + version.
	// (Repo/source are pushed into Description.)
	parts := []string{}
	if id != "" {
		parts = append(parts, withIcon(iconDeps, id))
	}
	if alias != "" {
		// Show both for clarity when alias is used.
		if name != "" && alias != name {
			parts = append(parts, name+" as "+alias)
		} else {
			parts = append(parts, alias)
		}
	} else if name != "" {
		parts = append(parts, name)
	}
	if ver != "" {
		parts = append(parts, "@ "+ver)
	}
	return strings.Join(parts, "  ")
}

func (d depItem) Description() string {
	dd := d.Dep
	parts := []string{}
	if tag, _ := depSourceTagAndLabel(d.Source, d.SourceOK); strings.TrimSpace(tag) != "" {
		parts = append(parts, tag)
	}
	if strings.TrimSpace(dd.Repository) != "" {
		parts = append(parts, dd.Repository)
	}
	return strings.Join(parts, " • ")
}

func (d depItem) FilterValue() string { return d.Title() + " " + d.Description() }

type versionItem string

func (v versionItem) Title() string       { return string(v) }
func (v versionItem) Description() string { return "" }
func (v versionItem) FilterValue() string { return string(v) }

type valuesFileItem string

func (v valuesFileItem) Title() string { return string(v) }
func (v valuesFileItem) Description() string {
	name := string(v)
	switch name {
	case "values.default.yaml":
		return "Baseline defaults"
	case "values.platform.yaml":
		return "Platform overrides"
	case "values.instance.yaml":
		return "User overrides (editable)"
	case "values.yaml":
		return "Merged output (generated)"
	default:
		// values.set.<name>.yaml
		const prefix = "values.set."
		const suffix = ".yaml"
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, suffix) {
			setName := strings.TrimSuffix(strings.TrimPrefix(name, prefix), suffix)
			if strings.TrimSpace(setName) != "" {
				return "Preset layer: " + setName
			}
			return "Preset layer"
		}
		return ""
	}
}
func (v valuesFileItem) FilterValue() string { return string(v) }

func (m *AppModel) refreshValuesList() {
	if m.selected == nil {
		m.valuesList.SetItems(nil)
		return
	}
	inst := *m.selected

	// Only show existing files. Keep ordering stable.
	//
	// Order:
	//  1) values.default.yaml (optional)
	//  2) values.platform.yaml (optional)
	//  3) values.set.*.yaml (0..n, lexicographic)
	//  4) values.instance.yaml (required by generator; user-owned)
	//  5) values.yaml (generated)
	base := []string{"values.default.yaml", "values.platform.yaml"}
	items := []list.Item{}
	for _, rel := range base {
		p := filepath.Join(inst.Path, rel)
		if _, err := os.Stat(p); err == nil {
			items = append(items, valuesFileItem(rel))
		}
	}
	setFiles, _ := filepath.Glob(filepath.Join(inst.Path, "values.set.*.yaml"))
	sort.Strings(setFiles)
	for _, p := range setFiles {
		items = append(items, valuesFileItem(filepath.Base(p)))
	}
	for _, rel := range []string{"values.instance.yaml", "values.yaml"} {
		p := filepath.Join(inst.Path, rel)
		if _, err := os.Stat(p); err == nil {
			items = append(items, valuesFileItem(rel))
		}
	}

	// Preserve selection when possible.
	prevSel := ""
	if it := m.valuesList.SelectedItem(); it != nil {
		if vf, ok := it.(valuesFileItem); ok {
			prevSel = string(vf)
		}
	}

	m.valuesList.SetItems(items)
	if prevSel != "" {
		for i, it := range items {
			if vf, ok := it.(valuesFileItem); ok && string(vf) == prevSel {
				m.valuesList.Select(i)
				break
			}
		}
	}
}

func renderAddDepView(m AppModel) string {
	header := lipgloss.NewStyle().Bold(true).Render(withIcon(iconAdd, "Add dependency"))

	if m.modalErr != "" {
		errLine := styleErrStrong.Render(withIcon(iconErr, "Error:") + " " + m.modalErr)
		header = header + "\n" + errLine
	}

	switch m.depStep {
	case depStepChooseSource:
		return header + "\n\n" + m.depSource.View()
	case depStepCatalog:
		if len(m.catalogEntries) == 0 {
			lines := []string{}
			if m.params.Config == nil {
				lines = append(lines, styleMuted.Render("No config loaded (helmdex.yaml)."))
				lines = append(lines, styleMuted.Render("Catalog requires at least one configured source."))
				lines = append(lines, "")
				lines = append(lines, styleMuted.Render("c: configure sources • esc: back"))
				return header + "\n\n" + strings.Join(lines, "\n")
			}
			if !hasCatalogEnabledSources(m.params.Config) {
				lines = append(lines, styleMuted.Render("No catalog-enabled sources configured."))
				lines = append(lines, "")
				lines = append(lines, styleMuted.Render("c: configure sources • esc: back"))
				return header + "\n\n" + strings.Join(lines, "\n")
			}
			if m.catalogWizardSyncing {
				lines = append(lines, styleMuted.Render("Syncing catalog into .helmdex/catalog…"))
				lines = append(lines, "")
				lines = append(lines, styleMuted.Render("esc: back"))
				return header + "\n\n" + strings.Join(lines, "\n")
			}
			lines = append(lines, styleMuted.Render("No local catalog entries."))
			lines = append(lines, styleMuted.Render("Catalog entries are read from .helmdex/catalog/*.yaml."))
			lines = append(lines, "")
			lines = append(lines, styleMuted.Render("s: sync catalog • c: configure sources • esc: back"))
			return header + "\n\n" + strings.Join(lines, "\n")
		}
		return header + "\n\n" + m.catalogList.View()
	case depStepCatalogDetail:
		if m.catalogDetailEntry == nil {
			return header + "\n\n" + styleMuted.Render("No catalog entry selected")
		}
		e := m.catalogDetailEntry
		lines := []string{}
		lines = append(lines, lipgloss.NewStyle().Bold(true).Render(withIcon(iconCatalog, e.Entry.ID)))
		lines = append(lines, styleMuted.Render(e.Entry.Chart.Repo+"/"+e.Entry.Chart.Name+"@"+e.Entry.Version))
		if strings.TrimSpace(e.Entry.Description) != "" {
			lines = append(lines, "")
			lines = append(lines, e.Entry.Description)
		}
		lines = append(lines, "")
		if m.catalogSetsLoading {
			lines = append(lines, styleMuted.Render("Loading sets from preset cache…"))
		} else {
			if len(m.catalogSetList.Items()) == 0 {
				lines = append(lines, styleMuted.Render("No sets found for this chart/version in the preset cache."))
			} else {
				lines = append(lines, lipgloss.NewStyle().Bold(true).Render("Sets:"))
				lines = append(lines, m.catalogSetList.View())
				lines = append(lines, "")
				lines = append(lines, styleMuted.Render("space: toggle • D: toggle defaults • enter: add+apply"))
			}
		}
		return header + "\n\n" + strings.Join(lines, "\n")
	case depStepCatalogCollision:
		lines := []string{}
		lines = append(lines, lipgloss.NewStyle().Bold(true).Render(withIcon(iconErr, "Dependency already exists")))
		lines = append(lines, styleMuted.Render(fmt.Sprintf("A dependency named %q already exists in this instance.", m.catalogCollisionDep.Name)))
		lines = append(lines, "")
		lines = append(lines, lipgloss.NewStyle().Bold(true).Render("Choose:"))
		aliasLine := "( ) Alias (recommended)"
		overrideLine := "( ) Override existing"
		cancelLine := "( ) Cancel"
		switch m.catalogCollisionChoice {
		case collisionChoiceAlias:
			aliasLine = "(*) Alias (recommended)"
		case collisionChoiceOverride:
			overrideLine = "(*) Override existing"
		case collisionChoiceCancel:
			cancelLine = "(*) Cancel"
		}
		lines = append(lines, aliasLine)
		lines = append(lines, overrideLine+" "+styleMuted.Render("(will delete dep-set markers for this depID)"))
		lines = append(lines, cancelLine)
		if m.catalogCollisionChoice == collisionChoiceAlias {
			lines = append(lines, "")
			lines = append(lines, m.catalogCollisionAlias.View())
			lines = append(lines, styleMuted.Render("alias is required"))
		}
		lines = append(lines, "")
		lines = append(lines, styleMuted.Render("↑/↓ select • enter confirm • esc back"))
		return header + "\n\n" + strings.Join(lines, "\n")
	case depStepAHQuery:
		return header + "\n\n" + withIcon(iconAH, "Artifact Hub search") + "\n\n" + m.ahQuery.View() + "\n\n(enter to search)"
	case depStepAHResults:
		return header + "\n\n" + m.ahResults.View() + "\n\n" + styleMuted.Render("enter: open details")
	case depStepAHVersions:
		return header + "\n\n" + m.ahVersions.View()
	case depStepAHDetail:
		body := renderTabs(m.ahDetailTabNames, m.ahDetailTab) + "\n"
		switch m.ahDetailTab {
		case 2:
			body += m.ahVersions.View() + "\n\n" + styleMuted.Render("enter: load README/values • a: add")
		default:
			body += m.ahPreview.View() + "\n\n" + styleMuted.Render("a: add")
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
	if m.instanceManageOpen && m.instanceManageMode == instanceManageRename && m.instanceManageName.Focused() {
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
	}
	if m.depDetailOpen {
		if m.depDetailMode == depEditModeManual && m.depDetailVersionInput.Focused() {
			return true
		}
		if m.depDetailMode == depEditModeList && m.depDetailVersions.FilterState() == list.Filtering {
			return true
		}
		if m.depDetailAliasInput.Focused() {
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

func (m *AppModel) clearAnyActiveFilter() bool {
	cleared := false
	resetIfActive := func(l *list.Model) {
		fs := l.FilterState()
		if fs == list.Filtering || fs == list.FilterApplied {
			l.ResetFilter()
			cleared = true
		}
	}
	resetIfActive(&m.instList)
	resetIfActive(&m.catalogList)
	resetIfActive(&m.depSource)
	resetIfActive(&m.depsList)
	resetIfActive(&m.valuesList)
	return cleared
}

func (m AppModel) depEditUpdate(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEsc {
		// When filtering is active, Esc should clear the filter first (rather than
		// closing the whole modal).
		if m.depEditMode == depEditModeList {
			fs := m.depEditVersions.FilterState()
			if fs == list.Filtering || fs == list.FilterApplied {
				m.depEditVersions.ResetFilter()
				return m, nil
			}
		}
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
			m.depEditVersionInput.Blur()
			m.modalErr = ""
			// Keep modal open; close only on successful apply.
			// Validate chosen version before writing Chart.yaml.
			return m, tea.Batch(m.beginBusy("Validating"), m.validateDependencyVersionCmd(dep))
		}
		return m, cmd
	}

	var cmd tea.Cmd
	m.depEditVersions, cmd = m.depEditVersions.Update(msg)
	if msg.Type == tea.KeyEnter {
		// When the versions list is in filtering mode, Enter applies the filter.
		// Do not treat it as selecting/applying a version.
		if m.depEditVersions.FilterState() == list.Filtering {
			return m, cmd
		}
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
		m.modalErr = ""
		// Validate the chosen version before writing Chart.yaml. This prevents UI
		// state from being corrupted by versions that appear in `helm search repo`
		// output but cannot be resolved by Helm.
		return m, tea.Batch(cmd, m.beginBusy("Validating"), m.validateDependencyVersionCmd(dep))
	}
	return m, cmd
}

func (m AppModel) openDepDetailSelected() (tea.Model, tea.Cmd) {
	it := m.depsList.SelectedItem()
	if it == nil {
		return m, nil
	}
	di, ok := it.(depItem)
	if !ok {
		return m, func() tea.Msg { return errMsg{fmt.Errorf("unexpected dependency item type")} }
	}
	dep := di.Dep
	// Load dep source metadata for display + dynamic tabs.
	if m.selected != nil {
		if meta, ok := readDepSourceMeta(m.params.RepoRoot, m.selected.Name, yamlchart.DependencyID(dep)); ok {
			m.depDetailSource = meta
			m.depDetailSourceOK = true
		} else {
			m.depDetailSource = depSourceMeta{}
			m.depDetailSourceOK = false
		}
	}
	m.depDetailTabNames, m.depDetailTabKinds = depDetailTabs(m.depDetailSource, m.depDetailSourceOK)

	m.depDetailOpen = true
	m.depDetailDep = dep
	m.depDetailTab = 0
	m.depDetailLoading = true
	m.depDetailSetsLoading = false
	m.modalErr = ""
	m.depDetailReadme = ""
	m.depDetailDefaultValues = ""
	m.depDetailPendingVersion = ""
	m.depDetailVersionsData = nil
	m.depDetailVersions.SetItems(nil)
	m.depDetailVersionsLoading = false
	m.depDetailAliasInput.SetValue(strings.TrimSpace(dep.Alias))
	m.depDetailAliasInput.Blur()
	m.depDetailDeleteConfirm = false
	m.depDetailSets.SetItems(nil)
	m.depDetailPreview.SetContent(m.renderDepDetailBody())

	// Decide mode.
	if strings.HasPrefix(dep.Repository, "oci://") {
		m.depDetailMode = depEditModeManual
		m.depDetailVersionInput.SetValue(dep.Version)
		// Only focus when user opens the Versions tab.
		m.depDetailVersionInput.Blur()
	} else {
		m.depDetailMode = depEditModeList
		m.depDetailVersionInput.Blur()
	}

	// Kick off loads.
	m.depDetailSetsLoading = true
	cmds := []tea.Cmd{m.beginBusy("Loading"), m.loadDepDetailPreviewsCmd(dep), m.loadDepDetailSetsCmd(dep)}
	if m.depDetailMode == depEditModeList {
		// Cache-first: populate from cache immediately if present.
		if vs, _, ok, err := helmutil.ReadVersionsCache(m.params.RepoRoot, dep.Repository, dep.Name); err == nil && ok {
			m.setVersionsList(vs, versionsTargetDepDetail)
		}
		// Always background refresh; show loader in Versions tab only.
		key := versionsKey(dep.Repository, dep.Name)
		if !m.versionsInFlight[key] {
			m.versionsInFlight[key] = true
			m.depDetailVersionsLoading = true
			m.watchVersions(dep)
			cmds = append(cmds, m.refreshVersionsCmd(dep, versionsTargetDepDetail))
		}
	}
	return m, tea.Batch(cmds...)
}

func (m AppModel) depDetailUpdate(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	activeKind := depDetailTabValues
	if m.depDetailTab >= 0 && m.depDetailTab < len(m.depDetailTabKinds) {
		activeKind = m.depDetailTabKinds[m.depDetailTab]
	}
	versionsTab := len(m.depDetailTabNames) - 1

	// Close.
	if msg.Type == tea.KeyEsc {
		// Dependency tab: Esc cancels alias edit mode (do not close modal).
		if activeKind == depDetailTabDependency && m.depDetailAliasInput.Focused() {
			m.depDetailAliasInput.SetValue(strings.TrimSpace(m.depDetailDep.Alias))
			m.depDetailAliasInput.Blur()
			m.depDetailPreview.SetContent(m.renderDepDetailBody())
			return m, nil
		}
		if m.depDetailDeleteConfirm {
			m.depDetailDeleteConfirm = false
			m.depDetailPreview.SetContent(m.renderDepDetailBody())
			return m, nil
		}
		if m.depDetailMode == depEditModeList {
			fs := m.depDetailVersions.FilterState()
			if fs == list.Filtering || fs == list.FilterApplied {
				m.depDetailVersions.ResetFilter()
				return m, nil
			}
		}
		if m.depConfigure.editing {
			m.depConfigure.CancelEdit()
			m.depDetailPreview.SetContent(m.renderDepDetailBody())
			return m, nil
		}
		m.depDetailOpen = false
		m.depDetailLoading = false
		m.modalErr = ""
		m.depDetailDep = yamlchart.Dependency{}
		m.depDetailVersionInput.Blur()
		m.depDetailAliasInput.Blur()
		return m, nil
	}

	// Dependency tab.
	if activeKind == depDetailTabDependency {
		// Allow tab navigation with ←/→ while not editing the alias.
		// When editing (input focused), let the text input consume arrow keys.
		if !m.depDetailAliasInput.Focused() {
			if key.Matches(msg, m.keys.TabLeft) {
				m.depDetailTab = (m.depDetailTab - 1 + len(m.depDetailTabNames)) % len(m.depDetailTabNames)
				if m.depDetailTab == versionsTab && m.depDetailMode == depEditModeManual {
					m.depDetailVersionInput.Focus()
				} else {
					m.depDetailVersionInput.Blur()
				}
				m.depDetailPreview.SetContent(m.renderDepDetailBody())
				return m, nil
			}
			if key.Matches(msg, m.keys.TabRight) {
				m.depDetailTab = (m.depDetailTab + 1) % len(m.depDetailTabNames)
				if m.depDetailTab == versionsTab && m.depDetailMode == depEditModeManual {
					m.depDetailVersionInput.Focus()
				} else {
					m.depDetailVersionInput.Blur()
				}
				m.depDetailPreview.SetContent(m.renderDepDetailBody())
				return m, nil
			}
		}

		// Do not auto-focus the alias input. User must explicitly enter edit mode
		// (Enter focuses; Esc blurs/reverts; Enter while focused applies).
		// Confirmation flow.
		if m.depDetailDeleteConfirm {
			if msg.String() == "y" || msg.String() == "Y" {
				m.depDetailDeleteConfirm = false
				return m, tea.Batch(m.beginBusy("Deleting"), m.deleteDepFromDetailCmd())
			}
			if msg.String() == "n" || msg.String() == "N" {
				m.depDetailDeleteConfirm = false
				m.depDetailPreview.SetContent(m.renderDepDetailBody())
				return m, nil
			}
			return m, nil
		}

		// Alias edit mode.
		if msg.Type == tea.KeyEsc {
			if m.depDetailAliasInput.Focused() {
				m.depDetailAliasInput.SetValue(strings.TrimSpace(m.depDetailDep.Alias))
				m.depDetailAliasInput.Blur()
				m.depDetailPreview.SetContent(m.renderDepDetailBody())
				return m, nil
			}
			// fall through: global Esc handling above closes modal
		}
		if msg.Type == tea.KeyEnter {
			if !m.depDetailAliasInput.Focused() {
				m.depDetailAliasInput.Focus()
				m.depDetailPreview.SetContent(m.renderDepDetailBody())
				return m, nil
			}
			// focused: apply
			alias := strings.TrimSpace(m.depDetailAliasInput.Value())
			m.depDetailAliasInput.Blur()
			m.depDetailPreview.SetContent(m.renderDepDetailBody())
			return m, tea.Batch(m.beginBusy("Applying"), m.applyDepAliasFromDetailCmd(alias))
		}

		// While editing, update the input; otherwise ignore keystrokes.
		var cmd tea.Cmd
		if m.depDetailAliasInput.Focused() {
			m.depDetailAliasInput, cmd = m.depDetailAliasInput.Update(msg)
		}
		if msg.String() == "d" || msg.String() == "D" {
			m.depDetailDeleteConfirm = true
			m.depDetailPreview.SetContent(m.renderDepDetailBody())
			return m, nil
		}
		m.depDetailPreview.SetContent(m.renderDepDetailBody())
		return m, cmd
	}

	// Tab-specific interaction.
	// Configure tab: no focus mode.
	// - ↑/↓ navigate tree
	// - Enter edits scalars, cycles unions, and toggles expand/collapse for object/array
	// - ←/→ collapse/expand when possible
	// - ←/→ also navigates tabs ONLY when cursor is on the root "$" row and the tree cannot
	//   collapse/expand further in that direction.
	if activeKind == depDetailTabValues {
		// While editing, route keys to the active input.
		if m.depConfigure.editing {
			var cmd tea.Cmd
			if m.depConfigure.editMode == cfgEditNewPropKey {
				m.depConfigure.editPropKey, cmd = m.depConfigure.editPropKey.Update(msg)
			} else {
				m.depConfigure.editInput, cmd = m.depConfigure.editInput.Update(msg)
				// Clear displayed edit error as soon as user changes input.
				if msg.Type != tea.KeyEnter {
					m.depConfigure.editErr = ""
				}
			}
			// Always rerender while editing so typed text is visible.
			m.depDetailPreview.SetContent(m.renderDepDetailBody())
			if msg.Type == tea.KeyEnter {
				if err := m.depConfigure.ApplyEdit(); err != nil {
					m.modalErr = err.Error()
				} else {
					m.modalErr = ""
				}
				m.depDetailPreview.SetContent(m.renderDepDetailBody())
				return m, cmd
			}
			if msg.Type == tea.KeyEsc {
				m.depConfigure.CancelEdit()
				m.depDetailPreview.SetContent(m.renderDepDetailBody())
				return m, cmd
			}
			return m, cmd
		}
		// Tree navigation.
		switch {
		case msg.Type == tea.KeyUp:
			m.depConfigure.Move(-1)
			m.depDetailPreview.SetContent(m.renderDepDetailBody())
			return m, nil
		case msg.Type == tea.KeyDown:
			m.depConfigure.Move(1)
			m.depDetailPreview.SetContent(m.renderDepDetailBody())
			return m, nil
		case msg.Type == tea.KeyEnter:
			// Enter toggles expand for containers (object/array).
			if m.depConfigure.ToggleExpandCollapse() {
				m.depDetailPreview.SetContent(m.renderDepDetailBody())
				return m, nil
			}
			changed := m.depConfigure.StartEdit()
			if changed {
				_ = m.depConfigure.PersistDraft()
				m.refreshValuesList()
			}
			m.depDetailPreview.SetContent(m.renderDepDetailBody())
			return m, nil
		case msg.String() == "s" || msg.String() == "S":
			if err := m.depConfigure.Save(); err != nil {
				m.modalErr = err.Error()
			} else {
				m.modalErr = ""
			}
			m.refreshValuesList()
			m.depDetailPreview.SetContent(m.renderDepDetailBody())
			return m, nil
		case msg.Type == tea.KeyLeft:
			changed := m.depConfigure.Collapse()
			if changed {
				m.depDetailPreview.SetContent(m.renderDepDetailBody())
				return m, nil
			}
			// Only bubble to tab navigation when the cursor is on root.
			if !m.depConfigure.CursorIsRoot() {
				return m, nil
			}
			// fall through to normal tab switching below
		case msg.Type == tea.KeyRight:
			changed := m.depConfigure.Expand()
			if changed {
				m.depDetailPreview.SetContent(m.renderDepDetailBody())
				return m, nil
			}
			// Only bubble to tab navigation when the cursor is on root.
			if !m.depConfigure.CursorIsRoot() {
				return m, nil
			}
			// fall through to normal tab switching below
		}
	}

	// Sets tab.
	if activeKind == depDetailTabSets {
		// Allow tab navigation while the list is focused.
		if key.Matches(msg, m.keys.TabLeft) {
			m.depDetailTab = (m.depDetailTab - 1 + len(m.depDetailTabNames)) % len(m.depDetailTabNames)
			m.depDetailPreview.SetContent(m.renderDepDetailBody())
			return m, nil
		}
		if key.Matches(msg, m.keys.TabRight) {
			m.depDetailTab = (m.depDetailTab + 1) % len(m.depDetailTabNames)
			m.depDetailPreview.SetContent(m.renderDepDetailBody())
			return m, nil
		}

		var cmd tea.Cmd
		m.depDetailSets, cmd = m.depDetailSets.Update(msg)
		if msg.Type == tea.KeySpace {
			if it := m.depDetailSets.SelectedItem(); it != nil {
				if si, ok := it.(setChoiceForDepItem); ok {
					si.C.On = !si.C.On
					items := m.depDetailSets.Items()
					idx := m.depDetailSets.Index()
					if idx >= 0 && idx < len(items) {
						items[idx] = si
						m.depDetailSets.SetItems(items)
						m.depDetailSets.Select(idx)
					}
				}
			}
			m.depDetailPreview.SetContent(m.renderDepDetailBody())
			return m, cmd
		}
		if msg.Type == tea.KeyEnter {
			// Apply selection.
			return m, tea.Batch(cmd, m.beginBusy("Applying"), m.applyDepDetailSetsCmd())
		}
		if msg.String() == "s" || msg.String() == "S" {
			// Save+apply immediately.
			return m, tea.Batch(m.beginBusy("Applying"), m.applyDepDetailSetsCmd())
		}
		// Allow scrolling through the list.
		m.depDetailPreview.SetContent(m.renderDepDetailBody())
		return m, cmd
	}

	// Switch tabs.
	if key.Matches(msg, m.keys.TabLeft) {
		m.depDetailTab = (m.depDetailTab - 1 + len(m.depDetailTabNames)) % len(m.depDetailTabNames)
		if m.depDetailTab == versionsTab && m.depDetailMode == depEditModeManual {
			m.depDetailVersionInput.Focus()
		} else {
			m.depDetailVersionInput.Blur()
		}
		m.depDetailPreview.SetContent(m.renderDepDetailBody())
		return m, nil
	}
	if key.Matches(msg, m.keys.TabRight) {
		m.depDetailTab = (m.depDetailTab + 1) % len(m.depDetailTabNames)
		if m.depDetailTab == versionsTab && m.depDetailMode == depEditModeManual {
			m.depDetailVersionInput.Focus()
		} else {
			m.depDetailVersionInput.Blur()
		}
		m.depDetailPreview.SetContent(m.renderDepDetailBody())
		return m, nil
	}

	// Versions tab index is last.
	if m.depDetailTab == versionsTab {
		if m.depDetailMode == depEditModeManual {
			var cmd tea.Cmd
			m.depDetailVersionInput, cmd = m.depDetailVersionInput.Update(msg)
			if msg.Type == tea.KeyEnter {
				v := strings.TrimSpace(m.depDetailVersionInput.Value())
				if v == "" {
					m.modalErr = "version is required"
					return m, nil
				}
				dep := m.depDetailDep
				dep.Version = v
				m.modalErr = ""
				m.depDetailPendingVersion = v
				m.depDetailVersionInput.Blur()
				return m, tea.Batch(m.beginBusy("Validating"), m.validateDependencyVersionCmd(dep))
			}
			return m, cmd
		}

		var cmd tea.Cmd
		m.depDetailVersions, cmd = m.depDetailVersions.Update(msg)
		if msg.Type == tea.KeyEnter {
			if m.depDetailVersions.FilterState() == list.Filtering {
				return m, cmd
			}
			it := m.depDetailVersions.SelectedItem()
			if it == nil {
				return m, cmd
			}
			vi, ok := it.(versionItem)
			if !ok {
				return m, func() tea.Msg { return errMsg{fmt.Errorf("unexpected version item type")} }
			}
			v := strings.TrimSpace(string(vi))
			dep := m.depDetailDep
			dep.Version = v
			m.modalErr = ""
			m.depDetailPendingVersion = v
			return m, tea.Batch(cmd, m.beginBusy("Validating"), m.validateDependencyVersionCmd(dep))
		}
		return m, cmd
	}

	// Non-Versions tabs: allow scrolling in the preview viewport.
	var cmd tea.Cmd
	m.depDetailPreview, cmd = m.depDetailPreview.Update(msg)
	return m, cmd
}

func (m AppModel) renderDepDetailBody() string {
	dep := m.depDetailDep
	if dep.Name == "" {
		return "No dependency selected"
	}
	if m.depDetailLoading {
		return "Loading…"
	}
	activeKind := depDetailTabValues
	if m.depDetailTab >= 0 && m.depDetailTab < len(m.depDetailTabKinds) {
		activeKind = m.depDetailTabKinds[m.depDetailTab]
	}
	switch activeKind {
	case depDetailTabValues:
		return m.depConfigure.View(m.depDetailPreview.Width, m.depDetailPreview.Height)
	case depDetailTabDependency:
		lines := []string{}
		lines = append(lines, lipgloss.NewStyle().Bold(true).Render(withIcon(iconDeps, "Settings")))
		lines = append(lines, "")
		lines = append(lines, styleMuted.Render("Alias (changes depID)"))
		lines = append(lines, m.depDetailAliasInput.View())
		if m.depDetailAliasInput.Focused() {
			lines = append(lines, styleMuted.Render("enter: apply • esc: cancel"))
		} else {
			lines = append(lines, styleMuted.Render("enter: edit • (leave empty + enter to clear)"))
		}
		lines = append(lines, "")
		if m.depDetailDeleteConfirm {
			lines = append(lines, styleErrStrong.Render(withIcon(iconTrash, "Delete dependency?")))
			lines = append(lines, styleMuted.Render("This will remove from Chart.yaml and delete depID-keyed data (values.instance.yaml key, values.dep-set markers, depmeta)."))
			lines = append(lines, styleMuted.Render("y: delete • n: cancel"))
		} else {
			lines = append(lines, styleMuted.Render("d: delete dependency"))
		}
		return strings.Join(lines, "\n")
	case depDetailTabReadme:
		if m.depDetailReadme == "" {
			return "README not loaded."
		}
		return renderMarkdownForDisplay(m.depDetailPreview.Width, m.depDetailReadme)
	case depDetailTabDefault:
		if m.depDetailDefaultValues == "" {
			return "Default values not loaded."
		}
		return m.depDetailDefaultValues
	case depDetailTabSets:
		if m.depDetailSetsLoading {
			return styleMuted.Render("Loading sets from preset cache…")
		}
		if len(m.depDetailSets.Items()) == 0 {
			return styleMuted.Render("No preset sets found for this dependency/version in the preset cache.")
		}
		lines := []string{}
		lines = append(lines, lipgloss.NewStyle().Bold(true).Render("Sets:"))
		lines = append(lines, m.depDetailSets.View())
		lines = append(lines, "")
		lines = append(lines, styleMuted.Render("space: toggle • s: save+apply • esc: close"))
		return strings.Join(lines, "\n")
	case depDetailTabVersions:
		// Versions are rendered by the modal renderer.
		return ""
	default:
		return ""
	}
}

// renderDepLocalValuesBody removed: the dependency detail "Local" tab is intentionally hidden.

func (m AppModel) validateDependencyVersionCmd(dep yamlchart.Dependency) tea.Cmd {
	return func() tea.Msg {
		if strings.HasPrefix(dep.Repository, "oci://") {
			return depVersionValidatedMsg{dep: dep}
		}
		ctx, cancel := context.WithTimeout(contextBG(), 20*time.Second)
		defer cancel()
		env := helmutil.EnvForRepoURL(m.params.RepoRoot, dep.Repository)
		repoName := helmutil.RepoNameForURL(dep.Repository)
		ref := repoName + "/" + dep.Name
		// best-effort add; RepoAdd is cheap and ensures repo is present.
		if err := helmutil.RepoAdd(ctx, env, repoName, dep.Repository); err != nil {
			return errMsg{err}
		}
		// `helm show chart` fails fast if the version doesn't exist.
		if _, err := helmutil.ShowChart(ctx, env, ref, dep.Version); err != nil {
			return errMsg{fmt.Errorf("invalid version %q for %s: %w", dep.Version, yamlchart.DependencyID(dep), err)}
		}
		return depVersionValidatedMsg{dep: dep}
	}
}

type depVersionValidatedMsg struct {
	dep yamlchart.Dependency
}

type depAliasAppliedMsg struct{
	chart yamlchart.Chart
	dep   yamlchart.Dependency
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
	dep := di.Dep

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

	// Cache-first: show cached versions immediately if present.
	if vs, fetchedAt, ok, err := helmutil.ReadVersionsCache(m.params.RepoRoot, dep.Repository, dep.Name); err == nil && ok {
		m.setVersionsList(vs, versionsTargetDepEdit)
		// Start background refresh regardless; loader shown only in the Versions view.
		_ = fetchedAt
	}

	// Always start a background refresh (non-blocking UI). Use single-flight.
	key := versionsKey(dep.Repository, dep.Name)
	if !m.versionsInFlight[key] {
		m.versionsInFlight[key] = true
		m.depEditLoading = true
		m.watchVersions(dep)
		return m, m.refreshVersionsCmd(dep, versionsTargetDepEdit)
	}
	return m, nil
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
	dep := di.Dep
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
		// Keep a copy so we can roll back if Helm relock fails.
		origChartYAML, _ := os.ReadFile(chartPath)
		lockPath := filepath.Join(m.selected.Path, "Chart.lock")
		origLockYAML, _ := os.ReadFile(lockPath)
		hadOrigLock := origLockYAML != nil
		rollback := func() {
			if origChartYAML != nil {
				_ = os.WriteFile(chartPath, origChartYAML, 0o644)
			}
			if hadOrigLock {
				_ = os.WriteFile(lockPath, origLockYAML, 0o644)
			} else {
				_ = os.Remove(lockPath)
			}
		}

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
			rollback()
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

func appsDirFromConfig(cfg *config.Config) string {
	appsDir := "apps"
	if cfg != nil && cfg.Repo.AppsDir != "" {
		appsDir = cfg.Repo.AppsDir
	}
	return appsDir
}

func isValidDepSetMarker(base string) bool {
	// Minimal sanity check for marker filenames we manage.
	// Format: values.dep-set.<depID>--<set>.yaml
	if !strings.HasPrefix(base, "values.dep-set.") {
		return false
	}
	if !strings.HasSuffix(base, ".yaml") {
		return false
	}
	name := strings.TrimSuffix(strings.TrimPrefix(base, "values.dep-set."), ".yaml")
	parts := strings.SplitN(name, "--", 2)
	return len(parts) == 2 && strings.TrimSpace(parts[0]) != "" && strings.TrimSpace(parts[1]) != ""
}

func migrateDepOverrideKey(instancePath string, oldID, newID string) error {
	oldID = strings.TrimSpace(oldID)
	newID = strings.TrimSpace(newID)
	if instancePath == "" || oldID == "" || newID == "" || oldID == newID {
		return nil
	}
	path := filepath.Join(instancePath, "values.instance.yaml")
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(b) == 0 {
		return nil
	}
	var root any
	if err := yaml.Unmarshal(b, &root); err != nil {
		return err
	}
	obj, ok := root.(map[string]any)
	if !ok {
		return nil
	}
	val, ok := obj[oldID]
	if !ok {
		return nil
	}
	if _, exists := obj[newID]; exists {
		return fmt.Errorf("values.instance.yaml already contains key %q", newID)
	}
	delete(obj, oldID)
	obj[newID] = val
	out, err := yaml.Marshal(obj)
	if err != nil {
		return err
	}
	if len(out) == 0 || out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	return os.WriteFile(path, out, 0o644)
}

func deleteDepOverrideKey(instancePath, depID string) error {
	depID = strings.TrimSpace(depID)
	if instancePath == "" || depID == "" {
		return nil
	}
	path := filepath.Join(instancePath, "values.instance.yaml")
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(b) == 0 {
		return nil
	}
	var root any
	if err := yaml.Unmarshal(b, &root); err != nil {
		return err
	}
	obj, ok := root.(map[string]any)
	if !ok {
		return nil
	}
	if _, ok := obj[depID]; !ok {
		return nil
	}
	delete(obj, depID)
	out, err := yaml.Marshal(obj)
	if err != nil {
		return err
	}
	if len(out) == 0 || out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	return os.WriteFile(path, out, 0o644)
}

func migrateDepSetMarkers(instancePath string, oldID, newID yamlchart.DepID) error {
	if instancePath == "" || strings.TrimSpace(string(oldID)) == "" || strings.TrimSpace(string(newID)) == "" || oldID == newID {
		return nil
	}
	glob := filepath.Join(instancePath, fmt.Sprintf("values.dep-set.%s--*.yaml", oldID))
	files, err := filepath.Glob(glob)
	if err != nil {
		return err
	}
	for _, p := range files {
		base := filepath.Base(p)
		if !isValidDepSetMarker(base) {
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(base, "values.dep-set."), ".yaml")
		parts := strings.SplitN(name, "--", 2)
		if len(parts) != 2 {
			continue
		}
		setName := strings.TrimSpace(parts[1])
		if setName == "" {
			continue
		}
		np := filepath.Join(instancePath, fmt.Sprintf("values.dep-set.%s--%s.yaml", newID, setName))
		if _, err := os.Stat(np); err == nil {
			return fmt.Errorf("dep-set marker already exists for new depID %q set %q", newID, setName)
		}
		if err := os.Rename(p, np); err != nil {
			return err
		}
	}
	return nil
}

func deleteDepSetMarkersForID(instancePath string, depID yamlchart.DepID) error {
	return deleteDepSetMarkers(instancePath, depID)
}

func (m AppModel) applyDepAliasFromDetailCmd(alias string) tea.Cmd {
	return func() tea.Msg {
		if m.selected == nil {
			return errMsg{fmt.Errorf("no instance selected")}
		}
		instName := strings.TrimSpace(m.selected.Name)
		if instName == "" {
			return errMsg{fmt.Errorf("no instance selected")}
		}

		chartPath := filepath.Join(m.selected.Path, "Chart.yaml")
		c, err := yamlchart.ReadChart(chartPath)
		if err != nil {
			return errMsg{err}
		}

		oldDep := m.depDetailDep
		oldID := yamlchart.DependencyID(oldDep)
		// Build new dep with updated alias.
		newDep := oldDep
		newDep.Alias = strings.TrimSpace(alias)
		newID := yamlchart.DependencyID(newDep)

		// Update Chart.yaml.
		if err := c.ReplaceDependencyByID(oldID, newDep); err != nil {
			return errMsg{err}
		}
		if err := yamlchart.WriteChart(chartPath, c); err != nil {
			return errMsg{err}
		}

		// Migrate depID-keyed data when id changes.
		if newID != oldID {
			if err := migrateDepOverrideKey(m.selected.Path, string(oldID), string(newID)); err != nil {
				return errMsg{err}
			}
			if err := migrateDepSetMarkers(m.selected.Path, oldID, newID); err != nil {
				return errMsg{err}
			}
			if err := renameDepMetaFile(m.params.RepoRoot, instName, oldID, newID); err != nil {
				return errMsg{err}
			}
		}

		// Apply pipeline (presets import + values regen; lock isn't needed for alias only).
		if m.params.Config != nil {
			if _, err := presets.Import(presets.ImportParams{RepoRoot: m.params.RepoRoot, InstancePath: m.selected.Path, Config: *m.params.Config, Dependencies: c.Dependencies}); err != nil {
				return errMsg{err}
			}
		}
		if err := values.GenerateMergedValues(m.selected.Path); err != nil {
			return errMsg{err}
		}

		// Keep modal in sync.
		return depAliasAppliedMsg{chart: c, dep: newDep}
	}
}

func (m AppModel) deleteDepFromDetailCmd() tea.Cmd {
	return func() tea.Msg {
		if m.selected == nil {
			return errMsg{fmt.Errorf("no instance selected")}
		}
		instName := strings.TrimSpace(m.selected.Name)
		if instName == "" {
			return errMsg{fmt.Errorf("no instance selected")}
		}
		dep := m.depDetailDep
		depID := yamlchart.DependencyID(dep)

		chartPath := filepath.Join(m.selected.Path, "Chart.yaml")
		c, err := yamlchart.ReadChart(chartPath)
		if err != nil {
			return errMsg{err}
		}
		if ok := c.RemoveDependencyByID(depID); !ok {
			return errMsg{fmt.Errorf("dependency %q not found", depID)}
		}
		if err := yamlchart.WriteChart(chartPath, c); err != nil {
			return errMsg{err}
		}

		// Cleanup depID-keyed data.
		if err := deleteDepOverrideKey(m.selected.Path, string(depID)); err != nil {
			return errMsg{err}
		}
		_ = deleteDepSetMarkersForID(m.selected.Path, depID)
		_ = deleteDepMetaFile(m.params.RepoRoot, instName, depID)

		// Apply pipeline.
		if m.params.Config != nil {
			if _, err := presets.Import(presets.ImportParams{RepoRoot: m.params.RepoRoot, InstancePath: m.selected.Path, Config: *m.params.Config, Dependencies: c.Dependencies}); err != nil {
				return errMsg{err}
			}
		}
		if err := values.GenerateMergedValues(m.selected.Path); err != nil {
			return errMsg{err}
		}
		return depAppliedMsg{chart: c}
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
		editor := strings.TrimSpace(os.Getenv("EDITOR"))
		if editor == "" {
			editor = "vi"
		}
		path := filepath.Join(m.selected.Path, "values.instance.yaml")
		name, args := editorCommand(editor, path)
		cmd := exec.Command(name, args...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return errMsg{fmt.Errorf("editor failed: %w", err)}
		}
		return editValuesDoneMsg{}
	}
}

// editorCommand builds an editor invocation that tries to *block until the file
// is closed* for common GUI editors.
//
// NOTE: This uses strings.Fields (no shell parsing). If you need quoting,
// prefer setting EDITOR to a wrapper script.
func editorCommand(editor, path string) (name string, args []string) {
	parts := strings.Fields(strings.TrimSpace(editor))
	if len(parts) == 0 {
		return "vi", []string{path}
	}
	name = parts[0]
	args = append([]string{}, parts[1:]...)

	base := filepath.Base(name)

	// Terminal editors block by default (no special flags required).
	// Keep this list as documentation / future hook point.
	switch base {
	case "vi", "vim", "nvim", "neovim", "nano", "micro":
		args = append(args, path)
		return name, args
	}

	// VS Code and variants: require --wait to block.
	if (base == "code" || base == "code-insiders" || base == "codium" || base == "cursor") && !containsArg(args, "--wait") {
		args = append(args, "--wait")
	}
	// Sublime: -w blocks.
	if (base == "subl" || base == "sublime_text") && !containsArg(args, "-w") && !containsArg(args, "--wait") {
		args = append(args, "-w")
	}
	// gedit: --wait blocks.
	if base == "gedit" && !containsArg(args, "--wait") {
		args = append(args, "--wait")
	}

	args = append(args, path)
	return name, args
}

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
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
	case InstanceTabValues:
		m.refreshValuesList()
	default:
		// Deps tab (and any future non-viewport tabs) render via their own widgets.
		m.content.SetContent("")
	}
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
	// Deprecated: Values tab now renders via valuesList.
	return ""
}

// renderPresetsTab removed: preset resolution is still used internally (catalog set discovery
// and preset import on apply), but the instance Presets tab is intentionally hidden.

func (m AppModel) removeSelectedDepCmd() tea.Cmd {
	return func() tea.Msg {
		if m.selected == nil {
			return errMsg{fmt.Errorf("no instance selected")}
		}
		instName := strings.TrimSpace(m.selected.Name)
		if instName == "" {
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
		id := yamlchart.DependencyID(di.Dep)
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

		// Cleanup depID-keyed data.
		if err := deleteDepOverrideKey(m.selected.Path, string(id)); err != nil {
			return errMsg{err}
		}
		_ = deleteDepSetMarkersForID(m.selected.Path, id)
		_ = deleteDepMetaFile(m.params.RepoRoot, instName, id)

		// Apply pipeline.
		if m.params.Config != nil {
			if _, err := presets.Import(presets.ImportParams{RepoRoot: m.params.RepoRoot, InstancePath: m.selected.Path, Config: *m.params.Config, Dependencies: c.Dependencies}); err != nil {
				return errMsg{err}
			}
		}
		if err := values.GenerateMergedValues(m.selected.Path); err != nil {
			return errMsg{err}
		}
		return depAppliedMsg{chart: c}
	}
}

// contextBG avoids importing context in many places; v0.2 uses Background for Artifact Hub calls.
func contextBG() context.Context { return context.Background() }
