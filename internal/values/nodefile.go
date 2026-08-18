package values

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
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
		// Reject multi-document streams: a single-Node unmarshal keeps only
		// the first doc, so re-encoding would silently drop the rest. Fail
		// loudly rather than corrupt the file.
		dec := yaml.NewDecoder(bytes.NewReader(b))
		var probe yaml.Node
		if err := dec.Decode(&probe); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		var extra yaml.Node
		if err := dec.Decode(&extra); err == nil {
			return nil, fmt.Errorf("%s contains multiple YAML documents; helmdex does not edit multi-document files in place", path)
		} else if err != io.EOF {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		doc = probe
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

	// buf is the source of truth for content. restoreFormatting only ever
	// improves the *diff* (blank lines, comment alignment) of an unchanged
	// line — it must never change what the file parses to.
	out := buf.Bytes()
	if orig, err := os.ReadFile(path); err == nil {
		// The encoder emits LF; if the file was CRLF, work in LF internally
		// (so line matching is consistent) and convert the whole result back
		// to CRLF at the end. Otherwise unchanged CRLF lines wouldn't match
		// the LF re-encode and the diff would explode.
		crlf := bytes.Contains(orig, []byte("\r\n"))
		origLF := bytes.ReplaceAll(orig, []byte("\r\n"), []byte("\n"))
		reconciled := restoreFormatting(origLF, out)
		// Safety net: the line-level reconstruction can, on adversarial
		// inputs (block scalars whose content lines look like comments,
		// whitespace runs inside literal blocks), produce a file that no
		// longer parses to the same value — or does not parse at all. Verify
		// and fall back to the raw encoder output rather than corrupt the
		// file. Correct-but-uglier beats broken-but-pretty.
		if yamlSemanticEqual(out, reconciled) {
			out = reconciled
		}
		if crlf {
			out = bytes.ReplaceAll(out, []byte("\n"), []byte("\r\n"))
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	// Resolve symlinks so we edit the file the link points at (a common
	// shared-values gitops pattern) instead of replacing the link with a
	// regular file. Preserve the target's existing mode rather than forcing
	// 0644, so a 0600 file stays 0600.
	target := path
	mode := os.FileMode(0o644)
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		target = resolved
	}
	if st, err := os.Stat(target); err == nil {
		mode = st.Mode().Perm()
	}

	f, err := os.CreateTemp(pathDir(target), ".helmdex-values-*")
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
	if err := os.Chmod(tmp, mode); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, target)
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

// normalizeYAMLLine produces the matching key for a line. Leading
// indentation is dropped: yaml.v3 re-indents sequence items (and their
// continuations) relative to a hand-written "compact" list style, and
// trailing-comment column alignment is collapsed on re-encode. Matched
// lines are re-emitted with their ORIGINAL text (so original indentation
// and alignment survive), and yamlSemanticEqual rejects any reconstruction
// whose parsed value differs — so ignoring indentation here can only ever
// improve the diff, never change the file's meaning.
func normalizeYAMLLine(l string) string {
	s := strings.TrimLeft(l, " \t")
	if strings.HasPrefix(s, "#") {
		return s
	}
	if i := strings.Index(s, " #"); i >= 0 {
		code := strings.TrimRight(s[:i], " \t")
		return normalizeFlowSpacing(code) + " " + strings.TrimLeft(s[i:], " \t")
	}
	return normalizeFlowSpacing(s)
}

// normalizeFlowSpacing collapses padding inside flow collections, so a
// hand-written `[ ]` or `{ a: 1 }` still matches the encoder's `[]` and
// `{a: 1}`.
//
// Without it those lines read as changed, and a changed line is re-emitted
// from the encoder — losing not only its own spacing but the blank lines that
// preceded it. Like indentation above, this only widens matching: a matched
// line is re-emitted with its ORIGINAL text, and yamlSemanticEqual rejects any
// reconstruction whose parsed value differs.
func normalizeFlowSpacing(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != ' ' {
			b.WriteByte(c)
			continue
		}
		// Drop a run of spaces that touches a flow delimiter on either side.
		j := i
		for j < len(s) && s[j] == ' ' {
			j++
		}
		prev := byte(0)
		if b.Len() > 0 {
			prev = b.String()[b.Len()-1]
		}
		next := byte(0)
		if j < len(s) {
			next = s[j]
		}
		if !isFlowDelim(prev) && !isFlowDelim(next) {
			b.WriteString(s[i:j])
		}
		i = j - 1
	}
	return b.String()
}

func isFlowDelim(c byte) bool {
	switch c {
	case '[', ']', '{', '}', ',':
		return true
	}
	return false
}

// yamlSemanticEqual reports whether two YAML byte streams decode to the same
// value(s). Comments, blank lines and indentation style don't affect the
// result — only content does. A parse failure on either side ⇒ not equal.
func yamlSemanticEqual(a, b []byte) bool {
	decode := func(data []byte) ([]any, bool) {
		dec := yaml.NewDecoder(bytes.NewReader(data))
		var docs []any
		for {
			var v any
			err := dec.Decode(&v)
			if err == io.EOF {
				return docs, true
			}
			if err != nil {
				return nil, false
			}
			docs = append(docs, v)
		}
	}
	da, oka := decode(a)
	db, okb := decode(b)
	if !oka || !okb {
		return false
	}
	return reflect.DeepEqual(da, db)
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
