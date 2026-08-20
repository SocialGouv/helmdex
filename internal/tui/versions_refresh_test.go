package tui

import (
	"fmt"
	"testing"

	"helmdex/internal/yamlchart"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// filterVersions drives the version list into its filtering state the way a
// user does: "/" then the query.
func filterVersions(m *AppModel, query string) {
	m.depEditVersions, _ = m.depEditVersions.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range query {
		m.depEditVersions, _ = m.depEditVersions.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

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

	// The picker must not be restored under someone mid-tag: it would hide
	// their text and apply the list's selection on Enter instead.
	if m.depEditMode != depEditModeManual {
		t.Fatal("a refresh flipped the mode while the user was typing")
	}

	empty := errResult(dep, versionsTargetBackground)
	empty.err = nil
	empty.versions = nil
	m.handleVersionsRefreshResult(empty)

	if got := m.depEditVersionInput.Value(); got != "3.14.0" {
		t.Fatalf("mode oscillation erased the typed version: %q", got)
	}
}

// bubbles' SetItems drops the filtered view, and rebuilding it is the caller's
// job. This drives the result through the app's real Update, because that is
// where a rebuild scheduled as a command would be lost: nothing routes a
// FilterMatchesMsg back to a version list.
func TestVersionsRefresh_KeepsAFilteredViewPopulated(t *testing.T) {
	dep := yamlchart.Dependency{Name: "demo", Repository: "oci://registry.test/org/demo", Version: "1.0.0"}
	m := modelWithDepEditOpen(dep)
	m.setVersionsList([]string{"1.0.0", "1.1.0", "2.0.0"}, versionsTargetDepEdit)
	filterVersions(m, "1")

	ok := errResult(dep, versionsTargetBackground)
	ok.err = nil
	ok.versions = []string{"1.0.0", "1.1.0", "2.0.0"}

	next, _ := m.Update(ok)
	mm := next.(AppModel)

	if got := len(mm.depEditVersions.VisibleItems()); got != 2 {
		t.Fatalf("filter %q matched %d items after the refresh, want 2",
			mm.depEditVersions.FilterValue(), got)
	}
}

// A background refresh must not drag the cursor away from where the user was
// browsing.
func TestVersionsRefresh_KeepsTheBrowsingPosition(t *testing.T) {
	dep := yamlchart.Dependency{Name: "demo", Repository: "oci://registry.test/org/demo", Version: "1.0.0"}
	m := modelWithDepEditOpen(dep)
	versions := []string{"3.0.0", "2.0.0", "1.0.0"}
	m.setVersionsList(versions, versionsTargetDepEdit)
	m.depEditVersions.Select(0) // browsing 3.0.0, not the pinned 1.0.0

	ok := errResult(dep, versionsTargetBackground)
	ok.err = nil
	ok.versions = versions
	m.handleVersionsRefreshResult(ok)

	it, _ := m.depEditVersions.SelectedItem().(versionItem)
	if it.Ver != "3.0.0" {
		t.Fatalf("cursor jumped to %q; the user was browsing 3.0.0", it.Ver)
	}
}

// Dropping to manual hides the list, but its filter state would survive and
// swallow the next keystrokes.
func TestVersionsRefresh_ClearsTheFilterWhenLeavingTheList(t *testing.T) {
	dep := yamlchart.Dependency{Name: "demo", Repository: "oci://registry.test/org/demo", Version: "1.0.0"}
	m := modelWithDepEditOpen(dep)
	m.setVersionsList([]string{"1.0.0", "1.1.0"}, versionsTargetDepEdit)
	filterVersions(m, "1")

	empty := errResult(dep, versionsTargetBackground)
	empty.err = nil
	empty.versions = nil
	m.handleVersionsRefreshResult(empty)

	if m.depEditMode != depEditModeManual {
		t.Fatal("precondition: an empty listing should drop to manual")
	}
	if got := m.depEditVersions.FilterValue(); got != "" {
		t.Fatalf("an invisible filter %q survived into manual mode", got)
	}
}

// When the version the user was browsing disappears between refreshes, the
// cursor belongs on the pinned version — not on whatever happens to be first.
func TestVersionsRefresh_FallsBackToThePinnedVersion(t *testing.T) {
	dep := yamlchart.Dependency{Name: "demo", Repository: "oci://registry.test/org/demo", Version: "1.0.0"}
	m := modelWithDepEditOpen(dep)
	m.setVersionsList([]string{"3.0.0", "1.5.0", "1.0.0"}, versionsTargetDepEdit)
	m.depEditVersions.Select(1) // browsing 1.5.0

	ok := errResult(dep, versionsTargetBackground)
	ok.err = nil
	ok.versions = []string{"3.0.0", "1.0.0"} // 1.5.0 withdrawn
	m.handleVersionsRefreshResult(ok)

	it, _ := m.depEditVersions.SelectedItem().(versionItem)
	if it.Ver != "1.0.0" {
		t.Fatalf("cursor landed on %q; the browsed version is gone, so it belongs on the pinned one", it.Ver)
	}
}

// A catalog-attached dependency whose supported set is empty must not be
// offered a free-text field: that contradicts the message telling the user to
// detach from the catalog first.
func TestVersionsRefresh_CatalogEmptinessStaysInListMode(t *testing.T) {
	dep := yamlchart.Dependency{Name: "demo", Repository: "https://charts.test", Version: "1.0.0"}
	m := modelWithDepEditOpen(dep)
	m.depEditSourceOK = true
	m.depEditSource = depSourceMeta{Kind: depSourceCatalog, CatalogID: "demo-1.0.0"}

	empty := errResult(dep, versionsTargetBackground)
	empty.err = nil
	empty.versions = nil
	m.handleVersionsRefreshResult(empty)

	if m.depEditMode == depEditModeManual {
		t.Fatal("a catalog-attached dependency must not be offered free-text version entry")
	}
}

// The list's Select takes an index into the visible rows, which is the filtered
// slice while a filter is active. Anchoring on an unfiltered index leaves the
// cursor on the wrong row — or past the end, where Enter silently does nothing.
func TestVersionsRefresh_CursorStaysSelectableUnderAFilter(t *testing.T) {
	dep := yamlchart.Dependency{Name: "demo", Repository: "oci://registry.test/org/demo", Version: "9.0.0"}
	m := modelWithDepEditOpen(dep)
	// "1.0.0" sits at unfiltered index 2 but is the first row once "1" filters.
	versions := []string{"10.0.0", "9.0.0", "1.0.0"}
	m.setVersionsList(versions, versionsTargetDepEdit)
	filterVersions(m, "1")
	m.depEditVersions.Select(0)

	// Whatever the fuzzy filter ranked first is the row the user is on.
	browsing, ok := m.depEditVersions.SelectedItem().(versionItem)
	if !ok {
		t.Fatal("precondition: the filtered list should have a selection")
	}

	res := errResult(dep, versionsTargetBackground)
	res.err = nil
	res.versions = versions

	next, _ := m.Update(res)
	mm := next.(AppModel)

	after, ok := mm.depEditVersions.SelectedItem().(versionItem)
	if !ok {
		t.Fatalf("nothing is selected after the refresh: Enter would silently do nothing (index %d of %d visible)",
			mm.depEditVersions.Index(), len(mm.depEditVersions.VisibleItems()))
	}
	if after.Ver != browsing.Ver {
		t.Fatalf("cursor moved from %q to %q across a refresh", browsing.Ver, after.Ver)
	}
}

// The version list widget is shared across dependencies. A filter typed on one
// must not survive into the next open and silently hide every version.
func TestOpenDepEdit_DoesNotInheritThePreviousFilter(t *testing.T) {
	alpha := yamlchart.Dependency{Name: "alpha", Repository: "oci://reg.test/org", Version: "1.0.0"}
	m := modelWithDepEditOpen(alpha)
	m.setVersionsList([]string{"1.0.0", "1.1.0", "2.0.0"}, versionsTargetDepEdit)
	filterVersions(m, "1")
	if m.depEditVersions.FilterValue() == "" {
		t.Fatal("precondition: a filter should be active")
	}

	// Reopen on a different dependency whose versions share no digit with it.
	beta := yamlchart.Dependency{Name: "beta", Repository: "oci://reg.test/org", Version: "9.0.0"}
	m.depEditOpen = false
	m.depsList.SetItems([]list.Item{depItem{Dep: beta}})
	m.depsList.Select(0)
	nm, _ := m.openDepEditSelected()
	mm := nm.(AppModel)
	mm.setVersionsList([]string{"9.0.0", "8.5.0"}, versionsTargetDepEdit)

	if got := mm.depEditVersions.FilterValue(); got != "" {
		t.Fatalf("filter %q leaked into the next dependency", got)
	}
	if n := len(mm.depEditVersions.VisibleItems()); n != 2 {
		t.Fatalf("%d versions visible after reopening; a stale filter is hiding them", n)
	}
}

// The manual fallback focuses the version input; if nothing blurs it, the next
// list-mode open captures keys meant for the list.
func TestOpenDepEdit_StartsWithTheVersionInputBlurred(t *testing.T) {
	dep := yamlchart.Dependency{Name: "demo", Repository: "oci://reg.test/org", Version: "1.0.0"}
	m := modelWithDepEditOpen(dep)
	m.depEditVersionInput.Focus() // as the manual fallback leaves it

	m.depEditOpen = false
	m.depsList.SetItems([]list.Item{depItem{Dep: dep}})
	m.depsList.Select(0)
	nm, _ := m.openDepEditSelected()
	mm := nm.(AppModel)

	if mm.depEditVersionInput.Focused() {
		t.Fatal("a list-mode open must not start with the version input focused")
	}
}

// Validation runs in the background, so the user may have opened another
// dependency by the time it lands. The upgrade diff must be between the two
// versions of the dependency being upgraded, not between two different charts.
func TestDepVersionValidated_DiffUsesTheUpgradedDependency(t *testing.T) {
	alpha := yamlchart.Dependency{Name: "alpha", Repository: "oci://reg.test/alpha", Version: "1.0.0"}
	beta := yamlchart.Dependency{Name: "beta", Repository: "oci://reg.test/beta", Version: "9.0.0"}

	m := NewAppModel(Params{RepoRoot: "."})
	m.depsList.SetItems([]list.Item{depItem{Dep: alpha}, depItem{Dep: beta}})
	// The user moved on to beta while alpha's upgrade was validating.
	m.depDetailOpen = true
	m.depDetailDep = beta

	upgraded := alpha
	upgraded.Version = "9.9.9"
	nm, _ := m.Update(depVersionValidatedMsg{dep: upgraded})
	mm := nm.(AppModel)

	if got := mm.depDiffOldDep.Version; got != "1.0.0" {
		t.Fatalf("upgrade diff reports %q as the version being replaced; alpha was at 1.0.0", got)
	}
	if yamlchart.DependencyID(mm.depDiffOldDep) != yamlchart.DependencyID(upgraded) {
		t.Fatalf("diff compares %s against %s", yamlchart.DependencyID(mm.depDiffOldDep), yamlchart.DependencyID(upgraded))
	}
}

// A validation result can land after its dependency is gone — removed, or the
// user moved to another instance. Opening the diff then would offer to apply
// the upgrade to whichever chart is selected now.
func TestDepVersionValidated_DoesNotOpenADiffForAVanishedDependency(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "."})
	other := yamlchart.Dependency{Name: "beta", Repository: "oci://reg.test/beta", Version: "9.0.0"}
	m.depsList.SetItems([]list.Item{depItem{Dep: other}})

	upgraded := yamlchart.Dependency{Name: "alpha", Repository: "oci://reg.test/alpha", Version: "9.9.9"}
	nm, _ := m.Update(depVersionValidatedMsg{dep: upgraded})
	mm := nm.(AppModel)

	if mm.depDiffOpen {
		t.Fatalf("opened an upgrade diff for a dependency no longer in the chart: old=%+v new=%+v",
			mm.depDiffOldDep, mm.depDiffNewDep)
	}
}
