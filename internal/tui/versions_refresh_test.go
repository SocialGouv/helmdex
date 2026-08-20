package tui

import (
	"fmt"
	"testing"

	"helmdex/internal/yamlchart"
)

func modelWithDepEditOpen(dep yamlchart.Dependency) *AppModel {
	m := NewAppModel(Params{RepoRoot: "."})
	m.depEditOpen = true
	m.depEditDep = dep
	m.depEditMode = depEditModeList
	return &m
}

func errResult(dep yamlchart.Dependency, target versionsTarget) versionsRefreshResultMsg {
	return versionsRefreshResultMsg{
		key:       versionsKey(dep.Repository, dep.Name),
		repoURL:   dep.Repository,
		chartName: dep.Name,
		depID:     yamlchart.DependencyID(dep),
		target:    target,
		err:       fmt.Errorf("registry timeout"),
	}
}

// A failed refresh must not hide versions the modal already has: dropping to
// manual entry is for an empty picker, not for a transient registry error on
// top of a warm cache.
func TestVersionsRefreshError_KeepsCachedList(t *testing.T) {
	dep := yamlchart.Dependency{Name: "demo", Repository: "oci://registry.test/org/demo", Version: "1.0.0"}
	m := modelWithDepEditOpen(dep)
	m.setVersionsList([]string{"2.0.0", "1.0.0"}, versionsTargetDepEdit)
	m.depEditLoading = true

	m.handleVersionsRefreshResult(errResult(dep, versionsTargetDepEdit))

	if m.depEditMode != depEditModeList {
		t.Fatal("a cached list must stay usable when a refresh fails")
	}
	if len(m.depEditVersionsData) != 2 {
		t.Fatalf("cached versions lost: %v", m.depEditVersionsData)
	}
	if m.modalErr == "" {
		t.Fatal("the failure must still be reported")
	}
}

// With no versions to show, the same failure must fall back to manual entry so
// an unlistable registry never leaves the user unable to set a version.
func TestVersionsRefreshError_FallsBackToManualWhenEmpty(t *testing.T) {
	dep := yamlchart.Dependency{Name: "demo", Repository: "oci://registry.test/org/demo", Version: "1.0.0"}
	m := modelWithDepEditOpen(dep)
	m.depEditLoading = true

	m.handleVersionsRefreshResult(errResult(dep, versionsTargetDepEdit))

	if m.depEditMode != depEditModeManual {
		t.Fatal("an empty picker must fall back to manual entry")
	}
	if got := m.depEditVersionInput.Value(); got != "1.0.0" {
		t.Fatalf("manual input seeded with %q", got)
	}
	if m.modalErr == "" {
		t.Fatal("the failure must be reported")
	}
}

// Single-flight is keyed per dependency, not per modal: a reopened modal
// routinely rides on a refresh scheduled by the previous open, which never
// sets this open's loading flag. The failure must still reach the user rather
// than leaving an empty picker with no error and no way to retry.
func TestVersionsRefreshError_SurfacedOnModalRidingAnotherRefresh(t *testing.T) {
	dep := yamlchart.Dependency{Name: "demo", Repository: "oci://registry.test/org/demo", Version: "1.0.0"}
	m := modelWithDepEditOpen(dep)
	// Reopened while an earlier refresh was still in flight: nothing scheduled
	// for this open, so no loading flag.
	m.depEditLoading = false

	m.handleVersionsRefreshResult(errResult(dep, versionsTargetDepEdit))

	if m.modalErr == "" {
		t.Fatal("the failure was swallowed; the user is left with an empty picker")
	}
	if m.depEditMode != depEditModeManual {
		t.Fatal("an empty picker must fall back to manual entry")
	}
}

// The loading flag belongs to the modal that is open. A late result for a
// different dependency must not clear it, or the open modal's own failure is
// mistaken for someone else's and swallowed.
func TestVersionsRefreshResult_OtherDepDoesNotDisturbOpenModal(t *testing.T) {
	shown := yamlchart.Dependency{Name: "shown", Repository: "oci://registry.test/org/shown", Version: "1.0.0"}
	other := yamlchart.Dependency{Name: "other", Repository: "oci://registry.test/org/other", Version: "2.0.0"}
	m := modelWithDepEditOpen(shown)
	m.depEditLoading = true

	m.handleVersionsRefreshResult(errResult(other, versionsTargetDepEdit))

	if !m.depEditLoading {
		t.Fatal("a result for another dependency cleared the open modal's loading flag")
	}
	if m.modalErr != "" {
		t.Fatalf("another dependency's error leaked into this modal: %q", m.modalErr)
	}
}

// The periodic background refresh uses its own target. When it fails for the
// dependency a modal is currently showing, that modal must learn about it.
func TestVersionsRefreshError_BackgroundResultReachesOpenModal(t *testing.T) {
	dep := yamlchart.Dependency{Name: "demo", Repository: "oci://registry.test/org/demo", Version: "1.0.0"}
	m := modelWithDepEditOpen(dep)

	m.handleVersionsRefreshResult(errResult(dep, versionsTargetBackground))

	if m.modalErr == "" {
		t.Fatal("a background failure for the displayed dependency must surface")
	}
	if m.depEditMode != depEditModeManual {
		t.Fatal("an empty picker must fall back to manual entry")
	}
}

// A successful result is always worth applying, whoever scheduled it.
func TestVersionsRefreshSuccess_AppliedFromAnyTarget(t *testing.T) {
	dep := yamlchart.Dependency{Name: "demo", Repository: "oci://registry.test/org/demo", Version: "1.0.0"}
	m := modelWithDepEditOpen(dep)
	m.depEditLoading = false

	msg := errResult(dep, versionsTargetBackground)
	msg.err = nil
	msg.versions = []string{"3.0.0", "2.0.0"}
	m.handleVersionsRefreshResult(msg)

	if len(m.depEditVersionsData) != 2 {
		t.Fatalf("versions not applied: %v", m.depEditVersionsData)
	}
}

// A registry whose tags are all non-SemVer (latest, main, artifacthub.io)
// lists successfully but yields no versions. An empty picker is a dead end
// however it arose, so it must offer manual entry too.
func TestVersionsRefreshSuccess_EmptyListFallsBackToManual(t *testing.T) {
	dep := yamlchart.Dependency{Name: "demo", Repository: "oci://registry.test/org/demo", Version: "1.0.0"}
	m := modelWithDepEditOpen(dep)
	m.depEditLoading = true

	msg := errResult(dep, versionsTargetDepEdit)
	msg.err = nil
	msg.versions = nil
	m.handleVersionsRefreshResult(msg)

	if m.depEditMode != depEditModeManual {
		t.Fatal("a successful but empty listing must still allow typing a tag")
	}
	if got := m.depEditVersionInput.Value(); got != "1.0.0" {
		t.Fatalf("manual input seeded with %q", got)
	}
}

// Manual entry is a fallback, not a latch: when a later refresh brings versions
// back, the picker must return.
func TestVersionsRefresh_ManualModeIsNotSticky(t *testing.T) {
	dep := yamlchart.Dependency{Name: "demo", Repository: "oci://registry.test/org/demo", Version: "1.0.0"}
	m := modelWithDepEditOpen(dep)
	m.handleVersionsRefreshResult(errResult(dep, versionsTargetBackground))
	if m.depEditMode != depEditModeManual {
		t.Fatal("precondition: an empty listing should fall back to manual")
	}

	ok := errResult(dep, versionsTargetBackground)
	ok.err = nil
	ok.versions = []string{"2.0.0", "1.0.0"}
	m.handleVersionsRefreshResult(ok)

	if m.depEditMode != depEditModeList {
		t.Fatal("the picker must come back once versions are available again")
	}
}

// The background tick fires on its own schedule; it must not overwrite a tag
// the user is in the middle of typing.
func TestVersionsRefresh_DoesNotClobberManualInput(t *testing.T) {
	dep := yamlchart.Dependency{Name: "demo", Repository: "oci://registry.test/org/demo", Version: "1.0.0"}
	m := modelWithDepEditOpen(dep)
	m.handleVersionsRefreshResult(errResult(dep, versionsTargetBackground))
	m.depEditVersionInput.SetValue("2.5.0")

	m.handleVersionsRefreshResult(errResult(dep, versionsTargetBackground))

	if got := m.depEditVersionInput.Value(); got != "2.5.0" {
		t.Fatalf("a later refresh erased what the user typed: %q", got)
	}
}

// Dependencies are identified by alias-or-name, which repeats across
// repositories. A result must be matched on repository + chart, or one
// vendor's versions land in another's picker.
func TestVersionsRefresh_DoesNotApplyOtherRepositorysVersions(t *testing.T) {
	shown := yamlchart.Dependency{Name: "cache", Repository: "oci://vendor-a.test/charts", Version: "1.0.0"}
	other := yamlchart.Dependency{Name: "cache", Repository: "oci://vendor-b.test/charts", Version: "1.0.0"}
	m := modelWithDepEditOpen(shown)
	m.setVersionsList([]string{"1.0.0"}, versionsTargetDepEdit)

	msg := errResult(other, versionsTargetBackground)
	msg.err = nil
	msg.versions = []string{"666.0.0"}
	m.handleVersionsRefreshResult(msg)

	for _, v := range m.depEditVersionsData {
		if v == "666.0.0" {
			t.Fatalf("versions from %s applied to a modal showing %s", other.Repository, shown.Repository)
		}
	}
}

// The picker can come and go while the user is typing a tag: an empty listing
// drops to manual, a later refresh restores the list, and a further empty one
// drops back. Re-entering manual must not erase what was typed.
func TestVersionsRefresh_ModeOscillationKeepsUserInput(t *testing.T) {
	dep := yamlchart.Dependency{Name: "demo", Repository: "oci://registry.test/org/demo", Version: "1.0.0"}
	m := modelWithDepEditOpen(dep)

	m.handleVersionsRefreshResult(errResult(dep, versionsTargetBackground))
	m.depEditVersionInput.SetValue("3.14.0")

	ok := errResult(dep, versionsTargetBackground)
	ok.err = nil
	ok.versions = []string{"2.0.0"}
	m.handleVersionsRefreshResult(ok)
	if m.depEditMode != depEditModeList {
		t.Fatal("precondition: versions should have restored the picker")
	}

	// Back to empty — a successful listing that yields nothing, so the picker
	// really does empty out and manual mode is re-entered.
	empty := errResult(dep, versionsTargetBackground)
	empty.err = nil
	empty.versions = nil
	m.handleVersionsRefreshResult(empty)

	if m.depEditMode != depEditModeManual {
		t.Fatal("precondition: an empty listing should re-enter manual")
	}
	if got := m.depEditVersionInput.Value(); got != "3.14.0" {
		t.Fatalf("mode oscillation erased the typed version: %q", got)
	}
}
