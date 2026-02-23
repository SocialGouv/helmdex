package tui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"helmdex/internal/instances"

	tea "github.com/charmbracelet/bubbletea"
)

func TestValuesTabEnterOpensPreviewModal(t *testing.T) {
	tmp := t.TempDir()
	instPath := filepath.Join(tmp, "apps", "x")
	if err := os.MkdirAll(instPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(instPath, "values.instance.yaml"), []byte("a: 1\n"), 0o644); err != nil {
		t.Fatalf("write values.instance.yaml: %v", err)
	}

	m := NewAppModel(Params{RepoRoot: tmp})
	m.screen = ScreenInstance
	m.selected = &instances.Instance{Name: "x", Path: instPath}
	m.activeTab = 2 // Values tab unchanged
	m.refreshValuesList()

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := nm.(AppModel)
	if !mm.valuesPreviewOpen {
		t.Fatalf("expected values preview modal to open")
	}
}

func TestValuesTabEscClosesPreviewModal(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "."})
	m.screen = ScreenInstance
	m.activeTab = 2
	m.valuesPreviewOpen = true
	m.valuesPreviewPath = "values.instance.yaml"

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	mm := nm.(AppModel)
	if mm.valuesPreviewOpen {
		t.Fatalf("expected values preview modal to close")
	}
}

func TestValuesTabListShowsDescriptionsAndOrdering(t *testing.T) {
	tmp := t.TempDir()
	instPath := filepath.Join(tmp, "apps", "x")
	if err := os.MkdirAll(instPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Create a representative set of values files.
	files := map[string]string{
		"values.default.yaml":      "a: 1\n",
		"values.platform.yaml":     "b: 2\n",
		"values.set.team-a.yaml":   "c: 3\n",
		"values.instance.yaml":     "d: 4\n",
		"values.yaml":              "# generated\n",
		"values.set.team-b.yaml":   "e: 5\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(instPath, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		// Ensure stable mtimes aren’t relied upon (ordering is explicit/sorted).
		_ = os.Chtimes(filepath.Join(instPath, name), time.Now(), time.Now())
	}

	m := NewAppModel(Params{RepoRoot: tmp})
	m.screen = ScreenInstance
	m.selected = &instances.Instance{Name: "x", Path: instPath}
	m.activeTab = 2
	m.refreshValuesList()

	items := m.valuesList.Items()
	if len(items) != 6 {
		t.Fatalf("expected 6 values items, got %d", len(items))
	}

	// Verify ordering:
	// default, platform, set layers (sorted), instance, merged.
	gotTitles := []string{}
	gotDescs := []string{}
	for _, it := range items {
		gotTitles = append(gotTitles, it.(valuesFileItem).Title())
		gotDescs = append(gotDescs, it.(valuesFileItem).Description())
	}

	wantTitles := []string{
		"values.default.yaml",
		"values.platform.yaml",
		"values.set.team-a.yaml",
		"values.set.team-b.yaml",
		"values.instance.yaml",
		"values.yaml",
	}
	for i := range wantTitles {
		if gotTitles[i] != wantTitles[i] {
			t.Fatalf("unexpected item[%d] title: got %q want %q (all: %#v)", i, gotTitles[i], wantTitles[i], gotTitles)
		}
	}

	// Verify descriptions.
	wantDescs := []string{
		"Baseline defaults",
		"Platform overrides",
		"Preset layer: team-a",
		"Preset layer: team-b",
		"User overrides (editable)",
		"Merged output (generated)",
	}
	for i := range wantDescs {
		if gotDescs[i] != wantDescs[i] {
			t.Fatalf("unexpected item[%d] description: got %q want %q (all: %#v)", i, gotDescs[i], wantDescs[i], gotDescs)
		}
	}
}
