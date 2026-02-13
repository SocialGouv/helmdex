# Helm isolation in helmdex

helmdex runs Helm in an isolated environment under `.helmdex/` so it does **not**:

- read or modify the user’s global Helm config/cache (e.g. `~/.config/helm`, `~/.cache/helm`)
- read user Docker/OCI credentials (e.g. `~/.docker/config.json`)

This improves reproducibility and prevents performance issues caused by global Helm state.

## What helmdex isolates

All Helm subprocesses are run with an environment constructed in [`helmutil.helmEnvVars()`](../internal/helmutil/show.go:374) and filtered in [`helmutil.isolatedProcessEnv()`](../internal/helmutil/show.go:402):

- `HELM_CONFIG_HOME`, `HELM_CACHE_HOME`, `HELM_DATA_HOME`
- `HELM_REPOSITORY_CONFIG`, `HELM_REPOSITORY_CACHE`
- `HELM_REGISTRY_CONFIG`
- `HELM_PLUGINS`
- `HOME`, `XDG_CONFIG_HOME`, `XDG_CACHE_HOME`, `XDG_DATA_HOME`
- `DOCKER_CONFIG`

Any inherited `HELM_*`, `XDG_*`, `DOCKER_*`, and `HOME` values are stripped.

## Dependency operations are per-instance

For `helm dependency update/build`, helmdex uses a **per-instance** Helm env (under
`.helmdex/helm/instances/<hash>/...`) via [`helmutil.EnvForInstancePath()`](../internal/helmutil/show.go:55).

Before running dependency commands, helmdex prunes and re-adds repos so the env contains
**only** the repos referenced by the instance’s `Chart.yaml` (classic `https://...` repos).
This is implemented in [`helmutil.PrepareDependencyEnv()`](../internal/helmutil/repos.go:36).

## Repo update behavior (performance)

helmdex avoids global `helm repo update`.

- When updating, it calls `helm repo update <name...>` using [`helmutil.RepoUpdateNames()`](../internal/helmutil/show.go:162).
- For stale-aware updates, it uses [`helmutil.RepoUpdateIfStaleNames()`](../internal/helmutil/show.go:189).

This prevents Helm from touching unrelated repos that may exist in the environment.

## OCI registry authentication

Registry credentials are shared per repo-root and stored at:

- `.helmdex/helm/registry/config.json`

Use the helmdex command:

- `helmdex registry login <registry> [--username ...] [--password-stdin]`

This runs `helm registry login` with the same isolation rules so credentials are stored only
under `.helmdex/`.

