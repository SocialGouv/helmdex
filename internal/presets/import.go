package presets

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"helmdex/internal/config"
	"helmdex/internal/yamlchart"
)

// ImportParams controls importing resolved preset layers into an instance
// directory.
type ImportParams struct {
	RepoRoot     string
	InstancePath string
	Config       config.Config
	Dependencies []yamlchart.Dependency
}

// Import copies resolved remote preset files into instance-managed layer files:
// - values.default.yaml
// - values.platform.yaml (if platform presets exist)
// - values.set.<set>.yaml for every set file currently present locally (selection)
//
// Set selection rule: a set is selected iff `values.set.<set>.yaml` exists in the
// instance directory. The imported file overwrites it.
//
// This function never modifies user-owned values.instance.yaml.
func Import(p ImportParams) (Resolution, error) {
	res, err := Resolve(p.RepoRoot, p.Config, p.Dependencies)
	if err != nil {
		return Resolution{}, err
	}

	if err := os.MkdirAll(p.InstancePath, 0o755); err != nil {
		return Resolution{}, err
	}

	// Import default/platform.
	if err := importLayer(filepath.Join(p.InstancePath, "values.default.yaml"), collectLayerFiles(res, layerDefault)); err != nil {
		return Resolution{}, err
	}
	if err := importLayer(filepath.Join(p.InstancePath, "values.platform.yaml"), collectLayerFiles(res, layerPlatform)); err != nil {
		return Resolution{}, err
	}

	// Import set layers for selected sets.
	selected, err := selectedSets(p.InstancePath)
	if err != nil {
		return Resolution{}, err
	}
	for _, setName := range selected {
		outPath := filepath.Join(p.InstancePath, fmt.Sprintf("values.set.%s.yaml", setName))
		if err := importSet(outPath, res, setName); err != nil {
			return Resolution{}, err
		}
	}

	return res, nil
}

type layerKind int

const (
	layerDefault layerKind = iota
	layerPlatform
)

func collectLayerFiles(res Resolution, kind layerKind) []string {
	paths := []string{}
	for _, rd := range res.ByID {
		switch kind {
		case layerDefault:
			if rd.DefaultPath != "" {
				paths = append(paths, rd.DefaultPath)
			}
		case layerPlatform:
			if rd.PlatformPath != "" {
				paths = append(paths, rd.PlatformPath)
			}
		}
	}
	sort.Strings(paths)
	return paths
}

func importLayer(outPath string, srcPaths []string) error {
	if len(srcPaths) == 0 {
		// No layer to import; remove existing file to avoid stale generated content.
		_ = os.Remove(outPath)
		return nil
	}

	// Simple strategy for v0.2:
	// - If exactly one file exists, copy it verbatim.
	// - If multiple sources have this layer, concatenate docs with a comment.
	//
	// (Future: YAML-aware merge, keyed by dependency id.)
	if len(srcPaths) == 1 {
		b, err := os.ReadFile(srcPaths[0])
		if err != nil {
			return err
		}
		return os.WriteFile(outPath, b, 0o644)
	}

	buf := strings.Builder{}
	for i, p := range srcPaths {
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if i > 0 {
			buf.WriteString("\n---\n")
		}
		buf.WriteString("# imported from ")
		buf.WriteString(p)
		buf.WriteString("\n")
		buf.Write(b)
		if len(b) == 0 || b[len(b)-1] != '\n' {
			buf.WriteString("\n")
		}
	}
	return os.WriteFile(outPath, []byte(buf.String()), 0o644)
}

func selectedSets(instancePath string) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(instancePath, "values.set.*.yaml"))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return []string{}, nil
	}
	sets := make([]string, 0, len(files))
	for _, f := range files {
		base := filepath.Base(f)
		name := strings.TrimSuffix(strings.TrimPrefix(base, "values.set."), ".yaml")
		if name == "" {
			continue
		}
		sets = append(sets, name)
	}
	sort.Strings(sets)
	return sets, nil
}

func importSet(outPath string, res Resolution, setName string) error {
	// Import set files for each dependency, best-effort.
	// v0.2 minimal behavior: concatenate each dep's set file (if present) with doc separators.
	paths := []string{}
	for _, rd := range res.ByID {
		if p, ok := rd.SetPaths[setName]; ok {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		// Selected set but no remote content; keep file but make it empty map to be valid YAML.
		return os.WriteFile(outPath, []byte("{}\n"), 0o644)
	}
	return importLayer(outPath, paths)
}

