# Bundled Helm (v4.1.1) auto-download design

Goal: make `helmdex` self-contained and reproducible by automatically installing and using a pinned Helm binary.

## Desired behavior

- Default: always run Helm via a pinned binary (Helm `v4.1.1`) located under the user HOME in `.helmdex/bin/`.
- If the binary is missing **or** is not the pinned version: download + verify + replace it.
- Opt-out: if `HELMDEX_NO_BUNDLED_HELM=1`, do **not** download; require `helm` to exist on `PATH`.

## Download sources

Use `get.helm.sh` artifacts:

- Archive: `helm-v4.1.1-${GOOS}-${GOARCH}.${ext}`
- Checksums: `helm-v4.1.1-checksums.txt`

Notes:

- `linux`/`darwin` are typically `.tar.gz`
- `windows` is typically `.zip` and the binary is `helm.exe`

## Verification

1. Download `helm-v4.1.1-checksums.txt`
2. Parse the expected SHA-256 for the platform archive name
3. Download the archive
4. Hash the downloaded archive and compare to expected SHA-256
5. Extract the `helm` (or `helm.exe`) file from the archive
6. Install atomically into the target location and set executable bit (non-windows)
7. Optional hardening: run `helm version --short` to confirm it reports `v4.1.1`.

## Concurrency + atomic install

- Use a lock file under the same directory as the destination binary.
- Install to a temp file in the same filesystem, then `rename` into place.

## Integration points

All Helm subprocesses currently execute `helm` by name via:

- [`helmutil.run()`](../internal/helmutil/show.go:322)
- [`helmutil.runInteractive()`](../internal/helmutil/show.go:347)
- [`helmutil.runQuiet()`](../internal/helmutil/deps.go:29)

Plan: resolve an absolute helm path once (with caching) and substitute it whenever a subprocess is about to run `helm`.

