# helmdex

`helmdex` is a TUI-first organizer for Helm umbrella chart instances.

## TUI

Launch the interactive dashboard:

```bash
helmdex tui
```

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
