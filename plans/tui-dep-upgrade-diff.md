# TUI: dependency upgrade/change-version with schema/values diff

Goal: in the TUI dependency list, when the user triggers a version change (via `v`) or an upgrade-to-latest (via `u`), helmdex must show a **Diff modal** comparing the old vs new chart:

- Prefer diffing **values schema** (chart file `values.schema.json`).
- If schema is missing on either side, **fallback** to diffing **default values** (chart file `values.yaml`).
- Require explicit confirmation before applying (keys: `y` apply, `n`/`esc` cancel).

This integrates with the existing dependency flows in [`internal/tui/model.go`](internal/tui/model.go:1):

- `v` opens version picker modal and currently validates then applies.
- `u` currently computes best stable semver then immediately applies.

## UX spec (confirmed)

Diff modal:

- Two tabs:
  - Schema diff (preferred)
  - Values diff (fallback only)
- Top summary line: `<depID> <oldVersion> → <newVersion>` plus change counts.
- Keys:
  - `←/→` switch tabs
  - `j/k` or `↑/↓` scroll (viewport)
  - `y` apply
  - `n` cancel
  - `esc` cancel

Modal behavior:

- Open diff modal after target version is known and validated.
- Cancel returns to the previous UI (version picker / dep list) without applying.
- Apply closes the diff modal and runs the existing apply pipeline.

## Optional enhancement: side-by-side diff when wide enough

When the terminal window is sufficiently wide, render the diff content in a **2-column side-by-side** layout (old on the left, new on the right) rather than a single unified stream.

### Trigger

- Auto-enable side-by-side when `depDiffPreview.Width` is above a threshold (suggested: 120).
- Otherwise fall back to unified git-like output.

Allow user override with a toggle key `t` while the Diff modal is open.

### Rendering strategy (single viewport)

To keep scrolling simple, build a single viewport content string where each row is already composed as:

- `leftColumn` padded/truncated to `colW`
- a separator (` │ `)
- `rightColumn` padded/truncated to `colW`

This avoids managing two independent viewports.

### Data model

Instead of rendering only `+`/`-` lines, compute a row model keyed by path:

- `path`
- `oldText` (empty for additions)
- `newText` (empty for deletions)
- `kind` (added/removed/changed)

Then render:

- left: `- <path>: <oldText>` for removed/changed
- right: `+ <path>: <newText>` for added/changed

Apply the same colors as unified:

- `-` red, `+` green, headers faint.

### Notes

- Truncate long values per cell to avoid wrapping that would break alignment.
- Keep the existing unified output as the fallback for narrow terminals and as a baseline for copy/paste.

### Keybinding

- `t`: toggle view mode (Side-by-side vs Unified)

## Integration points (current code)

### Where version changes happen

- Version picker (`v`) flow:
  - Versions selection happens in [`AppModel.depEditUpdate()`](internal/tui/model.go:4242) and in [`AppModel.depDetailUpdate()`](internal/tui/model.go:4379).
  - Both flows use [`depVersionValidatedMsg`](internal/tui/model.go:4804) after [`validateDependencyVersionCmd()`](internal/tui/model.go:4679).
  - Today, the root update handles `depVersionValidatedMsg` by calling [`applyDependencyAndApplyInstanceCmd()`](internal/tui/model.go:4942).

- Upgrade-to-latest (`u`) flow:
  - Triggered by [`AppModel.upgradeSelectedDepCmd()`](internal/tui/model.go:4874) which runs [`upgradeDepToLatestCmd()`](internal/tui/model.go:4887).
  - Today that command computes the best stable version and immediately calls [`applyDependencyAndApplyInstanceCmd()`](internal/tui/model.go:4942).

### Where chart artifacts are fetched today

Dependency detail previews in [`AppModel.loadDepDetailPreviewsCmd()`](internal/tui/model.go:1686) already support a multi-tier fetch strategy:

1. Instance vendored chart dir: `charts/<dep.Name>/values.yaml`, `README.md`, `values.schema.json`
2. Cached `.tgz` extraction via [`helmutil.ReadChartArchiveFilesWithSchema()`](internal/helmutil/chart_archive.go:22)
3. helmdex show cache via [`helmutil.ReadShowCache()`](internal/helmutil/showcache.go:42)
4. Pull chart archive and extract
5. Last resort `helm show` (values/readme only; schema not available)

The CLI has a similar, more modular implementation in [`loadDepInspectContent()`](internal/cli/instance_dep_inspect.go:64) which is a good template for a reusable loader.

## Proposed design

### 1) Add a dedicated Diff modal state to the TUI root model

In [`internal/tui/model.go`](internal/tui/model.go:1), add fields (names indicative):

- `depDiffOpen bool`
- `depDiffTab int` (0 schema, 1 values)
- `depDiffLoading bool`
- `depDiffViewport viewport.Model`
- `depDiffOld yamlchart.Dependency`
- `depDiffNew yamlchart.Dependency`
- `depDiffSchemaText string` (rendered diff output)
- `depDiffValuesText string` (rendered diff output)
- `depDiffSummary string` (counts line)
- `depDiffErr string` (modal-local error, separate from `modalErr` if desired)

Add a renderer in [`internal/tui/ui.go`](internal/tui/ui.go:205):

- `renderDepDiffModal(m AppModel) string`

and update [`AppModel.View()`](internal/tui/model.go:2036) composition to render this modal with the highest priority (similar to `depDetailOpen`, `depEditOpen`).

Add input routing in [`AppModel.updateInner()`](internal/tui/model.go:2041) so `depDiffOpen` intercepts keys before other modals.

### 2) Unify all version changes behind a single “compute diff then confirm” step

Introduce a message:

- `type depDiffReadyMsg struct { oldDep, newDep yamlchart.Dependency; schemaOld, schemaNew string; valuesOld, valuesNew string; err error }`

and a command builder:

- `func (m AppModel) loadDepDiffCmd(oldDep, newDep yamlchart.Dependency) tea.Cmd`

This command:

1. Loads schema + values for `oldDep` and `newDep` using the loader (see section 3).
2. Computes schema diff when both schemas exist; otherwise compute values diff.
3. Produces rendered diff content strings + counts.

Then, in the root update:

- On `depVersionValidatedMsg`: **open diff modal** instead of immediately applying.
- On `upgradeDepToLatestCmd` completion: emit a new message such as `depUpgradeCandidateMsg{oldDep, newDep}` and open diff modal.

Finally, in `depDiffUpdate`:

- `y`: close diff modal; call [`applyDependencyAndApplyInstanceCmd()`](internal/tui/model.go:4942) for `newDep`.
- `n`/`esc`: close diff modal; do nothing else.

### 3) Create a reusable chart artifact loader (schema + values) for a dep+version

We need to load, for both old and new:

- schema: `values.schema.json`
- default values: `values.yaml`

Proposed helper (placement options):

- `internal/helmutil` (most reusable), or
- `internal/tui` (UI-focused but still generic), or
- new `internal/chartutil` (clean separation)

Suggested API:

```go
type ChartArtifacts struct {
  Schema string
  Values string
}

func LoadChartArtifacts(ctx context.Context, repoRoot, instancePath string, dep yamlchart.Dependency, version string, allowVendored bool) (ChartArtifacts, error)
```

Loader strategy (mirrors [`loadDepInspectContent()`](internal/cli/instance_dep_inspect.go:64)):

1. Vendored (only when `allowVendored==true`):
   - `charts/<dep.Name>/values.schema.json`
   - `charts/<dep.Name>/values.yaml`
2. Cached `.tgz` extraction (`FindCachedChartArchive` + `ReadChartArchiveFilesWithSchema`):
3. helmdex show cache (schema and values)
4. Pull `.tgz` and extract (again recognize both schema names)
5. Last resort `helm show values` for values only

Note: earlier mention of `values.json.schema` was a typo; only `values.schema.json` is in scope.

Note: schema is not available from `helm show` so missing schema is expected for some charts.

### 4) Diff algorithms

#### 4.1 Schema diff (JSON)

We can treat the schema file as a generic JSON tree and compute a **path-based structural diff**.

Algorithm:

- Parse both schemas with `json.Decoder` + `UseNumber()`.
- Recursively compare:
  - objects: key sets added/removed/changed (sorted keys)
  - arrays: compare by index and length
  - scalars: compare normalized string forms
- Emit changes as stable, line-based output:
  - `+ <path>: <value>` added
  - `- <path>: <value>` removed
  - `~ <path>: <old> -> <new>` changed

Paths:

- Use JSON-pointer-like paths (e.g. `#/properties/image/tag/default`).
- Keep them stable and sortable.

Counts:

- Track totals for `added/removed/changed`.

#### 4.2 Values diff (YAML)

Fallback when schema missing on either side.

Algorithm:

- Parse `values.yaml` into `any` via `yaml.Unmarshal`.
- Flatten into `map[path]canonicalValue` where path uses TUI’s `$` syntax already present in [`internal/values/yamlpath.go`](internal/values/yamlpath.go:22):
  - map keys: `$.a.b`
  - array indices: `$.a[0].b`
- Compare maps to derive added/removed/changed.
- Canonicalize leaf values deterministically:
  - scalars: straightforward
  - maps/arrays: stable string with sorted keys (avoid Go map iteration nondeterminism)

Output:

- Similar `+/-/~` lines with stable ordering by path.
- Truncate very large rendered values to keep the modal responsive.

### 5) Diff modal rendering and truncation

Implement `renderDepDiffModal` similarly to [`renderDepDetailModal()`](internal/tui/ui.go:205):

- Header: dependency label and version change
- Tabs line (only when both tabs present)
- Body: viewport rendering of current tab text
- Footer: key hints (`←/→ tabs • y apply • n or esc cancel`)

Truncation rules:

- Cap diff output by lines (ex: 5000) and/or bytes (ex: 200k) with an explicit `… truncated …` marker.

### 6) Tests

Add tests for:

- Schema JSON diff: stable ordering and basic added/removed/changed detection.
- Values YAML diff: flattening + canonicalization stability.
- TUI behavior: open diff modal from `v` and from `u`, confirm applies, cancel does not apply.

TUI tests can follow patterns in [`internal/tui/dep_detail_test.go`](internal/tui/dep_detail_test.go:1).

## Acceptance criteria

- Pressing `v` or `u` on a dependency does **not** change anything until the Diff modal is confirmed.
- Diff modal shows a schema diff when both versions have schema; otherwise shows values diff.
- Confirm (`y`) applies the version change and runs the existing apply pipeline.
- Cancel (`n`/`esc`) returns to previous UI without changing the instance.
- Works for non-OCI and OCI (OCI: `u` still disallowed, but `v` + diff works).
