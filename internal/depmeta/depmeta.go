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

// validate guards the two user-supplied path components. The instance name
// and the dependency id reach this package from URL segments, request bodies
// and chart files; unvalidated, they address any file the process can touch.
func validate(repoRoot, instanceName string, depID yamlchart.DepID) error {
	if strings.TrimSpace(repoRoot) == "" {
		return fmt.Errorf("repoRoot is required")
	}
	if err := paths.ValidateSegment("instance name", instanceName); err != nil {
		return err
	}
	return paths.ValidateSegment("dependency id", string(depID))
}

// Read loads the metadata for a dependency; ok=false when absent/invalid.
func Read(repoRoot, instanceName string, depID yamlchart.DepID) (Meta, bool) {
	if err := validate(repoRoot, instanceName, depID); err != nil {
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

// Remove drops the metadata for a dependency. Removing a dependency frees its
// id: metadata left behind would be inherited by whatever takes that id next.
// Absent metadata is not an error.
func Remove(repoRoot, instanceName string, depID yamlchart.DepID) error {
	if err := validate(repoRoot, instanceName, depID); err != nil {
		return err
	}
	if err := os.Remove(Path(repoRoot, instanceName, depID)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Write persists the metadata for a dependency.
func Write(repoRoot, instanceName string, depID yamlchart.DepID, m Meta) error {
	if err := validate(repoRoot, instanceName, depID); err != nil {
		return err
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
