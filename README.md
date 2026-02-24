# helmdex

`helmdex` is a TUI-first organizer for Helm umbrella chart instances.

## TUI

Launch the interactive dashboard:

```bash
helmdex tui
```

The TUI updates the terminal window title to show the current breadcrumb:

`🧭 HelmDex — Dashboard › Instance › <name> › <task>`

Opt-out by setting `HELMDEX_NO_TITLE=1`.

### Catalog + sets (presets)

helmdex supports a **remote catalog** (curated chart/version targets) and downloadable **sets** (YAML values files) that can be selected when adding a dependency.

- Catalog entries are synced into `.helmdex/catalog/` by running [`helmdex catalog sync`](internal/cli/catalog.go:26)
- Preset layers (default/platform/sets) are synced into `.helmdex/cache/` and resolved by [`presets.Resolve()`](internal/presets/resolve.go:33)

#### Pin a catalog source to a specific git ref

Each configured source supports `git.ref` to target a specific branch/tag/commit-ish during sync.

```yaml
sources:
  - name: example
    git:
      url: https://github.com/acme/helmdex-catalog.git
      ref: v1.2.3
    presets:
      enabled: true
    catalog:
      enabled: true
```

#### Try the built-in example catalog in the TUI (local, no network)

This repo includes an example “remote source repo” fixture at [`fixtures/remote-source/`](fixtures/remote-source/README.md:1).

You can use it in two ways:

1) **Filesystem source (no git):** point `sources[].git.url` directly at `fixtures/remote-source`.
2) **Local git repo (matches real-world sync behavior):** copy it to `/tmp`, `git init`, commit, and point `sources[].git.url` at that `/tmp` path.

1) (Option A) Use it directly as a filesystem source (no git required):

```yaml
apiVersion: helmdex.io/v1alpha1
kind: HelmdexConfig
repo:
  appsDir: apps
platform:
  name: eks
sources:
  - name: example
    git:
      url: fixtures/remote-source
    presets:
      enabled: true
      chartsPath: charts
    catalog:
      enabled: true
      path: catalog.yaml
```

2) (Option B) Create a local git repo from the fixture:

```bash
rm -rf /tmp/helmdex-example-remote-source
cp -a fixtures/remote-source /tmp/helmdex-example-remote-source
cd /tmp/helmdex-example-remote-source

git init
git config user.email e2e@example.invalid
git config user.name helmdex-example

git add -A
git commit -m 'example catalog + presets'
```

2) In your helmdex repo (the one containing `apps/<instance>/`), add this source to `helmdex.yaml`:

```yaml
apiVersion: helmdex.io/v1alpha1
kind: HelmdexConfig
repo:
  appsDir: apps
platform:
  name: eks
sources:
  - name: example
    git:
      url: /tmp/helmdex-example-remote-source
    presets:
      enabled: true
      chartsPath: charts
    catalog:
      enabled: true
      path: catalog.yaml
```

3) Sync + launch:

```bash
helmdex catalog sync
helmdex tui
```

4) In the TUI:

- open an instance → **Dependencies** tab
- press `a` → choose **Predefined catalog**
- select an entry → `enter`
- in the detail step:
  - `space` toggles a set
  - `d` toggles all default sets
  - `enter` adds + applies

After a dependency has been added, you can:

- press `enter` on a dependency to open its detail modal
  - use the **Sets** tab to toggle per-dependency preset sets and press `s` to save+apply
- press `x` on a dependency to open the dependency actions menu
  - choose **Sync presets** to fetch the latest preset cache (git fetch/checkout using `git.ref`), remove orphan set markers, re-import presets, and regenerate merged values

If you see “No local catalog entries”, it means you haven’t run sync yet (TUI reads from `.helmdex/catalog/*.yaml` via [`LoadLocalCatalogEntries()`](internal/catalog/load.go:14)).

## Non-interactive CLI parity (scriptable)

helmdex is TUI-first, but all current TUI capabilities are also available via non-interactive CLI commands suitable for CI/scripts.

### Instance values (no $EDITOR)

Read / write `values.instance.yaml` by path (TUI syntax):

```bash
# Set a scalar
helmdex instance values set my-app --path '$.global.replicas' --value-yaml '3'

# Read it back (default JSON)
helmdex instance values get my-app --path '$.global.replicas'

# Unset it
helmdex instance values unset my-app --path '$.global.replicas'

# Replace the whole file (from stdin)
cat values.instance.yaml | helmdex instance values replace my-app --stdin

# Regenerate merged values.yaml
helmdex instance values regen my-app
```

### Dependency overrides (Configure tab parity)

The TUI “Configure” tab edits overrides under the dependency id key (`alias` if set else `name`) inside `values.instance.yaml`. The CLI provides the same operations:

```bash
# Set per-dependency override (relative to the dependency root)
helmdex instance dep values set my-app nginx --path '$.replicaCount' --value-yaml '2'

# Get it
helmdex instance dep values get my-app nginx --path '$.replicaCount'

# Unset it
helmdex instance dep values unset my-app nginx --path '$.replicaCount'
```

### Dependencies (add/version/upgrade)

```bash
# Add/update a dependency directly
helmdex instance dep add my-app --repo https://charts.bitnami.com/bitnami --name nginx --version 15.0.0

# Add from the local catalog cache
helmdex catalog sync
helmdex instance dep add-from-catalog my-app --id my-catalog-entry

# Set an exact version (optionally validate) and apply
helmdex instance dep set-version my-app nginx --version 15.1.0 --apply

# Upgrade to latest stable SemVer (non-OCI)
helmdex instance dep upgrade my-app nginx --apply
```

### Dependency inspection (README / values / schema)

These commands use the same best-effort + caching strategy as the TUI (vendored chart dir → chart archive cache → helmdex show cache → pull → helm show).

```bash
helmdex instance dep inspect readme my-app nginx
helmdex instance dep inspect values my-app nginx
helmdex instance dep inspect schema my-app nginx
```

### Presets resolution (read-only)

```bash
# All deps
helmdex instance presets resolve my-app

# One dep
helmdex instance presets resolve-dep my-app nginx
```

### Artifact Hub helpers (non-interactive)

```bash
helmdex artifacthub search nginx
helmdex artifacthub versions bitnami nginx
```

### Values tab

In an instance view, the **Values** tab lists the values-related files that exist in the instance directory, with a short description next to each:

- `values.default.yaml` — baseline defaults
- `values.platform.yaml` — platform overrides
- `values.set.<name>.yaml` — preset layer `<name>` (sorted)
- `values.dep-set.<depID>--<set>.yaml` — selected set file for one dependency (downloaded on apply)
- `values.instance.yaml` — user overrides (**editable**)
- `values.yaml` — merged output (**generated**)

Select a file to open a preview.

## YAML syntax highlighting

YAML previews are syntax-highlighted in the TUI (instance values preview, Artifact Hub “Values”, dependency detail “Default”).

## Markdown README rendering

README previews in the TUI are rendered as Markdown (to ANSI) when shown in:

- Artifact Hub detail “README”
- Dependency detail “README”

Color output is **automatically disabled** when:

- `NO_COLOR` is set (any value)
- `TERM=dumb`
