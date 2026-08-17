package instances

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"helmdex/internal/config"
	"helmdex/internal/yamlchart"
)

// Layout describes where instances (and optional blueprints) live in a repo.
type Layout struct {
	// AppsDir is the directory (relative to repoRoot) holding instances.
	AppsDir string
	// TemplatesDir holds blueprint chart dirs ("" when the repo has none).
	// Blueprints are listed separately and can be copied into AppsDir via
	// CreateFromTemplate.
	TemplatesDir string
}

// DiscoverLayout resolves the repo layout. Configured dirs win; otherwise it
// auto-detects, so helmdex works on repos that never opted into helmdex
// (no helmdex.yaml): any top-level dir whose children are chart dirs
// qualifies, `templates/` being reserved for blueprints.
func DiscoverLayout(repoRoot, cfgAppsDir, cfgTemplatesDir string) Layout {
	if cfgAppsDir == "" {
		cfgAppsDir = "apps"
	}

	templatesDir := cfgTemplatesDir
	if templatesDir == "" && hasChartSubdirs(filepath.Join(repoRoot, "templates")) {
		templatesDir = "templates"
	}

	appsDir := cfgAppsDir
	// Only auto-detect when the configured dir doesn't exist: an existing
	// (even empty) dir is an explicit choice.
	if _, err := os.Stat(filepath.Join(repoRoot, appsDir)); err != nil {
		if cand := detectAppsDir(repoRoot, templatesDir); cand != "" {
			appsDir = cand
		}
	}

	return Layout{AppsDir: appsDir, TemplatesDir: templatesDir}
}

// ApplyLayout returns cfg with its repo section replaced by the discovered
// layout. Explicit --config resolutions are trusted as-is.
func ApplyLayout(repoRoot string, res config.Resolved) config.Config {
	cfg := res.Config
	if res.Source == config.SourceFlag {
		return cfg
	}
	layout := DiscoverLayout(repoRoot, cfg.Repo.AppsDir, cfg.Repo.TemplatesDir)
	cfg.Repo.AppsDir = layout.AppsDir
	cfg.Repo.TemplatesDir = layout.TemplatesDir
	return cfg
}

// detectAppsDir scans top-level dirs for the one holding the most chart
// subdirs. Hidden dirs, the blueprints dir and helm's `charts/` convention
// are excluded. Ties resolve lexicographically for stability.
func detectAppsDir(repoRoot, templatesDir string) string {
	entries, err := os.ReadDir(repoRoot)
	if err != nil {
		return ""
	}
	best, bestCount := "", 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") || name == "charts" || name == templatesDir {
			continue
		}
		n := countChartSubdirs(filepath.Join(repoRoot, name))
		if n > bestCount {
			best, bestCount = name, n
		}
	}
	return best
}

func hasChartSubdirs(dir string) bool {
	return countChartSubdirs(dir) > 0
}

func countChartSubdirs(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, e.Name(), "Chart.yaml")); err == nil {
			n++
		}
	}
	return n
}

// ListTemplates lists blueprint chart dirs under the repo's templates dir.
func ListTemplates(repoRoot, templatesDir string) ([]Instance, error) {
	if templatesDir == "" {
		return []Instance{}, nil
	}
	return List(repoRoot, templatesDir)
}

// CreateFromTemplate copies a blueprint chart dir into the apps dir under
// newName and renames the chart. Only plain files are copied; nothing
// helmdex-specific is added, so agnostic repos stay agnostic.
func CreateFromTemplate(repoRoot, appsDir, templatesDir, templateName, newName string) (Instance, error) {
	if templatesDir == "" {
		return Instance{}, fmt.Errorf("this repo has no templates dir")
	}
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return Instance{}, fmt.Errorf("instance name is required")
	}
	if strings.Contains(newName, string(os.PathSeparator)) {
		return Instance{}, fmt.Errorf("invalid instance name %q", newName)
	}

	srcDir := filepath.Join(repoRoot, templatesDir, templateName)
	if _, err := os.Stat(filepath.Join(srcDir, "Chart.yaml")); err != nil {
		return Instance{}, fmt.Errorf("template %q not found at %s", templateName, srcDir)
	}
	dstDir := instanceDir(repoRoot, appsDir, newName)
	if _, err := os.Stat(dstDir); err == nil {
		return Instance{}, fmt.Errorf("instance already exists at %s", dstDir)
	}

	if err := copyTree(srcDir, dstDir); err != nil {
		// Leave no partial instance behind.
		_ = os.RemoveAll(dstDir)
		return Instance{}, fmt.Errorf("copy template %q: %w", templateName, err)
	}

	chartPath := filepath.Join(dstDir, "Chart.yaml")
	c, err := yamlchart.ReadChart(chartPath)
	if err != nil {
		return Instance{}, err
	}
	c.Name = newName
	if err := yamlchart.WriteChart(chartPath, c); err != nil {
		return Instance{}, err
	}

	return Instance{Name: newName, Path: dstDir}, nil
}

func copyTree(srcDir, dstDir string) error {
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(dstDir, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("unsupported file type at %s", path)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o644)
	})
}
