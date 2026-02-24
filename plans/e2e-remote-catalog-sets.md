# Hermetic E2E: remote catalog + sets

This repository includes a hermetic (CI-safe) end-to-end test that demonstrates helmdex’s “remote catalog” + “values sets” workflow without requiring network access or a `helm` binary.

The test is implemented in [`internal/cli/e2e_remote_catalog_sets_test.go`](../internal/cli/e2e_remote_catalog_sets_test.go:1).

## What it covers

1. A **local git repo** acts as the “remote” source containing:
   - `catalog.yaml` (catalog entries)
   - preset files under `charts/<chart>/<version>/`:
     - `values.default.yaml`
     - `values.platform.<platform>.yaml`
     - `values.set.<set>.yaml`

2. `helmdex catalog sync` clones that repo into `.helmdex/cache/` and copies the `catalog.yaml` into `.helmdex/catalog/`.

3. `helmdex instance dep add-from-catalog` adds the dependency from the local catalog cache **and materializes the entry’s `defaultSets`** by creating empty `values.set.<set>.yaml` files.

4. `helmdex instance apply` imports default/platform/selected set layers into the instance and generates a merged `values.yaml`.

## Why it doesn’t need Helm

`helmdex instance apply` normally re-locks dependencies when `Chart.yaml` and `Chart.lock` are out of sync.

The e2e test pre-writes a matching `Chart.lock` (same `name`, `version`, `repository` tuples) so dependency re-locking is skipped, keeping the flow hermetic.

## How to run

The test runs as part of the default suite:

```bash
go test ./...
```

