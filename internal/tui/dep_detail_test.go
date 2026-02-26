package tui

import (
	"strings"
	"testing"
	"helmdex/internal/yamlchart"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func TestDepsEnterOpensDepDetailModal(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "."})
	m.screen = ScreenInstance
	m.activeTab = 0 // deps tab (Dependencies is first)

	dep := yamlchart.Dependency{Name: "postgresql", Repository: "https://charts.bitnami.com/bitnami", Version: "1.2.3"}
	m.depsList.SetItems([]list.Item{depItem{Dep: dep}})

	// Press Enter while on deps tab.
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := nm.(AppModel)
	if !mm.depDetailOpen {
		t.Fatalf("expected dep detail modal to open")
	}
	if mm.depDetailDep.Name != dep.Name || mm.depDetailDep.Repository != dep.Repository || mm.depDetailDep.Version != dep.Version {
		t.Fatalf("expected dep detail dep to be set, got %+v", mm.depDetailDep)
	}
}
func TestDepDetailPreviewsMsgPopulatesBuffers(t *testing.T) {
	// Keep this test focused on the message wiring; disable color so highlighted
	// output doesn't change string equality assertions, and markdown rendering
	// returns raw strings.
	t.Setenv("NO_COLOR", "1")

	m := NewAppModel(Params{RepoRoot: "."})
	dep := yamlchart.Dependency{Name: "nginx", Repository: "https://example.com", Version: "0.1.0"}
	m.depDetailOpen = true
	m.depDetailDep = dep
	m.depDetailLoading = true

	readme := "# nginx\n"
	vals := "replicaCount: 1\n"

	nm, _ := m.Update(depDetailPreviewsMsg{ID: yamlchart.DependencyID(dep), readme: readme, defaultValues: vals, schema: ""})
	mm := nm.(AppModel)
	if mm.depDetailLoading {
		t.Fatalf("expected loading=false")
	}
	if mm.depDetailReadme != readme {
		t.Fatalf("expected readme to be set")
	}
	if mm.depDetailDefaultValues != vals {
		t.Fatalf("expected default values to be set")
	}
}

func TestDepDetailVersionsEnterSetsPendingVersion(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "."})
	dep := yamlchart.Dependency{Name: "nginx", Repository: "https://example.com", Version: "0.1.0"}
	m.depDetailOpen = true
	m.depDetailDep = dep
	// Versions tab is last.
	m.depDetailTab = len(m.depDetailTabNames) - 1
	if len(m.depDetailTabKinds) == 0 || m.depDetailTabKinds[m.depDetailTab] != depDetailTabVersions {
		// In case default tabs change, force the last kind to versions for this test.
		m.depDetailTabKinds = make([]depDetailTabKind, len(m.depDetailTabNames))
		for i := range m.depDetailTabKinds {
			m.depDetailTabKinds[i] = depDetailTabValues
		}
		m.depDetailTabKinds[m.depDetailTab] = depDetailTabVersions
	}
	m.depDetailMode = depEditModeList
	m.depDetailVersions.SetItems([]list.Item{versionItem("1.0.0"), versionItem("1.1.0")})
	m.depDetailVersions.Select(1)

	// Use the modal update directly (Update() routes here when depDetailOpen).
	nm, _ := m.depDetailUpdate(tea.KeyMsg{Type: tea.KeyEnter})
	mm := nm.(AppModel)
	if mm.depDetailPendingVersion != "1.1.0" {
		t.Fatalf("expected pending version to be set, got %q", mm.depDetailPendingVersion)
	}
}

func TestDepDetailDependencyTabEnterStartsAliasEditDoesNotApply(t *testing.T) {
	// Ensure Enter on the Dependency/Settings tab focuses the alias input (edit mode)
	// but does not apply immediately.
	m := NewAppModel(Params{RepoRoot: "."})
	dep := yamlchart.Dependency{Name: "nginx", Repository: "https://example.com", Version: "0.1.0", Alias: "old"}
	m.depDetailOpen = true
	m.depDetailDep = dep
	m.depDetailLoading = false
	// Force active tab kind to Dependency.
	m.depDetailTabNames = []string{"Configure", "Dependency"}
	m.depDetailTabKinds = []depDetailTabKind{depDetailTabValues, depDetailTabDependency}
	m.depDetailTab = 1
	m.depDetailAliasInput.SetValue("new")
	m.depDetailAliasInput.Blur()

	nm, _ := m.depDetailUpdate(tea.KeyMsg{Type: tea.KeyEnter})
	mm := nm.(AppModel)
	if !mm.depDetailAliasInput.Focused() {
		t.Fatalf("expected alias input to be focused after first enter")
	}
	if mm.depDetailDep.Alias != dep.Alias {
		t.Fatalf("expected alias not to be applied on first enter; dep alias changed to %q", mm.depDetailDep.Alias)
	}
}

func TestDepDetailDependencyTabEscWhileEditingBlursAndRevertsValue(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "."})
	dep := yamlchart.Dependency{Name: "nginx", Repository: "https://example.com", Version: "0.1.0", Alias: "keep"}
	m.depDetailOpen = true
	m.depDetailDep = dep
	m.depDetailLoading = false
	// Force active tab kind to Dependency.
	m.depDetailTabNames = []string{"Configure", "Dependency"}
	m.depDetailTabKinds = []depDetailTabKind{depDetailTabValues, depDetailTabDependency}
	m.depDetailTab = 1

	// Start edit.
	m.depDetailAliasInput.SetValue("changed")
	m.depDetailAliasInput.Focus()

	nm, _ := m.depDetailUpdate(tea.KeyMsg{Type: tea.KeyEsc})
	mm := nm.(AppModel)
	if mm.depDetailAliasInput.Focused() {
		t.Fatalf("expected alias input to be blurred after esc")
	}
	if got := mm.depDetailAliasInput.Value(); got != "keep" {
		t.Fatalf("expected alias input value to be reverted to %q, got %q", "keep", got)
	}
}

func TestDepAliasAppliedMsgKeepsDepDetailDepInSync(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "."})
	m.depDetailOpen = true
	m.depDetailDep = yamlchart.Dependency{Name: "nginx", Repository: "https://example.com", Version: "1.0.0", Alias: "old"}

	updated := yamlchart.Dependency{Name: "nginx", Repository: "https://example.com", Version: "1.0.0", Alias: "new"}
	chart := yamlchart.Chart{Dependencies: []yamlchart.Dependency{updated}}

	nm, _ := m.Update(depAliasAppliedMsg{chart: chart, dep: updated})
	mm := nm.(AppModel)
	if got := mm.depDetailDep.Alias; got != "new" {
		t.Fatalf("expected depDetailDep alias to be updated, got %q", got)
	}
}

func TestDepDetailTabsCatalogIncludesSetsFirst(t *testing.T) {
	names, kinds := depDetailTabs(depSourceMeta{Kind: depSourceCatalog, CatalogID: "x"}, true)
	if len(kinds) == 0 || kinds[0] != depDetailTabSets {
		t.Fatalf("expected Sets to be first tab for catalog deps; kinds=%#v names=%#v", kinds, names)
	}
}

func TestDepSourceTagAndLabel_CatalogShowsSourceAndEntryID(t *testing.T) {
	tag, label := depSourceTagAndLabel(depSourceMeta{Kind: depSourceCatalog, CatalogID: "bitnami-nginx-15.0.0", CatalogSource: "remote-source"}, true)
	if tag == "" || label == "" {
		t.Fatalf("expected non-empty tag/label")
	}
	if want := "remote-source"; !strings.Contains(tag, want) {
		t.Fatalf("expected tag to contain %q, got %q", want, tag)
	}
	if want := "remote-source"; !strings.Contains(label, want) {
		t.Fatalf("expected label to contain %q, got %q", want, label)
	}
	if want := "bitnami-nginx-15.0.0"; !strings.Contains(label, want) {
		t.Fatalf("expected label to contain %q, got %q", want, label)
	}
}

func TestDepDetailTabsNonCatalogHidesSets(t *testing.T) {
	_, kinds := depDetailTabs(depSourceMeta{Kind: depSourceArbitrary}, true)
	for _, k := range kinds {
		if k == depDetailTabSets {
			t.Fatalf("expected Sets to be hidden for non-catalog deps")
		}
	}
}

func TestDepDetailTabsIncludesDependencyTab(t *testing.T) {
	_, kinds := depDetailTabs(depSourceMeta{Kind: depSourceArbitrary}, true)
	found := false
	for _, k := range kinds {
		if k == depDetailTabDependency {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected Settings tab (depDetailTabDependency kind) to be present")
	}
}

func TestDepDetailTabsRenamesDependencyTabToSettings(t *testing.T) {
	names, kinds := depDetailTabs(depSourceMeta{Kind: depSourceArbitrary}, true)
	idx := -1
	for i, k := range kinds {
		if k == depDetailTabDependency {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatalf("expected Settings tab to be present")
	}
	if !strings.Contains(names[idx], "Settings") {
		t.Fatalf("expected Settings tab label, got %q", names[idx])
	}
}

func TestDepDetailTabsRenamesValuesTabToConfigure(t *testing.T) {
	names, kinds := depDetailTabs(depSourceMeta{Kind: depSourceArbitrary}, true)
	idx := -1
	for i, k := range kinds {
		if k == depDetailTabValues {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatalf("expected Configure tab to be present")
	}
	if !strings.Contains(names[idx], "Configure") {
		t.Fatalf("expected Configure tab label, got %q", names[idx])
	}
}

func TestDepDetailSetsTabLeftRightSwitchTabs(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "."})
	m.depDetailOpen = true
	m.depDetailTabNames, m.depDetailTabKinds = depDetailTabs(depSourceMeta{Kind: depSourceCatalog, CatalogID: "x"}, true)
	m.depDetailTab = 0
	if len(m.depDetailTabKinds) == 0 || m.depDetailTabKinds[0] != depDetailTabSets {
		t.Fatalf("test setup expected Sets first")
	}

	// Left should move away from Sets.
	nm, _ := m.depDetailUpdate(tea.KeyMsg{Type: tea.KeyLeft})
	mm := nm.(AppModel)
	if mm.depDetailTabKinds[mm.depDetailTab] == depDetailTabSets {
		t.Fatalf("expected left to switch tabs away from Sets")
	}

	// Right should also switch (from whatever tab we landed on).
	nm2, _ := mm.depDetailUpdate(tea.KeyMsg{Type: tea.KeyRight})
	mm2 := nm2.(AppModel)
	if mm2.depDetailTab == mm.depDetailTab {
		t.Fatalf("expected right to switch tabs")
	}
}

// NOTE: the dependency actions modal (x) was removed.
