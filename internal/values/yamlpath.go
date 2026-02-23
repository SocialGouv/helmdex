package values

import (
	"fmt"
	"strconv"
	"strings"
)

// yamlPath matches the TUI internal path key syntax: $ or $.a.b[0].c.
// It is intentionally limited to what we need for non-interactive scripting.
type yamlPathPart struct {
	key   string
	index *int
}

type yamlPath []yamlPathPart

// Path is the exported representation of a parsed values path.
// It is compatible with the TUI's internal path syntax.
type Path = yamlPath

// ParsePath parses a path in the TUI syntax (e.g. "$", "$.a.b[0].c").
func ParsePath(k string) (Path, error) { return parseYAMLPath(k) }

// GetAt returns the value at path, if present.
func GetAt(root any, p Path) (any, bool) { return getAt(root, yamlPath(p)) }

// SetAt sets the value at path and returns the (possibly replaced) root.
// If v is nil, SetAt deletes the key for object paths.
func SetAt(root any, p Path, v any) any { return setAt(root, yamlPath(p), v) }

func parseYAMLPath(k string) (yamlPath, error) {
	k = strings.TrimSpace(k)
	if k == "" || k == "$" {
		return yamlPath{}, nil
	}
	if !strings.HasPrefix(k, "$") {
		return nil, fmt.Errorf("path must start with '$' (got %q)", k)
	}
	rem := k[1:]
	out := yamlPath{}
	for len(rem) > 0 {
		if strings.HasPrefix(rem, ".") {
			rem = rem[1:]
			seg := rem
			cut := len(seg)
			for i := 0; i < len(seg); i++ {
				if seg[i] == '.' || seg[i] == '[' {
					cut = i
					break
				}
			}
			name := seg[:cut]
			if name == "" {
				return nil, fmt.Errorf("invalid path %q", k)
			}
			out = append(out, yamlPathPart{key: name})
			rem = seg[cut:]
			continue
		}
		if strings.HasPrefix(rem, "[") {
			i := strings.Index(rem, "]")
			if i < 0 {
				return nil, fmt.Errorf("invalid path %q", k)
			}
			idxS := rem[1:i]
			idx, err := strconv.Atoi(idxS)
			if err != nil {
				return nil, fmt.Errorf("invalid index %q in %q", idxS, k)
			}
			out = append(out, yamlPathPart{index: &idx})
			rem = rem[i+1:]
			continue
		}
		return nil, fmt.Errorf("invalid path %q", k)
	}
	return out, nil
}

func getAt(root any, p yamlPath) (any, bool) {
	cur := root
	for _, part := range p {
		if part.index != nil {
			arr, ok := cur.([]any)
			if !ok {
				return nil, false
			}
			idx := *part.index
			if idx < 0 || idx >= len(arr) {
				return nil, false
			}
			cur = arr[idx]
			continue
		}
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		v, ok := obj[part.key]
		if !ok {
			return nil, false
		}
		cur = v
	}
	return cur, true
}

// setAt sets a value at path. It creates missing objects/arrays on the way.
// When v is nil, it deletes the key (for object paths) or sets element to nil (for arrays).
func setAt(root any, p yamlPath, v any) any {
	if len(p) == 0 {
		return v
	}
	if root == nil {
		root = map[string]any{}
	}
	cur := root
	for i := 0; i < len(p)-1; i++ {
		part := p[i]
		nextPart := p[i+1]
		if part.index != nil {
			arr, ok := cur.([]any)
			if !ok {
				arr = []any{}
			}
			idx := *part.index
			for len(arr) <= idx {
				arr = append(arr, nil)
			}
			next := arr[idx]
			if next == nil {
				// Decide container type based on the next path part.
				if nextPart.index != nil {
					next = []any{}
				} else {
					next = map[string]any{}
				}
				arr[idx] = next
			}
			cur = next
			// write back to parent root if needed
			if i == 0 {
				root = arr
			}
			continue
		}
		obj, ok := cur.(map[string]any)
		if !ok {
			obj = map[string]any{}
			cur = obj
			if i == 0 {
				root = obj
			}
		}
		next := obj[part.key]
		if next == nil {
			if nextPart.index != nil {
				next = []any{}
			} else {
				next = map[string]any{}
			}
			obj[part.key] = next
		}
		cur = next
	}
	last := p[len(p)-1]
	if last.index != nil {
		arr, ok := cur.([]any)
		if !ok {
			arr = []any{}
		}
		idx := *last.index
		for len(arr) <= idx {
			arr = append(arr, nil)
		}
		arr[idx] = v
		return root
	}
	obj, ok := cur.(map[string]any)
	if !ok {
		obj = map[string]any{}
	}
	if v == nil {
		delete(obj, last.key)
		return root
	}
	obj[last.key] = v
	return root
}
