package presets

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"helmdex/internal/config"
	"helmdex/internal/yamlchart"

	"gopkg.in/yaml.v3"
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
	//
	// IMPORTANT: these layer files are written as a single YAML map keyed by
	// dependency id (alias if set else name), to avoid concatenating multi-document
	// YAML when an instance has multiple dependencies.
	if err := importDepLayer(filepath.Join(p.InstancePath, "values.default.yaml"), res, layerDefault); err != nil {
		return Resolution{}, err
	}
	if err := importDepLayer(filepath.Join(p.InstancePath, "values.platform.yaml"), res, layerPlatform); err != nil {
		return Resolution{}, err
	}

	// Import per-dependency set layers based on marker-file selection.
	// Selection rule: a set is selected for a dependency iff a marker file exists:
	//   values.dep-set.<depID>--<set>.yaml
	// The imported content overwrites that file.
	selectedByDep, err := selectedDepSets(p.InstancePath)
	if err != nil {
		return Resolution{}, err
	}
	for depID, setNames := range selectedByDep {
		rd, ok := res.ByID[yamlchart.DepID(depID)]
		for _, setName := range setNames {
			outPath := filepath.Join(p.InstancePath, fmt.Sprintf("values.dep-set.%s--%s.yaml", depID, setName))
			// Best-effort: if the dep isn't present in resolution, keep a valid YAML file.
			if !ok {
				_ = os.WriteFile(outPath, []byte("{}\n"), 0o644)
				continue
			}
			if err := importDepSet(outPath, rd, depID, setName); err != nil {
				return Resolution{}, err
			}
		}
	}

	// Backward-compat: global values.set.<set>.yaml selection.
	// (Older behavior selected sets by presence of values.set.<set>.yaml and
	// imported concatenated docs across deps.)
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

func importDepLayer(outPath string, res Resolution, kind layerKind) error {
	// Build a single mapping: depID -> valuesMap.
	// This ensures each dependency's presets apply under its umbrella key.
	out := map[string]any{}
	ids := make([]string, 0, len(res.ByID))
	for id := range res.ByID {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)
	for _, id := range ids {
		rd := res.ByID[yamlchart.DepID(id)]
		var src string
		switch kind {
		case layerDefault:
			src = rd.DefaultPath
		case layerPlatform:
			src = rd.PlatformPath
		default:
			src = ""
		}
		if strings.TrimSpace(src) == "" {
			continue
		}
		v, err := readYAMLAny(src)
		if err != nil {
			return err
		}
		out[id] = v
	}
	if len(out) == 0 {
		_ = os.Remove(outPath)
		return nil
	}
	b, err := yaml.Marshal(out)
	if err != nil {
		return err
	}
	if len(b) == 0 || b[len(b)-1] != '\n' {
		b = append(b, '\n')
	}
	return os.WriteFile(outPath, b, 0o644)
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

func selectedDepSets(instancePath string) (map[string][]string, error) {
	files, err := filepath.Glob(filepath.Join(instancePath, "values.dep-set.*--*.yaml"))
	if err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for _, f := range files {
		base := filepath.Base(f)
		name := strings.TrimSuffix(strings.TrimPrefix(base, "values.dep-set."), ".yaml")
		parts := strings.SplitN(name, "--", 2)
		if len(parts) != 2 {
			continue
		}
		depID := strings.TrimSpace(parts[0])
		setName := strings.TrimSpace(parts[1])
		if depID == "" || setName == "" {
			continue
		}
		out[depID] = append(out[depID], setName)
	}
	for depID := range out {
		sort.Strings(out[depID])
		out[depID] = unique(out[depID])
	}
	return out, nil
}

func unique(in []string) []string {
	if len(in) < 2 {
		return in
	}
	out := make([]string, 0, len(in))
	prev := ""
	for i, s := range in {
		if i == 0 || s != prev {
			out = append(out, s)
		}
		prev = s
	}
	return out
}

func readYAMLAny(path string) (any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var v any
	if err := yaml.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	if v == nil {
		return map[string]any{}, nil
	}
	return v, nil
}

func importDepSet(outPath string, rd ResolvedDependency, depID string, setName string) error {
	depID = strings.TrimSpace(depID)
	setName = strings.TrimSpace(setName)
	if depID == "" || setName == "" {
		return fmt.Errorf("invalid dep set: depID=%q set=%q", depID, setName)
	}

	// The remote preset file content is dependency-scoped (it should not be nested under depID).
	// We wrap it into a mapping so merging applies it under the umbrella dep key.
	val := map[string]any{}
	if p, ok := rd.SetPaths[setName]; ok {
		v, err := readYAMLAny(p)
		if err != nil {
			return err
		}
		val[depID] = v
	} else {
		// Selected set but no remote content; keep a valid empty map.
		val[depID] = map[string]any{}
	}
	b, err := yaml.Marshal(val)
	if err != nil {
		return err
	}
	if len(b) == 0 || b[len(b)-1] != '\n' {
		b = append(b, '\n')
	}
	return os.WriteFile(outPath, b, 0o644)
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
