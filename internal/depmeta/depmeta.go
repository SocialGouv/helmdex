// Package depmeta persists per-dependency source metadata (catalog /
// artifacthub / arbitrary) in helmdex's per-repo state dir. Shared by the
// CLI, TUI and server so attach/detach semantics stay in one place.
package depmeta

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"helmdex/internal/paths"
	"helmdex/internal/yamlchart"

	"gopkg.in/yaml.v3"
)

type Kind string

const (
	KindCatalog     Kind = "catalog"
	KindArtifactHub Kind = "artifacthub"
	KindArbitrary   Kind = "arbitrary"
)

type Meta struct {
	Kind      Kind   `yaml:"kind"`
	CatalogID string `yaml:"catalogID,omitempty"`
	// CatalogSource is the configured source name that produced the catalog
	// entry (matches the catalog/<source>.yaml state filename).
	CatalogSource string `yaml:"catalogSource,omitempty"`
}

// Path returns the metadata file for one dependency of one instance.
func Path(repoRoot, instanceName string, depID yamlchart.DepID) string {
	return paths.State(repoRoot, "depmeta", instanceName, fmt.Sprintf("%s.yaml", depID))
}

// InstanceDir returns the metadata dir of one instance.
func InstanceDir(repoRoot, instanceName string) string {
	return paths.State(repoRoot, "depmeta", instanceName)
}

// Read loads the metadata for a dependency; ok=false when absent/invalid.
func Read(repoRoot, instanceName string, depID yamlchart.DepID) (Meta, bool) {
	if strings.TrimSpace(repoRoot) == "" || strings.TrimSpace(instanceName) == "" || strings.TrimSpace(string(depID)) == "" {
		return Meta{}, false
	}
	b, err := os.ReadFile(Path(repoRoot, instanceName, depID))
	if err != nil {
		return Meta{}, false
	}
	var m Meta
	if err := yaml.Unmarshal(b, &m); err != nil {
		return Meta{}, false
	}
	if strings.TrimSpace(string(m.Kind)) == "" {
		return Meta{}, false
	}
	return m, true
}

// Write persists the metadata for a dependency.
func Write(repoRoot, instanceName string, depID yamlchart.DepID, m Meta) error {
	if strings.TrimSpace(repoRoot) == "" || strings.TrimSpace(instanceName) == "" || strings.TrimSpace(string(depID)) == "" {
		return fmt.Errorf("missing repoRoot/instanceName/depID")
	}
	p := Path(repoRoot, instanceName, depID)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}
