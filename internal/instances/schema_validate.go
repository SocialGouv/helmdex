package instances

import (
	"context"
	"fmt"
	"path/filepath"

	"helmdex/internal/schemaform"
	"helmdex/internal/yamlchart"

	"gopkg.in/yaml.v3"
)

// SchemaViolation is one values entry that breaks a dependency's
// values.schema.json.
type SchemaViolation struct {
	// Dep is the dependency id (alias or name) whose schema was violated.
	Dep string `json:"dep"`
	// Path is a dot/bracket path within that dependency's values.
	Path string `json:"path"`
	// Message describes the broken constraint.
	Message string `json:"message"`
}

// ValidateValuesAgainstSchemas validates a values document per dependency:
// each top-level key matching a dependency (by alias/name) is checked against
// that dependency's values.schema.json. Dependencies that ship no schema — or
// whose schema cannot be fetched/parsed — are skipped, so this only ever
// reports genuine violations. It is best-effort and non-authoritative: it
// exists to warn before apply, not to gate it.
func ValidateValuesAgainstSchemas(ctx context.Context, repoRoot, instPath string, valuesYAML []byte) ([]SchemaViolation, error) {
	var values map[string]any
	if err := yaml.Unmarshal(valuesYAML, &values); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}
	if len(values) == 0 {
		return nil, nil
	}

	c, err := yamlchart.ReadChart(filepath.Join(instPath, "Chart.yaml"))
	if err != nil {
		return nil, err
	}

	out := []SchemaViolation{}
	for _, d := range c.Dependencies {
		id := string(yamlchart.DependencyID(d))
		sub, ok := values[id]
		if !ok {
			continue // no values provided for this dependency
		}
		schemaJSON, err := LoadDepInspectContent(ctx, repoRoot, instPath, d, InspectSchema)
		if err != nil {
			continue // no schema (absent) or unreachable — nothing to check
		}
		sc, err := schemaform.ParseSchema(schemaJSON)
		if err != nil {
			continue
		}
		_ = schemaform.ResolveLocalRefs(sc)
		for _, v := range sc.Validate(sub) {
			out = append(out, SchemaViolation{Dep: id, Path: v.Path, Message: v.Message})
		}
	}
	return out, nil
}
