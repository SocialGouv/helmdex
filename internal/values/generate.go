package values

import (
	"fmt"
	"os"
	"path/filepath"

	"helmdex/internal/merge"

	"gopkg.in/yaml.v3"
)

// GenerateMergedValues generates `values.yaml` in the instance directory.
// It deep-merges (in order):
// - values.default.yaml (if exists)
// - values.platform.yaml (if exists)
// - values.set.*.yaml (if exists; lexicographic order)
// - values.instance.yaml (must exist; user-owned)
func GenerateMergedValues(instanceDir string) error {
	paths := []string{}
	addIfExists := func(p string, required bool) error {
		if _, err := os.Stat(p); err != nil {
			if required {
				return fmt.Errorf("missing required values file %s", p)
			}
			return nil
		}
		paths = append(paths, p)
		return nil
	}

	if err := addIfExists(filepath.Join(instanceDir, "values.default.yaml"), false); err != nil {
		return err
	}
	if err := addIfExists(filepath.Join(instanceDir, "values.platform.yaml"), false); err != nil {
		return err
	}

	setFiles, _ := filepath.Glob(filepath.Join(instanceDir, "values.set.*.yaml"))
	paths = append(paths, setFiles...)

	// Per-dependency set layers (selected via marker files, downloaded on apply).
	// These files are expected to contain a YAML map keyed by depID.
	depSetFiles, _ := filepath.Glob(filepath.Join(instanceDir, "values.dep-set.*--*.yaml"))
	paths = append(paths, depSetFiles...)

	if err := addIfExists(filepath.Join(instanceDir, "values.instance.yaml"), true); err != nil {
		return err
	}

	var merged *yaml.Node
	for _, p := range paths {
		n, err := loadYAMLDoc(p)
		if err != nil {
			return fmt.Errorf("read %s: %w", p, err)
		}
		merged, err = merge.DeepMerge(merged, n)
		if err != nil {
			return fmt.Errorf("merge %s: %w", p, err)
		}
	}

	if merged == nil {
		merged = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	}

	// Ensure a document node for encoding.
	doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{merged}}

	outPath := filepath.Join(instanceDir, "values.yaml")
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck // file close

	enc := yaml.NewEncoder(f)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		_ = enc.Close()
		return err
	}
	return enc.Close()
}

func loadYAMLDoc(path string) (*yaml.Node, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		return doc.Content[0], nil
	}
	return &doc, nil
}
