package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"helmdex/internal/catalog"
	"helmdex/internal/yamlchart"

	"gopkg.in/yaml.v3"
)

type depSourceKind string

const (
	depSourceCatalog     depSourceKind = "catalog"
	depSourceArtifactHub depSourceKind = "artifacthub"
	depSourceArbitrary   depSourceKind = "arbitrary"
)

type depSourceMeta struct {
	Kind      depSourceKind `yaml:"kind"`
	CatalogID string        `yaml:"catalogID,omitempty"`
	// CatalogSource is the configured source name that produced the catalog entry.
	// It corresponds to the `.helmdex/catalog/<source>.yaml` filename.
	CatalogSource string `yaml:"catalogSource,omitempty"`
}

func depMetaPath(repoRoot, instanceName string, depID yamlchart.DepID) string {
	// Stored at repo-level: .helmdex/depmeta/<instanceName>/<depID>.yaml
	return filepath.Join(repoRoot, ".helmdex", "depmeta", instanceName, fmt.Sprintf("%s.yaml", depID))
}

func depMetaInstanceDir(repoRoot, instanceName string) string {
	return filepath.Join(repoRoot, ".helmdex", "depmeta", instanceName)
}

func renameDepMetaInstanceDir(repoRoot, oldInstanceName, newInstanceName string) error {
	oldInstanceName = strings.TrimSpace(oldInstanceName)
	newInstanceName = strings.TrimSpace(newInstanceName)
	if oldInstanceName == "" || newInstanceName == "" || oldInstanceName == newInstanceName {
		return nil
	}
	oldDir := depMetaInstanceDir(repoRoot, oldInstanceName)
	newDir := depMetaInstanceDir(repoRoot, newInstanceName)
	if _, err := os.Stat(oldDir); err != nil {
		// Nothing to move.
		return nil
	}
	if _, err := os.Stat(newDir); err == nil {
		return fmt.Errorf("depmeta dir for instance %q already exists", newInstanceName)
	}
	if err := os.MkdirAll(filepath.Dir(newDir), 0o755); err != nil {
		return err
	}
	return os.Rename(oldDir, newDir)
}

func deleteDepMetaInstanceDir(repoRoot, instanceName string) error {
	instanceName = strings.TrimSpace(instanceName)
	if instanceName == "" {
		return nil
	}
	return os.RemoveAll(depMetaInstanceDir(repoRoot, instanceName))
}

func renameDepMetaFile(repoRoot, instanceName string, oldDepID, newDepID yamlchart.DepID) error {
	if strings.TrimSpace(repoRoot) == "" || strings.TrimSpace(instanceName) == "" {
		return nil
	}
	if strings.TrimSpace(string(oldDepID)) == "" || strings.TrimSpace(string(newDepID)) == "" || oldDepID == newDepID {
		return nil
	}
	oldP := depMetaPath(repoRoot, instanceName, oldDepID)
	newP := depMetaPath(repoRoot, instanceName, newDepID)
	if _, err := os.Stat(oldP); err != nil {
		return nil
	}
	if _, err := os.Stat(newP); err == nil {
		return fmt.Errorf("depmeta already exists for depID %q", newDepID)
	}
	if err := os.MkdirAll(filepath.Dir(newP), 0o755); err != nil {
		return err
	}
	return os.Rename(oldP, newP)
}

func deleteDepMetaFile(repoRoot, instanceName string, depID yamlchart.DepID) error {
	if strings.TrimSpace(repoRoot) == "" || strings.TrimSpace(instanceName) == "" || strings.TrimSpace(string(depID)) == "" {
		return nil
	}
	_ = os.Remove(depMetaPath(repoRoot, instanceName, depID))
	return nil
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
	// Backfill for legacy catalog deps that predate CatalogSource.
	if m.Kind == depSourceCatalog && strings.TrimSpace(m.CatalogSource) == "" && strings.TrimSpace(m.CatalogID) != "" {
		if entries, err := catalog.LoadLocalCatalogEntriesWithSource(repoRoot); err == nil {
			for _, e := range entries {
				if e.Entry.ID == strings.TrimSpace(m.CatalogID) {
					m.CatalogSource = e.SourceName
					// Best-effort persist so subsequent loads and dep list rendering show it.
					_ = writeDepSourceMeta(repoRoot, instanceName, depID, m)
					break
				}
			}
		}
	}
	return m, true
}

func depSourceTagAndLabel(meta depSourceMeta, ok bool) (tag, label string) {
	if !ok {
		return "", ""
	}
	switch meta.Kind {
	case depSourceCatalog:
		src := strings.TrimSpace(meta.CatalogSource)
		catID := strings.TrimSpace(meta.CatalogID)
		tagText := "CAT"
		labelText := "Catalog"
		if src != "" {
			tagText += " " + src
			labelText += " " + src
		}
		if catID != "" {
			labelText += " (" + catID + ")"
		}
		return withIcon(iconCatalog, tagText), withIcon(iconCatalog, labelText)
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
