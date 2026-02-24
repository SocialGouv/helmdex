package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"helmdex/internal/yamlchart"

	"gopkg.in/yaml.v3"
)

type depSourceKind string

const (
	depSourceCatalog   depSourceKind = "catalog"
	depSourceArtifactHub depSourceKind = "artifacthub"
	depSourceArbitrary depSourceKind = "arbitrary"
)

type depSourceMeta struct {
	Kind     depSourceKind `yaml:"kind"`
	CatalogID string       `yaml:"catalogID,omitempty"`
}

func depMetaPath(repoRoot, instanceName string, depID yamlchart.DepID) string {
	// Stored at repo-level: .helmdex/depmeta/<instanceName>/<depID>.yaml
	return filepath.Join(repoRoot, ".helmdex", "depmeta", instanceName, fmt.Sprintf("%s.yaml", depID))
}

func writeDepSourceMeta(repoRoot, instanceName string, depID yamlchart.DepID, meta depSourceMeta) error {
	if strings.TrimSpace(repoRoot) == "" || strings.TrimSpace(instanceName) == "" || strings.TrimSpace(string(depID)) == "" {
		return fmt.Errorf("missing repoRoot/instanceName/depID")
	}
	p := depMetaPath(repoRoot, instanceName, depID)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	buf, err := yaml.Marshal(meta)
	if err != nil {
		return err
	}
	return os.WriteFile(p, buf, 0o644)
}

func readDepSourceMeta(repoRoot, instanceName string, depID yamlchart.DepID) (depSourceMeta, bool) {
	if strings.TrimSpace(repoRoot) == "" || strings.TrimSpace(instanceName) == "" || strings.TrimSpace(string(depID)) == "" {
		return depSourceMeta{}, false
	}
	p := depMetaPath(repoRoot, instanceName, depID)
	buf, err := os.ReadFile(p)
	if err != nil {
		return depSourceMeta{}, false
	}
	var m depSourceMeta
	if err := yaml.Unmarshal(buf, &m); err != nil {
		return depSourceMeta{}, false
	}
	if strings.TrimSpace(string(m.Kind)) == "" {
		return depSourceMeta{}, false
	}
	return m, true
}

func depSourceTagAndLabel(meta depSourceMeta, ok bool) (tag, label string) {
	if !ok {
		return "", ""
	}
	switch meta.Kind {
	case depSourceCatalog:
		if strings.TrimSpace(meta.CatalogID) != "" {
			return withIcon(iconCatalog, "CAT"), withIcon(iconCatalog, "Catalog") + " (" + meta.CatalogID + ")"
		}
		return withIcon(iconCatalog, "CAT"), withIcon(iconCatalog, "Catalog")
	case depSourceArtifactHub:
		return withIcon(iconAH, "AH"), withIcon(iconAH, "Artifact Hub")
	case depSourceArbitrary:
		return withIcon(iconCustom, "ARB"), withIcon(iconCustom, "Arbitrary")
	default:
		return "", ""
	}
}

func (m AppModel) writeSelectedDepSourceMeta(dep yamlchart.Dependency, meta depSourceMeta) error {
	if m.selected == nil {
		return nil
	}
	instName := strings.TrimSpace(m.selected.Name)
	if instName == "" {
		return nil
	}
	depID := yamlchart.DependencyID(dep)
	return writeDepSourceMeta(m.params.RepoRoot, instName, depID, meta)
}
