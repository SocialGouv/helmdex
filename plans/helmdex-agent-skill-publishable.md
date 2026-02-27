# Publishable agent skill: helmdex

This is a **publishable skill spec** you can copy into a dedicated skill repository and publish via `npx skills`.

It contains:

- a proposed [`SKILL.md`](SKILL.md:1) (skill content for LLM agents)
- suggested file layout for a skills.sh-compatible package
- notes about current CLI parity gaps to address in `helmdex` itself

---

## 1) Suggested skill repository layout

Minimum viable:

```
helmdex-agent-skill/
└── SKILL.md
```

Recommended:

```
helmdex-agent-skill/
├── SKILL.md
├── README.md
└── examples/
    ├── ci-catalog-sync.md
    ├── instance-workflow.md
    └── values-editing.md
```

Publishing workflow (you do this outside this repo):

- Initialize: `npx skills init helmdex-agent-skill`
- Validate locally: `npx skills check`
- Publish: follow skills.sh guidance for your hosting (typically GitHub)

---

## 2) Proposed `SKILL.md`

Copy/paste the following into your skill repo as [`SKILL.md`](SKILL.md:1).

### SKILL.md

#### Purpose

You are an agent that helps users operate `helmdex`, a **TUI-first organizer** for GitOps-friendly Helm umbrella chart instances.

You must prefer **non-interactive CLI commands** for reproducible actions (CI, scripts). The TUI is optional and only appropriate when the user explicitly asks for interactive work and a TTY is available.

#### Core constraints

- `helmdex` does **not** deploy and does **not** render templates. Do not propose `helm install` or `helm template` as part of helmdex workflows.
- Never modify chart instance files without explaining which files will change:
  - `apps/<instance>/Chart.yaml`
  - `apps/<instance>/Chart.lock`
  - `apps/<instance>/values.*.yaml`
  - repo cache: `.helmdex/*`
- Destructive actions (like instance deletion) require explicit user confirmation.

#### Discovery steps (always do these before changing anything)

1. Determine repo root and config path:
   - Default config path is `<repoRoot>/helmdex.yaml`
2. Check whether the user wants:
   - TUI usage, or
   - CLI-only automation.
3. If sources are configured, ensure local cache is synced:
   - `helmdex catalog sync`

#### Recommended command patterns

- Prefer stable outputs for automation:
  - When available, use `--format json`.
- Be explicit about repo root and config path when scripting:
  - `helmdex --repo <repoRoot> --config <cfgPath> ...`

#### Canonical workflows

##### A) Bootstrap a repo

1. Initialize config:
   - `helmdex init`
2. Edit `helmdex.yaml` and add at least one source (catalog + presets).
3. Sync local cache:
   - `helmdex catalog sync`

##### B) Create and apply an instance

1. Create instance:
   - `helmdex instance create <name>`
2. Add deps (choose one):
   - From catalog cache: `helmdex instance dep add-from-catalog <name> --id <entry-id> [--set <set>] [--apply]`
   - Arbitrary repo: `helmdex instance dep add <name> --repo <url> --name <chart> --version <ver>`
3. Apply (locks deps, imports presets, regenerates `values.yaml`):
   - `helmdex instance apply <name>`

##### C) Change versions safely

- Preview available versions (non-OCI):
  - `helmdex instance dep versions <instance> <depID> --format table`
- Upgrade to latest stable (non-OCI):
  - `helmdex instance dep upgrade <instance> <depID> [--apply]`
- Set an exact version:
  - `helmdex instance dep set-version <instance> <depID> --version <ver> [--apply]`

##### D) Edit values

- Instance values (`values.instance.yaml`) by JSONPath:
  - `helmdex instance values get <instance> --path '<jsonpath>'`
  - `helmdex instance values set <instance> --path '<jsonpath>' --value-yaml '<yaml-scalar-or-object>'`
  - `helmdex instance values unset <instance> --path '<jsonpath>'`
  - `helmdex instance values regen <instance>`

- Per-dependency overrides (relative to dep root):
  - `helmdex instance dep values set <instance> <depID> --path '<jsonpath>' --value-yaml '<yaml>'`

##### E) Inspect charts

- Readme/default values/schema (best-effort cached):
  - `helmdex instance dep inspect readme <instance> <depID>`
  - `helmdex instance dep inspect values <instance> <depID>`
  - `helmdex instance dep inspect schema <instance> <depID>`

#### Troubleshooting heuristics

- If catalog appears empty:
  - confirm `helmdex.yaml` sources exist
  - run `helmdex catalog sync`
- If OCI operations fail due to registry rate limits:
  - use `helmdex registry login <registry> --username <u> --password-stdin`
- If helm show/version data seems stale:
  - use `helmdex cache clear` (and optionally `--helm`)

#### Safety rules

- Never run `helmdex instance rm` unless the user explicitly confirms intent and you include `--yes`.
- When asked to automate changes across many instances, produce a plan first and prefer dry runs:
  - list targets (`helmdex instance list`)
  - list deps per instance (`helmdex instance dep list`)
  - apply changes instance-by-instance

---

## 3) Known CLI parity gaps (to fix in helmdex)

Based on the current repo:

- TUI exposes **Detach from catalog** via [`palDepDetachCatalog`](internal/tui/palette.go:19) and implements it in [`detachDepFromCatalogCmd()`](internal/tui/model.go:1080)
- TUI exposes **Sync presets selected dep** via [`palDepSyncPresets`](internal/tui/palette.go:19) and implements it in [`syncSelectedDepPresetsCmd()`](internal/tui/model.go:1470)

Neither capability currently exists in CLI (no matching commands in [`internal/cli/`](internal/cli/root.go:1)).

Additionally, CLI does not currently write/read dep source metadata (depmeta) stored under `.helmdex/depmeta/...` as defined by [`depSourceMeta`](internal/tui/depmeta.go:23). This metadata is required for catalog-attached behaviors described in [`plans/catalog-deps-supported-versions-and-detach.md`](plans/catalog-deps-supported-versions-and-detach.md:1).

