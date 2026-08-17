package values

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// EditFileName returns the values file helmdex edits for an instance:
// values.instance.yaml for managed instances (layered, merged into a
// generated values.yaml), values.yaml itself for direct ones (user-owned,
// edited in place, never generated).
func EditFileName(instanceDir string) string {
	if IsManaged(instanceDir) {
		return "values.instance.yaml"
	}
	return "values.yaml"
}

// EditFilePath returns the absolute path of EditFileName.
func EditFilePath(instanceDir string) string {
	return filepath.Join(instanceDir, EditFileName(instanceDir))
}

// ReadInstanceValues reads the instance's edit file (see EditFileName) into a
// generic map. Missing or empty files return an empty map.
func ReadInstanceValues(instanceDir string) (map[string]any, error) {
	p := EditFilePath(instanceDir)
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	if len(b) == 0 {
		return map[string]any{}, nil
	}
	var root any
	if err := yaml.Unmarshal(b, &root); err != nil {
		return nil, fmt.Errorf("parse %s: %w", p, err)
	}
	obj, ok := root.(map[string]any)
	if !ok || obj == nil {
		// If user wrote a scalar/list at root, treat it as empty to avoid panics.
		return map[string]any{}, nil
	}
	return obj, nil
}

// WriteInstanceValues replaces the instance's edit file (see EditFileName)
// wholesale. Prefer targeted edits via SetInFile to preserve comments.
func WriteInstanceValues(instanceDir string, root map[string]any) error {
	if root == nil {
		root = map[string]any{}
	}
	b, err := yaml.Marshal(root)
	if err != nil {
		return err
	}
	p := EditFilePath(instanceDir)
	if err := os.MkdirAll(instanceDir, 0o755); err != nil {
		return err
	}
	// Keep a trailing newline for nicer diffs.
	if len(b) == 0 || b[len(b)-1] != '\n' {
		b = append(b, '\n')
	}
	return os.WriteFile(p, b, 0o644)
}
