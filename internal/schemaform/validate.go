package schemaform

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
)

// toFloat coerces a JSON/YAML numeric value to float64. Schema numbers are
// json.Number (ParseSchema uses UseNumber); YAML numbers are int/float64.
func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	}
	return 0, false
}

func isInteger(v any) bool {
	switch n := v.(type) {
	case int, int64, int32:
		return true
	case json.Number:
		_, err := n.Int64()
		return err == nil
	case float64:
		return n == math.Trunc(n)
	}
	return false
}

// Violation is a single schema constraint a value failed. Path is a
// dot/bracket path from the validated root (empty for the root itself).
type Violation struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// Validate checks value (a YAML/JSON-decoded any: map[string]any, []any,
// string, bool, int/int64/float64, or nil) against the schema and returns
// every violation found. It is deliberately best-effort: unknown keywords are
// ignored, and it never errors — a values file that cannot be validated
// simply yields no violations.
func (s *Schema) Validate(value any) []Violation {
	if s == nil {
		return nil
	}
	var out []Violation
	s.validate("", value, &out)
	return out
}

func (s *Schema) validate(path string, v any, out *[]Violation) {
	if s == nil {
		return
	}
	add := func(format string, args ...any) {
		*out = append(*out, Violation{Path: path, Message: fmt.Sprintf(format, args...)})
	}

	// allOf: every subschema must hold.
	for _, sub := range s.AllOf {
		sub.validate(path, v, out)
	}
	// anyOf / oneOf: pass if at least one subschema accepts v (lenient — a
	// warning system should not flag a value that satisfies one branch).
	if len(s.AnyOf) > 0 && !anyAccepts(s.AnyOf, v) {
		add("value does not match any of the allowed shapes")
	}
	if len(s.OneOf) > 0 && !anyAccepts(s.OneOf, v) {
		add("value does not match any of the allowed shapes")
	}

	// A nil value only violates when the type explicitly excludes null; most
	// Helm schemas allow an unset value.
	if v == nil {
		return
	}

	if types := normalizeTypes(s.Type); len(types) > 0 && !matchesAnyType(types, v) {
		add("expected type %s, got %s", strings.Join(types, "|"), yamlTypeName(v))
		return // further checks assume the type held
	}

	if len(s.Enum) > 0 && !enumContains(s.Enum, v) {
		add("value %v is not one of the allowed values", v)
	}

	switch vv := v.(type) {
	case map[string]any:
		for _, req := range s.Required {
			if _, ok := vv[req]; !ok {
				*out = append(*out, Violation{Path: joinPath(path, req), Message: "required property is missing"})
			}
		}
		for key, val := range vv {
			if sub, ok := s.Properties[key]; ok {
				sub.validate(joinPath(path, key), val, out)
			} else if s.AdditionalProperties == false {
				*out = append(*out, Violation{Path: joinPath(path, key), Message: "unknown property (additionalProperties is false)"})
			} else if sub, ok := s.AdditionalProperties.(*Schema); ok {
				sub.validate(joinPath(path, key), val, out)
			}
		}
	case []any:
		if s.MinItems != nil && len(vv) < *s.MinItems {
			add("expected at least %d items, got %d", *s.MinItems, len(vv))
		}
		if s.MaxItems != nil && len(vv) > *s.MaxItems {
			add("expected at most %d items, got %d", *s.MaxItems, len(vv))
		}
		if s.Items != nil {
			for i, item := range vv {
				s.Items.validate(fmt.Sprintf("%s[%d]", path, i), item, out)
			}
		}
	case string:
		if s.MinLength != nil && len(vv) < *s.MinLength {
			add("string is shorter than the minimum length %d", *s.MinLength)
		}
		if s.MaxLength != nil && len(vv) > *s.MaxLength {
			add("string is longer than the maximum length %d", *s.MaxLength)
		}
		if s.Pattern != "" {
			if re, err := regexp.Compile(s.Pattern); err == nil && !re.MatchString(vv) {
				add("string does not match pattern %s", s.Pattern)
			}
		}
	default:
		if f, ok := toFloat(v); ok {
			if s.Minimum != nil && f < *s.Minimum {
				add("value %v is less than the minimum %v", v, *s.Minimum)
			}
			if s.Maximum != nil && f > *s.Maximum {
				add("value %v is greater than the maximum %v", v, *s.Maximum)
			}
		}
	}
}

// accepts reports whether v satisfies s with no violations (used by anyOf/oneOf).
func (s *Schema) accepts(v any) bool {
	var out []Violation
	s.validate("", v, &out)
	return len(out) == 0
}

func anyAccepts(subs []*Schema, v any) bool {
	for _, sub := range subs {
		if sub.accepts(v) {
			return true
		}
	}
	return false
}

func normalizeTypes(t any) []string {
	switch tt := t.(type) {
	case string:
		if tt == "" {
			return nil
		}
		return []string{tt}
	case []any:
		var out []string
		for _, e := range tt {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return tt
	}
	return nil
}

func matchesAnyType(types []string, v any) bool {
	for _, t := range types {
		if matchesType(t, v) {
			return true
		}
	}
	return false
}

func matchesType(t string, v any) bool {
	switch t {
	case "object":
		_, ok := v.(map[string]any)
		return ok
	case "array":
		_, ok := v.([]any)
		return ok
	case "string":
		_, ok := v.(string)
		return ok
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "null":
		return v == nil
	case "integer":
		return isInteger(v)
	case "number":
		_, ok := toFloat(v)
		return ok
	}
	// Unknown type keyword: don't flag.
	return true
}

func yamlTypeName(v any) string {
	switch v.(type) {
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case bool:
		return "boolean"
	case nil:
		return "null"
	default:
		if isInteger(v) {
			return "integer"
		}
		if _, ok := toFloat(v); ok {
			return "number"
		}
		return "unknown"
	}
}

func enumContains(enum []any, v any) bool {
	for _, e := range enum {
		if equalScalar(e, v) {
			return true
		}
	}
	return false
}

func equalScalar(a, b any) bool {
	if fa, oka := toFloat(a); oka {
		if fb, okb := toFloat(b); okb {
			return fa == fb
		}
		return false
	}
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

func joinPath(base, key string) string {
	if base == "" {
		return key
	}
	return base + "." + key
}
