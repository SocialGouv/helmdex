package schemaform

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// JSON is a thin, JSON-serialization-compatible type used as the underlying
// representation for values edited via schema.
//
// We keep it in interface{} form so we can:
// - unmarshal JSON schema fragments easily
// - validate and coerce scalars
// - marshal into YAML when persisting
type JSON = any

// Schema is a partial JSON Schema model sufficient for Helm's values.schema.json.
//
// Notes:
// - We intentionally keep this permissive: unknown fields are ignored.
// - We support local $ref via JSON Pointer.
// - For allOf, we merge object properties/required heuristically.
// - For oneOf/anyOf, selection is delegated to caller UI (we keep raw subschemas).
type Schema struct {
	ID          string `json:"$id,omitempty"`
	Ref         string `json:"$ref,omitempty"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Type        any    `json:"type,omitempty"` // string or []string

	Properties           map[string]*Schema `json:"properties,omitempty"`
	Required             []string           `json:"required,omitempty"`
	AdditionalProperties any                `json:"additionalProperties,omitempty"` // bool or *Schema

	Items *Schema `json:"items,omitempty"`

	Enum []any `json:"enum,omitempty"`

	Default any `json:"default,omitempty"`

	Pattern string   `json:"pattern,omitempty"`
	Minimum *float64 `json:"minimum,omitempty"`
	Maximum *float64 `json:"maximum,omitempty"`
	MinLength *int   `json:"minLength,omitempty"`
	MaxLength *int   `json:"maxLength,omitempty"`
	MinItems  *int   `json:"minItems,omitempty"`
	MaxItems  *int   `json:"maxItems,omitempty"`

	OneOf []*Schema `json:"oneOf,omitempty"`
	AnyOf []*Schema `json:"anyOf,omitempty"`
	AllOf []*Schema `json:"allOf,omitempty"`

	// Non-standard but used by some charts to hint UI widgets.
	SliderMin  *float64 `json:"sliderMin,omitempty"`
	SliderMax  *float64 `json:"sliderMax,omitempty"`
	SliderUnit string   `json:"sliderUnit,omitempty"`
}

func ParseSchema(jsonText string) (*Schema, error) {
	var s Schema
	dec := json.NewDecoder(strings.NewReader(jsonText))
	dec.UseNumber()
	if err := dec.Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

// ResolveLocalRefs resolves local $ref pointers (#/...) by copying the referenced
// schema into place. It does NOT fetch remote refs.
func ResolveLocalRefs(root *Schema) error {
	if root == nil {
		return nil
	}
	return resolveLocalRefs(root, root, 0)
}

const maxRefDepth = 64

func resolveLocalRefs(root, cur *Schema, depth int) error {
	if cur == nil {
		return nil
	}
	if depth > maxRefDepth {
		return fmt.Errorf("schema $ref recursion too deep")
	}
	if strings.HasPrefix(cur.Ref, "#/") {
		target, err := lookupPointer(root, cur.Ref)
		if err != nil {
			return err
		}
		if target == nil {
			return fmt.Errorf("unresolved $ref %q", cur.Ref)
		}
		// Replace fields from target into cur, while keeping local annotations when present.
		merged := *target
		if cur.Title != "" {
			merged.Title = cur.Title
		}
		if cur.Description != "" {
			merged.Description = cur.Description
		}
		// Prevent re-resolving the same ref loop.
		merged.Ref = ""
		*cur = merged
	}

	for _, ss := range cur.AllOf {
		if err := resolveLocalRefs(root, ss, depth+1); err != nil {
			return err
		}
	}
	for _, ss := range cur.OneOf {
		if err := resolveLocalRefs(root, ss, depth+1); err != nil {
			return err
		}
	}
	for _, ss := range cur.AnyOf {
		if err := resolveLocalRefs(root, ss, depth+1); err != nil {
			return err
		}
	}
	if cur.Items != nil {
		if err := resolveLocalRefs(root, cur.Items, depth+1); err != nil {
			return err
		}
	}
	for _, p := range cur.Properties {
		if err := resolveLocalRefs(root, p, depth+1); err != nil {
			return err
		}
	}
	if ap, ok := cur.AdditionalProperties.(*Schema); ok {
		if err := resolveLocalRefs(root, ap, depth+1); err != nil {
			return err
		}
	}
	return nil
}

// lookupPointer supports a minimal JSON Pointer subset used by Helm schemas.
// It can traverse into Schema objects by known fields (properties/oneOf/anyOf/allOf/items).
func lookupPointer(root *Schema, ref string) (*Schema, error) {
	if root == nil {
		return nil, nil
	}
	if ref == "#" {
		return root, nil
	}
	if !strings.HasPrefix(ref, "#/") {
		return nil, fmt.Errorf("only local $ref supported, got %q", ref)
	}
	parts := strings.Split(ref[2:], "/")
	cur := root
	for i := 0; i < len(parts); i++ {
		p := unescapeJSONPointer(parts[i])
		switch p {
		case "properties":
			i++
			if i >= len(parts) {
				return nil, fmt.Errorf("invalid pointer %q", ref)
			}
			k := unescapeJSONPointer(parts[i])
			if cur.Properties == nil {
				return nil, nil
			}
			cur = cur.Properties[k]
			if cur == nil {
				return nil, nil
			}
		case "items":
			cur = cur.Items
			if cur == nil {
				return nil, nil
			}
		case "oneOf":
			i++
			idx, err := atoiPart(ref, parts, i)
			if err != nil {
				return nil, err
			}
			if idx < 0 || idx >= len(cur.OneOf) {
				return nil, nil
			}
			cur = cur.OneOf[idx]
		case "anyOf":
			i++
			idx, err := atoiPart(ref, parts, i)
			if err != nil {
				return nil, err
			}
			if idx < 0 || idx >= len(cur.AnyOf) {
				return nil, nil
			}
			cur = cur.AnyOf[idx]
		case "allOf":
			i++
			idx, err := atoiPart(ref, parts, i)
			if err != nil {
				return nil, err
			}
			if idx < 0 || idx >= len(cur.AllOf) {
				return nil, nil
			}
			cur = cur.AllOf[idx]
		default:
			// Unknown traversal segment.
			return nil, fmt.Errorf("unsupported JSON pointer segment %q in %q", p, ref)
		}
	}
	return cur, nil
}

func unescapeJSONPointer(s string) string {
	s = strings.ReplaceAll(s, "~1", "/")
	s = strings.ReplaceAll(s, "~0", "~")
	return s
}

func atoiPart(ref string, parts []string, i int) (int, error) {
	if i >= len(parts) {
		return 0, fmt.Errorf("invalid pointer %q", ref)
	}
	idxS := unescapeJSONPointer(parts[i])
	idx, err := strconv.Atoi(idxS)
	if err != nil {
		return 0, fmt.Errorf("invalid pointer index %q in %q", idxS, ref)
	}
	return idx, nil
}
