package schemaform

import "testing"

func mustSchema(t *testing.T, s string) *Schema {
	t.Helper()
	sc, err := ParseSchema(s)
	if err != nil {
		t.Fatalf("parse schema: %v", err)
	}
	if err := ResolveLocalRefs(sc); err != nil {
		t.Fatalf("resolve refs: %v", err)
	}
	return sc
}

func TestValidate(t *testing.T) {
	schema := mustSchema(t, `{
		"type": "object",
		"required": ["replicaCount"],
		"additionalProperties": false,
		"properties": {
			"replicaCount": {"type": "integer", "minimum": 1},
			"image": {
				"type": "object",
				"properties": {
					"pullPolicy": {"type": "string", "enum": ["Always", "IfNotPresent", "Never"]}
				}
			},
			"ports": {"type": "array", "items": {"type": "integer"}}
		}
	}`)

	// Valid values: no violations.
	ok := map[string]any{
		"replicaCount": 2,
		"image":        map[string]any{"pullPolicy": "IfNotPresent"},
		"ports":        []any{80, 443},
	}
	if v := schema.Validate(ok); len(v) != 0 {
		t.Fatalf("valid values reported violations: %+v", v)
	}

	// A bag of violations, one per broken constraint.
	bad := map[string]any{
		"replicaCount": "two",                                     // wrong type
		"image":        map[string]any{"pullPolicy": "Sometimes"}, // bad enum
		"ports":        []any{80, "https"},                        // bad item type
		"extra":        true,                                      // additionalProperties:false
	}
	got := map[string]bool{}
	for _, viol := range schema.Validate(bad) {
		got[viol.Path] = true
	}
	for _, want := range []string{"replicaCount", "image.pullPolicy", "ports[1]", "extra"} {
		if !got[want] {
			t.Errorf("expected a violation at %q; got %v", want, got)
		}
	}

	// Missing required property is reported at its path.
	missing := schema.Validate(map[string]any{"image": map[string]any{}})
	found := false
	for _, viol := range missing {
		if viol.Path == "replicaCount" && viol.Message == "required property is missing" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing required not reported: %+v", missing)
	}

	// minimum is enforced.
	if v := schema.Validate(map[string]any{"replicaCount": 0}); len(v) == 0 {
		t.Fatal("replicaCount below minimum should violate")
	}

	// nil/absent optional value is fine.
	if v := schema.Validate(map[string]any{"replicaCount": 1, "image": nil}); len(v) != 0 {
		t.Fatalf("nil optional should not violate: %+v", v)
	}
}
