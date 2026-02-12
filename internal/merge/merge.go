package merge

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// DeepMerge merges b into a, returning a new node.
// Semantics:
// - mapping nodes: recursively merge by key (b overrides a)
// - sequences: replaced (b wins)
// - scalars: replaced (b wins)
func DeepMerge(a, b *yaml.Node) (*yaml.Node, error) {
	if a == nil {
		return clone(b), nil
	}
	if b == nil {
		return clone(a), nil
	}

	// If types mismatch, b wins.
	if a.Kind != b.Kind {
		return clone(b), nil
	}

	switch a.Kind {
	case yaml.MappingNode:
		return mergeMaps(a, b)
	case yaml.SequenceNode:
		return clone(b), nil
	case yaml.ScalarNode:
		return clone(b), nil
	case yaml.DocumentNode:
		// Merge the first content element if present.
		if len(a.Content) == 0 {
			return clone(b), nil
		}
		if len(b.Content) == 0 {
			return clone(a), nil
		}
		m, err := DeepMerge(a.Content[0], b.Content[0])
		if err != nil {
			return nil, err
		}
		out := clone(a)
		out.Content = []*yaml.Node{m}
		return out, nil
	default:
		return clone(b), nil
	}
}

func mergeMaps(a, b *yaml.Node) (*yaml.Node, error) {
	if a.Kind != yaml.MappingNode || b.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("mergeMaps expects mapping nodes")
	}
	out := clone(a)
	out.Content = []*yaml.Node{}

	// Build index for a.
	idx := map[string]*yaml.Node{}
	for i := 0; i < len(a.Content); i += 2 {
		k := a.Content[i]
		v := a.Content[i+1]
		idx[k.Value] = v
	}

	// Collect keys in stable order: keys from a then new keys from b.
	seen := map[string]bool{}
	keys := []string{}
	for i := 0; i < len(a.Content); i += 2 {
		k := a.Content[i].Value
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	for i := 0; i < len(b.Content); i += 2 {
		k := b.Content[i].Value
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}

	for _, k := range keys {
		var av *yaml.Node
		if v, ok := idx[k]; ok {
			av = v
		}
		bv := findMapValue(b, k)
		mv, err := DeepMerge(av, bv)
		if err != nil {
			return nil, err
		}
		out.Content = append(out.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: k},
			mv,
		)
	}

	return out, nil
}

func findMapValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(m.Content); i += 2 {
		k := m.Content[i]
		v := m.Content[i+1]
		if k.Value == key {
			return v
		}
	}
	return nil
}

func clone(n *yaml.Node) *yaml.Node {
	if n == nil {
		return nil
	}
	out := *n
	out.Content = nil
	if len(n.Content) > 0 {
		out.Content = make([]*yaml.Node, 0, len(n.Content))
		for _, c := range n.Content {
			out.Content = append(out.Content, clone(c))
		}
	}
	return &out
}

