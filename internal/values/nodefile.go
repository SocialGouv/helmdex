package values

import (
	"bytes"
	"fmt"
	"os"
	"strings"

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
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		_ = enc.Close()
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}

	out := buf.Bytes()
	// yaml.v3 drops blank separator lines and comment alignment on
	// re-encode; restore the original text of unchanged lines so user-owned
	// files keep minimal diffs.
	if orig, err := os.ReadFile(path); err == nil {
		out = restoreFormatting(orig, out)
	} else if !os.IsNotExist(err) {
		return err
	}

	f, err := os.CreateTemp(pathDir(path), ".helmdex-values-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(out); err != nil {
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

// restoreFormatting reconciles a YAML re-encode with the original text:
// lines that are semantically unchanged (matched by longest-common-
// subsequence over normalized content) are emitted with their original
// formatting — blank separator lines and aligned trailing comments — while
// genuinely edited lines keep the new encoding.
func restoreFormatting(orig, out []byte) []byte {
	o := strings.Split(strings.TrimRight(string(orig), "\n"), "\n")
	n := []string{}
	// Blank lines in the re-encode are encoder separators; drop them and
	// trust the original's blanks (restored below) instead, so they don't
	// double up.
	for _, l := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if strings.TrimSpace(l) == "" {
			continue
		}
		n = append(n, l)
	}

	// Original lines with blanks removed, remembering how many blanks
	// precede each kept line.
	type oline struct {
		text   string
		norm   string
		blanks int
	}
	kept := make([]oline, 0, len(o))
	blanks := 0
	for _, l := range o {
		if strings.TrimSpace(l) == "" {
			blanks++
			continue
		}
		kept = append(kept, oline{text: l, norm: normalizeYAMLLine(l), blanks: blanks})
		blanks = 0
	}
	norm := make([]string, len(n))
	for j, l := range n {
		norm[j] = normalizeYAMLLine(l)
	}

	// LCS table between kept original lines and new lines.
	// Files are small (values files); O(n*m) is fine.
	lo, ln := len(kept), len(n)
	dp := make([][]int, lo+1)
	for i := range dp {
		dp[i] = make([]int, ln+1)
	}
	for i := lo - 1; i >= 0; i-- {
		for j := ln - 1; j >= 0; j-- {
			if kept[i].norm == norm[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	var b strings.Builder
	i, j := 0, 0
	for j < ln {
		if i < lo && kept[i].norm == norm[j] && dp[i][j] == dp[i+1][j+1]+1 {
			for k := 0; k < kept[i].blanks; k++ {
				b.WriteByte('\n')
			}
			// Unchanged line: keep its original formatting.
			b.WriteString(kept[i].text)
			b.WriteByte('\n')
			i++
			j++
			continue
		}
		if i < lo && dp[i+1][j] >= dp[i][j+1] {
			i++ // original line deleted by the edit
			continue
		}
		b.WriteString(n[j]) // new/changed line
		b.WriteByte('\n')
		j++
	}
	return []byte(b.String())
}

// normalizeYAMLLine produces the matching key for a line: comment-only lines
// compare by their trimmed text (re-encode re-indents them), other lines by
// collapsing the whitespace run before a trailing comment (re-encode drops
// column alignment). Content — including indentation of code — stays
// significant.
func normalizeYAMLLine(l string) string {
	trimmed := strings.TrimSpace(l)
	if strings.HasPrefix(trimmed, "#") {
		return trimmed
	}
	if i := strings.Index(l, " #"); i >= 0 {
		code := strings.TrimRight(l[:i], " \t")
		return code + " " + strings.TrimLeft(l[i:], " \t")
	}
	return l
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
				// Keep the key node (and its comments); swap the value only,
				// carrying over comments attached to the old value node.
				old := cur.Content[found+1]
				n.HeadComment, n.LineComment, n.FootComment = old.HeadComment, old.LineComment, old.FootComment
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
