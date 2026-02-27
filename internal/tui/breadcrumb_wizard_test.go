package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	"helmdex/internal/artifacthub"
	"helmdex/internal/catalog"
	"helmdex/internal/instances"
)

func TestBreadcrumbShowsAddDepInsteadOfUnderlyingTabWhenWizardOpen(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "."})
	m.screen = ScreenInstance
	m.selected = &instances.Instance{Name: "my-app"}
	m.activeTab = InstanceTabDeps
	m.addingDep = true
	m.depStep = depStepChooseSource

	b := renderTopBar(m)
	if !strings.Contains(b, "Add dep") {
		t.Fatalf("expected top bar to include Add dep when wizard is open; got %q", b)
	}
	if strings.Contains(b, "Dependencies") {
		t.Fatalf("expected top bar NOT to include underlying tab when wizard is open; got %q", b)
	}
}

func TestBreadcrumbAddDepCatalogDetailIncludesSourceAndEntryID(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "."})
	m.screen = ScreenInstance
	m.selected = &instances.Instance{Name: "demo"}
	m.activeTab = InstanceTabDeps
	m.addingDep = true
	m.depStep = depStepCatalogDetail
	m.catalogDetailEntry = &catalog.EntryWithSource{SourceName: "remote-source", Entry: catalog.Entry{ID: "bitnami-nginx-15.0.0"}}

	b := renderTopBar(m)
	if !strings.Contains(b, "Catalog") || !strings.Contains(b, "remote-source") || !strings.Contains(b, "bitnami-nginx-15.0.0") {
		t.Fatalf("expected top bar to include catalog source+entry; got %q", b)
	}
}

func TestBreadcrumbAddDepArtifactHubDetailIncludesChartAndVersion(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "."})
	m.screen = ScreenInstance
	m.selected = &instances.Instance{Name: "demo"}
	m.activeTab = InstanceTabDeps
	m.addingDep = true
	m.depStep = depStepAHDetail

	sel := artifacthub.PackageSummary{Name: "postgresql", DisplayName: "postgresql"}
	m.ahSelected = &sel
	m.ahSelectedVersion = "15.5.0"

	b := renderTopBar(m)
	if !strings.Contains(b, "Artifact Hub") || !strings.Contains(b, "postgresql") || !strings.Contains(b, "15.5.0") {
		t.Fatalf("expected top bar to include artifact hub selection+version; got %q", b)
	}
}

func TestBreadcrumbAddDepCatalogListIncludesSelectedEntry(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "."})
	m.screen = ScreenInstance
	m.selected = &instances.Instance{Name: "demo"}
	m.activeTab = InstanceTabDeps
	m.addingDep = true
	m.depStep = depStepCatalog

	// Simulate a populated catalog list with a selected item.
	m.catalogList.SetItems([]list.Item{
		catalogListItem{E: catalog.EntryWithSource{SourceName: "remote-source", Entry: catalog.Entry{ID: "bitnami-nginx-15.0.0"}}},
		catalogListItem{E: catalog.EntryWithSource{SourceName: "other", Entry: catalog.Entry{ID: "other-thing"}}},
	})
	m.catalogList.Select(0)

	b := renderTopBar(m)
	if !strings.Contains(b, "Catalog") || !strings.Contains(b, "remote-source") || !strings.Contains(b, "bitnami-nginx-15.0.0") {
		t.Fatalf("expected top bar to include catalog selected entry; got %q", b)
	}
}
