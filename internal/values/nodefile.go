package values

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Node-file editing: read-modify-write of YAML files via yaml.Node so
// comments and ordering of untouched nodes survive the rewrite. This is what
// makes in-place edits of user-owned values files (direct-mode instances in
// helmdex-agnostic repos) non-destructive.

// GetInFile returns the value at path p in a YAML file.
// A missing file reports (nil, false, nil).
func GetInFile(path string, p Path) (any, bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	var root any
	if err := yaml.Unmarshal(b, &root); err != nil {
		return nil, false, fmt.Errorf("parse %s: %w", path, err)
	}
	v, ok := getAt(root, p)
	return v, ok, nil
}

// SetInFile sets path p to v in a YAML file, creating missing maps on the
// way. v == nil deletes the key for map paths. Comments and ordering of
// untouched nodes are preserved.
func SetInFile(path string, p Path, v any) error {
	doc, err := loadNodeDoc(path)
	if err != nil {
		return err
	}
	root := doc.Content[0]

	if len(p) == 0 {
		n := &yaml.Node{}
		if err := n.Encode(v); err != nil {
			return err
		}
		doc.Content[0] = n
		return writeNodeDoc(path, doc)
	}

	if err := setNodeAt(root, p, v); err != nil {
		return fmt.Errorf("set %s in %s: %w", renderPath(p), path, err)
	}
	return writeNodeDoc(path, doc)
}

func loadNodeDoc(path string) (*yaml.Node, error) {
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	var doc yaml.Node
	if len(b) > 0 {
		if err := yaml.Unmarshal(b, &doc); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		doc = yaml.Node{
			Kind:    yaml.DocumentNode,
			Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}},
		}
	}
	// Normalize a null root (empty file with comments, `null`, …) to a map so
	// paths can be created.
	if doc.Content[0].Kind == yaml.ScalarNode && doc.Content[0].Tag == "!!null" {
		doc.Content[0] = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	}
	return &doc, nil
}

func writeNodeDoc(path string, doc *yaml.Node) error {
	f, err := os.CreateTemp(pathDir(path), ".helmdex-values-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	enc := yaml.NewEncoder(f)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		_ = enc.Close()
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := enc.Close(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Chmod(tmp, 0o644); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

func pathDir(p string) string {
	i := len(p) - 1
	for i >= 0 && !os.IsPathSeparator(p[i]) {
		i--
	}
	if i <= 0 {
		return "."
	}
	return p[:i]
}

func setNodeAt(root *yaml.Node, p Path, v any) error {
	cur := root
	for i, part := range p {
		last := i == len(p)-1

		if part.index != nil {
			if cur.Kind != yaml.SequenceNode {
				if len(cur.Content) == 0 && cur.Kind == yaml.MappingNode {
					// Empty map created on the way: turn it into a sequence.
					cur.Kind = yaml.SequenceNode
					cur.Tag = "!!seq"
				} else {
					return fmt.Errorf("element %d is not a sequence", i)
				}
			}
			idx := *part.index
			if idx < 0 {
				return fmt.Errorf("negative index %d", idx)
			}
			for len(cur.Content) <= idx {
				cur.Content = append(cur.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"})
			}
			if last {
				n := &yaml.Node{}
				if err := n.Encode(v); err != nil {
					return err
				}
				cur.Content[idx] = n
				return nil
			}
			next := cur.Content[idx]
			if next.Kind == yaml.ScalarNode && next.Tag == "!!null" {
				next = containerFor(p[i+1])
				cur.Content[idx] = next
			}
			cur = next
			continue
		}

		if cur.Kind != yaml.MappingNode {
			return fmt.Errorf("element %q is not a map", part.key)
		}
		found := -1
		for j := 0; j+1 < len(cur.Content); j += 2 {
			if cur.Content[j].Value == part.key {
				found = j
				break
			}
		}
		if last {
			if v == nil {
				if found >= 0 {
					cur.Content = append(cur.Content[:found], cur.Content[found+2:]...)
				}
				return nil
			}
			n := &yaml.Node{}
			if err := n.Encode(v); err != nil {
				return err
			}
			if found >= 0 {
				// Keep the key node (and its comments); swap the value only.
				cur.Content[found+1] = n
			} else {
				cur.Content = append(cur.Content,
					&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: part.key}, n)
			}
			return nil
		}
		if found >= 0 {
			next := cur.Content[found+1]
			if next.Kind == yaml.ScalarNode && next.Tag == "!!null" {
				next = containerFor(p[i+1])
				cur.Content[found+1] = next
			}
			cur = next
			continue
		}
		next := containerFor(p[i+1])
		cur.Content = append(cur.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: part.key}, next)
		cur = next
	}
	return nil
}

func containerFor(next yamlPathPart) *yaml.Node {
	if next.index != nil {
		return &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	}
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
}

func renderPath(p Path) string {
	s := "$"
	for _, part := range p {
		if part.index != nil {
			s += fmt.Sprintf("[%d]", *part.index)
			continue
		}
		s += "." + part.key
	}
	return s
}
