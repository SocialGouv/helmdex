package instances

import (
	"context"
	"path/filepath"

	"helmdex/internal/config"
	"helmdex/internal/presets"
	"helmdex/internal/values"
	"helmdex/internal/yamlchart"
)

// Apply reconciles an instance: relocks dependencies (forced, or when
// Chart.yaml drifted from Chart.lock), imports presets and regenerates the
// merged values.yaml. The last two are managed-mode only; direct-mode
// instances (helmdex-agnostic repos) get a pure dependency reconcile.
func Apply(ctx context.Context, repoRoot string, cfg config.Config, inst Instance, forceRelock bool) error {
	if forceRelock {
		if err := RelockDependencies(ctx, repoRoot, inst.Path); err != nil {
			return err
		}
	} else {
		if _, err := RelockIfDepsChanged(ctx, repoRoot, inst.Path); err != nil {
			return err
		}
	}

	c, err := yamlchart.ReadChart(filepath.Join(inst.Path, "Chart.yaml"))
	if err != nil {
		return err
	}
	if _, err := presets.Import(presets.ImportParams{RepoRoot: repoRoot, InstancePath: inst.Path, Config: cfg, Dependencies: c.Dependencies}); err != nil {
		return err
	}
	return values.GenerateIfManaged(inst.Path)
}
