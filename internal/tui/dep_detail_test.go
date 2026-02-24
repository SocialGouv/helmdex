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
	m.depsList.SetItems([]list.Item{depItem(dep)})

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
	// Versions tab is last (Configure was inserted before it).
	m.depDetailTab = len(m.depDetailTabNames) - 1
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

func TestDepDetailTabNamesIncludesSets(t *testing.T) {
	names := depDetailTabNames()
	found := false
	for _, n := range names {
		if strings.Contains(n, "Sets") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected dep detail tab names to include Sets, got %#v", names)
	}
}

func TestDepActionsMenuOpensFromDepsTab(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "."})
	m.screen = ScreenInstance
	m.activeTab = InstanceTabDeps

	dep := yamlchart.Dependency{Name: "nginx", Repository: "https://example.com", Version: "1.2.3"}
	m.depsList.SetItems([]list.Item{depItem(dep)})

	// Press x to open actions menu.
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	mm := nm.(AppModel)
	if !mm.depActionsOpen {
		t.Fatalf("expected dep actions menu to open")
	}
}
