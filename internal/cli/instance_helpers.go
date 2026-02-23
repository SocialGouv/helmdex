package cli

import (
	"path/filepath"

	"helmdex/internal/config"
	"helmdex/internal/instances"
	"helmdex/internal/repo"
	"helmdex/internal/yamlchart"
)

func resolveRepoAndConfig(f *rootFlags) (repoRoot string, cfgPath string, cfg config.Config, err error) {
	repoRoot, err = repo.ResolveRoot(f.RepoRoot)
	if err != nil {
		return "", "", config.Config{}, err
	}
	cfgPath = f.Config
	if cfgPath == "" {
		cfgPath = filepath.Join(repoRoot, "helmdex.yaml")
	}
	cfg, err = config.LoadFile(cfgPath)
	if err != nil {
		return "", "", config.Config{}, err
	}
	return repoRoot, cfgPath, cfg, nil
}

func resolveInstanceByName(repoRoot string, cfg config.Config, name string) (instances.Instance, error) {
	return instances.Get(repoRoot, cfg.Repo.AppsDir, name)
}

func readInstanceChart(inst instances.Instance) (yamlchart.Chart, error) {
	return yamlchart.ReadChart(filepath.Join(inst.Path, "Chart.yaml"))
}
