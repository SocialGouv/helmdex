package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// LoadLocalCatalogEntries loads all `*.yaml` catalog files from `.helmdex/catalog/`.
// This reads the cache produced by [`catalog sync`](internal/catalog/sync.go:1).
func LoadLocalCatalogEntries(repoRoot string) ([]Entry, error) {
	catDir := filepath.Join(repoRoot, ".helmdex", "catalog")
	files, err := filepath.Glob(filepath.Join(catDir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return []Entry{}, nil
	}

	entriesByID := map[string]Entry{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var c Catalog
		if err := yaml.Unmarshal(b, &c); err != nil {
			return nil, fmt.Errorf("parse %s: %w", f, err)
		}
		for _, e := range c.Entries {
			if e.ID == "" {
				continue
			}
			// Later files override earlier ones.
			entriesByID[e.ID] = e
		}
	}

	out := make([]Entry, 0, len(entriesByID))
	for _, e := range entriesByID {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

