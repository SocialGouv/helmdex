package values

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ReadInstanceValues reads <instanceDir>/values.instance.yaml into a generic map.
// Missing or empty files return an empty map.
func ReadInstanceValues(instanceDir string) (map[string]any, error) {
	p := filepath.Join(instanceDir, "values.instance.yaml")
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
		return nil, fmt.Errorf("parse values.instance.yaml: %w", err)
	}
	obj, ok := root.(map[string]any)
	if !ok || obj == nil {
		// If user wrote a scalar/list at root, treat it as empty to avoid panics.
		return map[string]any{}, nil
	}
	return obj, nil
}

// WriteInstanceValues writes <instanceDir>/values.instance.yaml.
func WriteInstanceValues(instanceDir string, root map[string]any) error {
	if root == nil {
		root = map[string]any{}
	}
	b, err := yaml.Marshal(root)
	if err != nil {
		return err
	}
	p := filepath.Join(instanceDir, "values.instance.yaml")
	if err := os.MkdirAll(instanceDir, 0o755); err != nil {
		return err
	}
	// Keep a trailing newline for nicer diffs.
	if len(b) == 0 || b[len(b)-1] != '\n' {
		b = append(b, '\n')
	}
	return os.WriteFile(p, b, 0o644)
}
