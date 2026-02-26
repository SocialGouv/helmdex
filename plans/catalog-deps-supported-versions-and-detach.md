## Goal

When a dependency is **attached to a catalog** (dep source kind `catalog`), Helmdex must:

1. Prevent selecting a chart version that is **outside the catalog supported range** for that chart.
2. Provide an action to **Detach from catalog**, switching dep source meta to `arbitrary`, after which **any version** can be selected.

This is primarily a TUI behavior change, enforced both in the versions pickers and in final validation.

## Definitions

### Dependency is catalog-attached

A dependency is considered catalog-attached if depmeta exists and is:

- `kind: catalog`
- `catalogSource` is set (required for preset-cache range checks)

See [`depSourceMeta`](internal/tui/depmeta.go:23).

### Catalog supported versions (chosen rule)

Supported versions are determined by **preset coverage** in the synced cache for the catalog source:

`<repoRoot>/.helmdex/cache/<catalogSource>/<chartsPath>/<chartName>/`

Where `<chartsPath>` is resolved from config for that source (default `charts`).

Supported coverage is derived from the directory names directly under that chart root:

- Exact version directories like `15.0.0`
- Semver constraint directories like `>=1.0.0 <2.0.0`

If a Helm version string matches **any** exact dir OR validates against **any** constraint dir, it is supported.

This aligns with how presets are resolved today via [`bestPresetDir()`](internal/presets/resolve.go:93), but here we use the directories to *filter/validate* versions rather than pick one.

### Edge case: missing coverage

If the chart root exists but no dir names match any available Helm versions, Helmdex should:

- Show an empty versions list (no selection possible)
- Surface an error explaining there are no catalog-supported versions and suggest **Detach from catalog**

## UX changes

### 1) Version picker restrictions

Apply to:

- Dep version editor opened from dependencies list (`v` / “Change version”)
- Dep detail modal “Versions” tab

Behavior:

- If source is catalog-attached: show only supported versions.
- If the filtered list is empty: show an error message in the modal (using existing `modalErr`) and no selectable items.

Implementation touchpoints:

- Dep edit modal logic in [`openDepEditSelected()`](internal/tui/model.go:5162) and list fill in [`setVersionsList()`](internal/tui/model.go:1525)
- Dep detail versions list fill in [`setVersionsList()`](internal/tui/model.go:1525) when target is `versionsTargetDepDetail`

### 2) Validation enforcement (defense in depth)

Even if a version slips through (manual entry, stale UI state), final validation must enforce the same rule.

Extend [`validateDependencyVersionCmd()`](internal/tui/model.go:4941) to:

- If dep is catalog-attached, reject versions not supported by preset coverage, with a clear error and detach suggestion.
- Then proceed with the existing Helm validation (`helm show chart`) for non-OCI repos.

### 3) Upgrade to latest restriction

When catalog-attached, “Upgrade to latest” should choose the best stable version **within the supported set**.

Touchpoint: [`upgradeDepToLatestCmd()`](internal/tui/model.go:5236).

### 4) Detach from catalog action

Add an action in the dependency actions menu:

- Visible only when the selected dep is catalog-attached.
- Label: `Detach from catalog`.

Behavior:

- Write depmeta for this dep to: `kind: arbitrary`.
- Clear `catalogID` and `catalogSource`.
- Do not change `Chart.yaml` (repo/name/version remain unchanged).

Touchpoints:

- Actions menu rendered in [`renderDepActionsModal()`](internal/tui/ui.go:185)
- Actions list population and handler in [`openDepActionsSelected()`](internal/tui/model.go:941) and [`depActionsUpdate()`](internal/tui/model.go:980)
- Persisting meta via [`writeSelectedDepSourceMeta()`](internal/tui/depmeta.go:172)

## Data/model changes

No schema changes required.

We will reuse existing depmeta in `.helmdex/depmeta/<instance>/<depID>.yaml`.

## Proposed implementation outline

### A) Compute supported versions from preset cache

Add a helper that:

1. Reads directory names under the chart root.
2. Builds:
   - `exactSet` of exact versions
   - list of semver constraints (Masterminds)
3. Filters a candidate versions list.

Placement options:

- New helper near [`presets.Resolve()`](internal/presets/resolve.go:33) (recommended)
- Or a new file in `internal/presets/` to avoid mixing concerns

Notes:

- Directory names that are neither a valid semver nor a valid constraint are ignored.
- Wildcard dirs like `1.2.x` are currently only mentioned in specs; if needed, implement later.

### B) Resolve chartsPath for a catalog source

When `depSourceMeta.Kind == catalog` and we have `CatalogSource`:

- Find that source entry in loaded config (`m.params.Config.Sources`).
- Use `src.Presets.ChartsPath` or default `charts`.

If config is missing or the source cannot be found:

- Treat as missing coverage and block version changes with an actionable error.

### C) Wire into TUI version listing

Approach:

- Store dep source meta for the dep edit modal similarly to `depActionsSource` and `depDetailSource`.
- When setting versions list for dep edit / dep detail, filter based on computed supported versions.

### D) Wire into validation

Before Helm validation in [`validateDependencyVersionCmd()`](internal/tui/model.go:4941):

- If catalog-attached and `!supported(dep.Version)`: return `errMsg{fmt.Errorf(...)}`.

### E) Detach action

- Add action item constant and render it conditionally.
- On selection, call `writeSelectedDepSourceMeta(dep, depSourceMeta{Kind: depSourceArbitrary})`.
- Update any in-memory source meta (so the UI immediately reflects “Arbitrary”).

## Sets tab behavior after detach

Default proposal: keep current behavior (Sets tab only for catalog deps) via [`depDetailTabs()`](internal/tui/model.go:455).

Rationale: once detached, user may pick versions without preset coverage; keeping Sets visible risks confusing UX when sets cannot be resolved.

## Tests

Add tests for:

- Helper behavior: constraint/exact parsing and filtering outcomes.
- UI logic: catalog-attached filtering + empty-coverage error.
- Validation: manual entry rejection when unsupported.
- Detach action visibility + depmeta persistence.

Likely touchpoints include [`internal/tui/dep_detail_test.go`](internal/tui/dep_detail_test.go:1).

