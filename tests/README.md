# Tests

helmdex has four faces — CLI, TUI, HTTP API and web UI (which the desktop app
re-wraps) — and one rule: a test that only proves a screen rendered proves
nothing about the repo. Mutating scenarios end on assertions about files on
disk.

Everything below runs **hermetically**: no network, no chart registry, no Helm
installation, no cluster.

## The layers

| Layer | Where | Runs with |
|---|---|---|
| Go unit tests | `internal/*/` | `task test` |
| HTTP API | `internal/server/*_test.go` | `task test:server` |
| CLI end-to-end | `internal/cli/e2e_*_test.go` | `task test` |
| Web UI integration (jsdom) | `webui/src/**/*.test.tsx` | `task ui:test` |
| TUI end-to-end (real PTY) | `tests/tui/specs/` | `task test-tui` |
| Web UI end-to-end (real browser) | `tests/web/specs/` | `task test-web` |

`task test:all` runs the lot. `task` alone lists every target.

## The fake Helm

`internal/testutil/fakehelm` is a stand-in for the `helm` binary: a small,
deterministic chart registry. Putting it first on `PATH` makes every
Helm-dependent code path testable without a network — and, unlike a stub
inside helmdex, it leaves the real code running.

Two ways to reach it, and the difference matters:

- **`HELMDEX_NO_BUNDLED_HELM=1`** — helmdex resolves Helm through `PATH`. The
  real relock/versions/inspect paths run against the fake registry. **Prefer
  this.**
- **`HELMDEX_E2E_STUB_HELM=1`** — same for `helmutil`, but it *also* enables
  the TUI's own bypasses (`internal/tui/e2e_env.go`), which replace
  Helm-dependent results with placeholders. Useful to reach a screen quickly,
  useless to prove the screen is right.

It is deliberately as strict as the real thing on the points helmdex depends
on: a classic chart reference resolves only once its repository has been
registered, `dependency update` prunes the archives it replaces, and an empty
`repo list -o json` is an empty array rather than an error. A fake that says
yes more often than Helm does would leave whole layers — repository
registration, above all — green and untested.

Behaviour knobs (all optional): `HELMDEX_FAKE_HELM_SPEC` (replace the chart
universe), `_LOG` (record every invocation), `_DELAY_MS` / `_DELAY_CMD` (make
async UI states observable), `_BLOCK_UNTIL` (hold a call open until the test
releases it — prefer it over a delay when acting on an in-flight operation),
`_FAIL` (force a failure).

Build it with `task test:fakehelm`; the Go helpers build it on demand.

## Shared helpers

- **Go**: `internal/testutil` — `Hermetic(t)` isolates `PATH`, `HOME`, `XDG_*`,
  the helmdex cache and the user config; `NewRepo(t, opts)` builds a throwaway
  workspace (opted-in with a local git source, or seeded from
  `fixtures/agnostic-gitops`); `FakeHelmCalls` reads back what Helm was asked
  to do.
- **JS**: `tests/shared/` — the same repo shapes (`repoFixture.ts`), filesystem
  assertions (`diskAssertions.ts`) and the fake Helm environment
  (`fakehelm.ts`), shared by the TUI and web harnesses.
- **Web (jsdom)**: `webui/src/test/fakeApi.ts` stubs the API at the `fetch`
  boundary, so `api/client.ts` stays in the test; mutations update the state,
  so a refetch reflects them.

## Conventions

- **No conditional assertions.** A test with an `if` is a test that skips half
  the time. If a flow is racy, synchronise on something real — a recorded Helm
  call, a server event — not on a timer.
- **Every API route is covered.** `internal/server/routes_coverage_test.go`
  parses the routes out of `routes()` and fails the suite if one is never
  exercised.
- **Assert what the user gets and what the disk holds.** In agnostic repos
  that includes proving files were *not* touched.
- **Fixtures are hostile on purpose.** `fixtures/agnostic-gitops` keeps blank
  lines, trailing comments and padded flow collections, because a fixture that
  happens to be a fixed point of the YAML encoder turns "helmdex never
  reformats this" into a test that cannot fail.
- **User-supplied strings that become paths go through `paths.ValidateSegment`.**
  Instance names, dependency ids and source names have each been an
  arbitrary-write primitive; the guard and its table test are the canary.

## Prerequisites

```bash
devbox shell
task deps
```

The browser suite needs Chromium once: `pnpm exec playwright install chromium`
(`task test-web:ci` installs it with its system dependencies).
