package presets

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"helmdex/internal/config"
	"helmdex/internal/yamlchart"

	semver "github.com/Masterminds/semver/v3"
)

type ResolvedDependency struct {
	Dependency yamlchart.Dependency

	// Fully-qualified paths in the local cache.
	DefaultPath  string
	PlatformPath string
	SetPaths     map[string]string // setName -> path
}

type Resolution struct {
	ByID map[yamlchart.DepID]ResolvedDependency
}

// Resolve finds preset file paths for the given instance dependencies.
//
// Sources are applied in config order; later sources override earlier sources
// within a layer type.
func Resolve(repoRoot string, cfg config.Config, deps []yamlchart.Dependency) (Resolution, error) {
	byID := map[yamlchart.DepID]ResolvedDependency{}
	for _, d := range deps {
		id := yamlchart.DependencyID(d)
		byID[id] = ResolvedDependency{Dependency: d, SetPaths: map[string]string{}}
	}

	for _, src := range cfg.Sources {
		if !src.Presets.Enabled {
			continue
		}
		chartsPath := src.Presets.ChartsPath
		if chartsPath == "" {
			chartsPath = "charts"
		}
		cacheRoot := filepath.Join(repoRoot, ".helmdex", "cache", src.Name, chartsPath)

		for _, d := range deps {
			id := yamlchart.DependencyID(d)
			current := byID[id]

			bestDir, err := bestPresetDir(filepath.Join(cacheRoot, d.Name), d.Version)
			if err != nil {
				return Resolution{}, fmt.Errorf("resolve presets for %s: %w", d.Name, err)
			}
			if bestDir == "" {
				// No presets in this source for this dep.
				byID[id] = current
				continue
			}

			// Default
			if p := filepath.Join(bestDir, "values.default.yaml"); fileExists(p) {
				current.DefaultPath = p
			}
			// Platform
			if cfg.Platform.Name != "" {
				pf := fmt.Sprintf("values.platform.%s.yaml", cfg.Platform.Name)
				if p := filepath.Join(bestDir, pf); fileExists(p) {
					current.PlatformPath = p
				}
			}
			// Sets (discover all sets; selection is based on which are imported locally).
			gl, _ := filepath.Glob(filepath.Join(bestDir, "values.set.*.yaml"))
			sort.Strings(gl)
			for _, p := range gl {
				setName := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(p), "values.set."), ".yaml")
				if setName == "" {
					continue
				}
				current.SetPaths[setName] = p
			}

			byID[id] = current
		}
	}

	return Resolution{ByID: byID}, nil
}

func bestPresetDir(chartRoot string, version string) (string, error) {
	entries, err := os.ReadDir(chartRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	v, err := semver.NewVersion(version)
	if err != nil {
		return "", fmt.Errorf("invalid semver %q", version)
	}

	// Prefer exact directory match.
	exact := filepath.Join(chartRoot, version)
	if dirExists(exact) {
		return exact, nil
	}

	// Otherwise, find all constraint directories that match and pick the most
	// specific (heuristic: longer constraint string, then lexical).
	best := ""
	bestLen := -1
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if name == version {
			continue
		}
		c, err := semver.NewConstraint(name)
		if err != nil {
			continue
		}
		if ok, _ := c.Validate(v); ok {
			if len(name) > bestLen || (len(name) == bestLen && name > best) {
				best = filepath.Join(chartRoot, name)
				bestLen = len(name)
			}
		}
	}
	return best, nil
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	if err != nil {
		return false
	}
	return !st.IsDir()
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	if err != nil {
		return false
	}
	return st.IsDir()
}
