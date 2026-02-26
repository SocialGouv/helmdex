# TUI E2E tests (agent-tui)

This directory contains end-to-end tests that drive the `helmdex` Bubble Tea TUI via **agent-tui**.

## Prereqs (local)

This repo uses **devbox** to provide a consistent Node + pnpm toolchain.

```bash
devbox shell
corepack pnpm install
corepack pnpm run build:helmdex
corepack pnpm test:tui
```

## Commands

- `corepack pnpm test:tui` — run all interactive TUI tests
- `corepack pnpm build:helmdex` — build `./bin/helmdex` used by the harness
- `corepack pnpm tui:daemon` — start agent-tui daemon (idempotent)
- `corepack pnpm tui:stop` — stop agent-tui daemon (idempotent)
- `corepack pnpm tui:run` — launch helmdex under test (used internally by tests)

## Stability knobs

Tests run the TUI with a fixed terminal size and with visual noise disabled:

- `HELMDEX_NO_TITLE=1`
- `HELMDEX_NO_ICONS=1`
- `HELMDEX_NO_LOGO=1`
- `NO_COLOR=1`

## E2E stubs (CI/determinism)

Some UI surfaces depend on external tools/services (Helm, Artifact Hub, editor). For stable E2E coverage, helmdex supports **env-gated stubs** used by the agent-tui harness.

- `HELMDEX_E2E_STUB_HELM=1`
  - Bypasses Helm-dependent calls (versions lookup, `helm show`, relock) with deterministic placeholders.
  - Enables opening higher-risk modals deterministically (upgrade diff, apply overlay, dep detail previews).
- `HELMDEX_E2E_STUB_ARTIFACTHUB=1`
  - Stubs Artifact Hub search + versions without network.
- `HELMDEX_E2E_NO_EDITOR=1`
  - Skips spawning `$EDITOR` and returns success (so post-edit regen runs).

Default behavior is unchanged; these only activate when explicitly set.

## Scenarios covered

Existing:
- Smoke launch (dashboard visible)
- Help overlay toggle
- Create + rename instance

Added:
- Command palette open/close
- Sources modal open/close
- Confirm delete cancel flow
- Values preview modal open/close
- Regen values OK status
- Edit-values gating error + dismiss
- Add-dep wizard catalog-empty recovery (open sources) + back navigation
- Add-dep wizard arbitrary dep draft
- Version editor open/close (OCI/manual)
- Tier 2 (stubs): dep detail modal open + tab switch + delete confirm cancel
- Tier 2 (stubs): upgrade diff modal appears + cancel
- Tier 2 (stubs): apply overlay appears + cancel-confirm flow

Assertions prefer structured screenshots when available; otherwise they normalize text output (strip ANSI, normalize whitespace).

## CI

GitHub Actions workflow: [`.github/workflows/tui.yml`](.github/workflows/tui.yml:1)
