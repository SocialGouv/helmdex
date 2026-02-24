# TUI window title updates (🧭 HelmDex — breadcrumb)

## Goal

When running the TUI, update the terminal window title to:

`🧭 HelmDex — <breadcrumb>`

Where `<breadcrumb>` reflects the user’s current navigation/task context.

Opt-out via environment variable:

`HELMDEX_NO_TITLE=1`

## Constraints / notes

- Must emit **plain text** (no ANSI styling) for maximum terminal compatibility.
- Use Bubble Tea’s window title mechanism if available.
- Avoid emitting repeatedly; only update when the computed title changes.

## Where to integrate

- Program entry: [`Run()`](internal/tui/app.go:27) creates the Bubble Tea program.
- Main model lifecycle:
  - [`AppModel.Init()`](internal/tui/model.go:562)
  - [`AppModel.Update()`](internal/tui/model.go:1728)
  - [`AppModel.View()`](internal/tui/model.go:2995) already renders an in-TUI header + breadcrumb.
- Existing visible breadcrumb rendering is in [`renderBreadcrumbBar()`](internal/tui/ui.go:61), but it returns ANSI-styled text with icons; we will **not** reuse it directly.

## Title breadcrumb rules (approved)

Base crumbs:

1) `Dashboard`
2) If on instance screen: `Instance`
3) If an instance is selected and has a name: `<instance name>`

Then append *one* final crumb for the “current task overlay”, with the following priority order:

1) Help overlay: `Help`
2) Command palette: `Commands`
3) Sources modal: `Configure sources`
4) Apply overlay: `Applying`
5) Dep actions modal: `Dependency actions`
6) Dep version editor modal: `Change dependency version`
7) Dep detail modal: `Dependency detail`
8) Values preview modal: `Preview values`
9) Add-dependency wizard (if open): `Add dep` + step label (see below)

Add-dependency wizard step labels (when `addingDep` is true):

- `Choose source`
- `Catalog`
- `Catalog detail`
- `Resolve collision`
- `Artifact Hub search`
- `Artifact Hub results`
- `Artifact Hub versions`
- `Artifact Hub detail`
- `Arbitrary`

Delimit crumbs using a plain separator:

`Dashboard › Instance › my-app › Add dep › Artifact Hub search`

Final title format:

`🧭 HelmDex — Dashboard › Instance › my-app › Add dep › Artifact Hub search`

Non-goals (per approval):

- Do **not** include active tab names.
- Do **not** add length clamping/truncation unless it becomes necessary.

## Implementation design (Code mode)

### 1) Title builder

Add a new helper (new file recommended):

- [`internal/tui/title.go`](internal/tui/title.go:1)

Core functions:

- `func buildTitleCrumbs(m AppModel) []string`
- `func buildWindowTitle(m AppModel) string`

Key points:

- Only use plain strings; avoid calling [`withIcon()`](internal/tui/theme.go:67) or any `lipgloss` rendering.
- Treat instance name as user content; include as-is (trimmed).
- Use the approved overlay priority order above.

### 2) Opt-out env var

In the title emission path, check:

- `os.Getenv("HELMDEX_NO_TITLE") == "1"`

If true, do not emit title commands.

### 3) Emitting the title

Prefer Bubble Tea:

- `tea.SetWindowTitle(title)`

If Bubble Tea lacks that API in this version, fallback to writing OSC (0/2):

- `\x1b]0;{title}\x07`

but only if needed.

### 4) Avoid redundant emits

Add a field to [`AppModel`](internal/tui/model.go:33):

- `lastWindowTitle string`

Then add a helper wrapper:

- `func (m AppModel) withWindowTitle(cmd tea.Cmd) (tea.Model, tea.Cmd)`

Behavior:

- Compute new title.
- If unchanged vs `m.lastWindowTitle`, return unchanged cmd.
- If changed, set `m.lastWindowTitle = newTitle` and return `tea.Batch(cmd, tea.SetWindowTitle(newTitle))`.

### 5) Wiring points

- In [`AppModel.Init()`](internal/tui/model.go:562), batch a first title set (so the title updates immediately on launch).
- In [`AppModel.Update()`](internal/tui/model.go:1728), ensure all returns go through `withWindowTitle(...)`.

Practical approach:

- Replace `return m, cmd` with `return m.withWindowTitle(cmd)` (mechanical change).
- Replace `return m, nil` with `return m.withWindowTitle(nil)`.

This avoids a large refactor while still ensuring the title stays in sync.

## Tests

Add table-driven tests for the pure builder:

- [`internal/tui/title_test.go`](internal/tui/title_test.go:1)

Cases (minimum):

- Dashboard only
- Instance screen with selected instance name
- Add-dep wizard at multiple steps (AH query/results/detail)
- Help overlay wins over wizard
- Apply overlay wins over other overlays

These tests should only validate strings returned by `buildWindowTitle(m)`.

## Documentation

Update [`README.md`](README.md:5) TUI section to mention:

- TUI updates window title to show breadcrumb
- Opt-out via `HELMDEX_NO_TITLE=1`

