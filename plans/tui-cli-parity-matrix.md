# TUI ↔ CLI parity matrix (helmdex)

This document inventories **user-visible capabilities** in the TUI and maps them to **existing** (or **missing**) CLI commands.

Repo anchor points:

- CLI root: [`internal/cli/NewRootCmd()`](internal/cli/root.go:25)
- TUI app: [`internal/tui/app.go`](internal/tui/app.go:1)

## Parity table

Legend:

- ✅ implemented
- ⚠️ partial / semantics differ
- ❌ missing

| Capability | TUI entrypoint | CLI command | Status | Notes |
|---|---|---|---|---|
| Init repo config | Dashboard actions | `helmdex init` | ✅ | Creates `helmdex.yaml` |
| Open UI | `helmdex` (TTY) | `helmdex` | ✅ | TTY gating in [`internal/cli/root.go`](internal/cli/root.go:32) |
| Catalog sync | Palette: Catalog sync | `helmdex catalog sync` | ✅ | Cache in `.helmdex/catalog` + `.helmdex/cache` |
| Catalog list/get | Wizard pick list | `helmdex catalog list/get` | ✅ | JSON/table formats |
| Artifact Hub search/versions | Add dep wizard | `helmdex artifacthub search/versions` | ✅ | JSON/table formats |
| Instances list/create/remove | Dashboard | `helmdex instance list/create/rm` | ✅ | `rm` requires `--yes` |
| Apply instance pipeline | Save/apply flows | `helmdex instance apply` | ✅ | Relock + preset import + values regen |
| Regenerate merged values.yaml | Palette: Regenerate values | `helmdex instance values regen` | ✅ | |
| Add dep (arbitrary/OCI) | Add dep wizard | `helmdex instance dep add` | ⚠️ | CLI currently creates `values.set.<name>.yaml` markers, but TUI uses per-dep markers (`values.dep-set.<id>--<set>.yaml`) for catalog presets. Also CLI does not persist dep source metadata (see below). |
| Add dep from catalog | Add dep wizard: Predefined catalog | `helmdex instance dep add-from-catalog` | ⚠️ | CLI upserts `Chart.yaml` + writes per-dep set marker files, but **does not write dep source meta** (`kind: catalog`, `catalogID`, `catalogSource`). TUI writes depmeta at apply time in [`internal/tui/model.go`](internal/tui/model.go:3410). |
| Remove dep | Actions | `helmdex instance dep rm` | ⚠️ | CLI does not delete depmeta for the dep (TUI has helper [`deleteDepMetaFile()`](internal/tui/depmeta.go:90)). |
| List deps | Deps tab | `helmdex instance dep list` | ⚠️ | TUI shows source tags (CAT/AH/ARB). CLI prints basic fields only. Optional improvement: `--format json` include depmeta. |
| Set dep version | Version picker | `helmdex instance dep set-version` | ✅ | Non-OCI validation option |
| Upgrade dep | Upgrade action | `helmdex instance dep upgrade` | ✅ | Non-OCI only |
| List dep versions | Versions list | `helmdex instance dep versions` | ✅ | Non-OCI only |
| Inspect README/values/schema | Dep detail | `helmdex instance dep inspect ...` | ✅ | Cached helm show/pull |
| Per-dep values get/set/unset | Configure tab | `helmdex instance dep values ...` | ✅ | JSONPath editing |
| Preset resolution inspect | Sets tab | `helmdex instance presets resolve*` | ✅ | Non-interactive inspection |
| Sync presets for selected dep | Palette: Sync presets selected dep | (proposed) `helmdex instance dep sync-presets <instance> <depID>` | ❌ | TUI implementation: [`syncSelectedDepPresetsCmd()`](internal/tui/model.go:1470) |
| Detach dep from catalog | Palette: Detach from catalog | (proposed) `helmdex instance dep detach <instance> <depID>` | ❌ | TUI implementation: [`detachDepFromCatalogCmd()`](internal/tui/model.go:1080) |

## Key shared state: dep source metadata (depmeta)

The TUI persists dependency source state to repo-local files:

- Path format: `.helmdex/depmeta/<instanceName>/<depID>.yaml`
- Schema: [`depSourceMeta`](internal/tui/depmeta.go:23)

This enables catalog-attached behaviors (sets tab, version restrictions, detach), as described in [`plans/catalog-deps-supported-versions-and-detach.md`](plans/catalog-deps-supported-versions-and-detach.md:1).

CLI currently does not read/write this metadata, which is the root cause of several parity gaps.

