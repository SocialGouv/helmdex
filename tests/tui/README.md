# TUI E2E tests (agent-tui)

End-to-end tests that drive the `helmdex` Bubble Tea TUI in a real PTY via
**agent-tui**. See [`tests/README.md`](../README.md) for how this layer fits
with the others, and for the fake Helm binary these specs rely on.

## Running them

```bash
devbox shell
task test-tui
```

`task test-tui` installs JS deps, builds `./bin/helmdex` and the fake Helm,
starts the agent-tui daemon, runs the specs and stops the daemon.

Other targets: `task tui:daemon` / `task tui:stop` (idempotent daemon
control), `task tui:run` (launch the app under the harness).

## Stability

The TUI runs at a fixed terminal size with visual noise disabled:
`HELMDEX_NO_TITLE=1`, `HELMDEX_NO_ICONS=1`, `HELMDEX_NO_LOGO=1`, `NO_COLOR=1`.

Two ways to make Helm-dependent flows deterministic:

- **`realHelmEnv()`** (`tests/shared/fakehelm.ts`) — the real code paths run
  against the fake registry. Use it whenever the assertion is about what the
  flow *did*.
- **`HELMDEX_E2E_STUB_HELM=1` / `_STUB_ARTIFACTHUB=1` / `_NO_EDITOR=1`** —
  in-app bypasses that replace results with placeholders. They get you to a
  screen quickly; they cannot tell you the screen is right.

`realHelmEnv()` turns the stubs off explicitly: specs share a process, and a
sibling spec that set them would otherwise silently put yours back on
placeholders.

## What is covered

- **Shell**: launch, help overlay, command palette, sources modal, navigation.
- **State on disk** (`disk-state.spec.ts`): create, rename, regen, confirmed
  delete and dependency drafting, each asserted against the repo.
- **Catalog** (`catalog-and-sets.spec.ts`): a catalog entry lands pinned,
  attributed to its source, with its default sets materialised, and really
  relocked through Helm.
- **Direct mode** (`direct-mode.spec.ts`): on a helmdex-agnostic repo,
  browsing and regenerating leave every file byte-identical.
- **Dependency detail / diff / apply** (`tier2-*.spec.ts`): the modals,
  including the cancellable apply overlay driven against a Helm call held
  open by the test.
- **Values**: preview modal, regen status, edit gating and its error surface.

## Artifacts

Failures write the screen to `tests/tui/artifacts/` (gitignored); CI uploads
them.

CI workflow: [`.github/workflows/tui-e2e.yml`](../../.github/workflows/tui-e2e.yml)
