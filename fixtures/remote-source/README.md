# remote-source (fixture)

This directory is a **checked-in fixture** used by the hermetic e2e test.

Tests copy these files into a temporary directory, run `git init`, and commit
them. That temp git repo path is then used as `sources[].git.url` in
`helmdex.yaml`, so [`helmdex catalog sync`](../../internal/cli/catalog.go:26)
exercises the full “remote source” flow while remaining offline.

## Layout expected by helmdex

### Catalog

`catalog.yaml` is copied into `.helmdex/catalog/<source>.yaml` by
[`(*Syncer).Sync()`](../../internal/catalog/sync.go:28).

### Presets

Preset files live under:

`charts/<chartName>/<chartVersion>/`

and are discovered/resolved by [`Resolve()`](../../internal/presets/resolve.go:29).

Supported layers in this fixture:

- `values.default.yaml`
- `values.platform.<platform>.yaml` (platform is set by `platform.name` in `helmdex.yaml`)
- `values.set.<set>.yaml` (set selection is by local file presence, see [`Import()`](../../internal/presets/import.go:23))

## Included example entries

- `bitnami/postgresql` pinned to `15.5.0`
- `bitnami/nginx` pinned to `15.0.0`

Each entry defines selectable sets:

- `dev`
- `ha-production`

