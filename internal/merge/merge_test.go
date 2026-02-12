package merge

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func mustNode(t *testing.T, s string) *yaml.Node {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(s), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		return doc.Content[0]
	}
	return &doc
}

func toYAML(t *testing.T, n *yaml.Node) string {
	t.Helper()
	doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{n}}
	b, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

func TestDeepMerge_ScalarsLastWins(t *testing.T) {
	a := mustNode(t, "a: 1\n")
	b := mustNode(t, "a: 2\n")

	out, err := DeepMerge(a, b)
	if err != nil {
		t.Fatalf("DeepMerge: %v", err)
	}

	got := toYAML(t, out)
	want := "a: 2\n"
	if got != want {
		t.Fatalf("want\n%s\ngot\n%s", want, got)
	}
}

func TestDeepMerge_MapsRecursive(t *testing.T) {
	a := mustNode(t, "a:\n  b: 1\n  c: 2\n")
	b := mustNode(t, "a:\n  b: 9\n  d: 3\n")

	out, err := DeepMerge(a, b)
	if err != nil {
		t.Fatalf("DeepMerge: %v", err)
	}

	// Compare structurally to avoid indentation differences.
	var gotObj any
	if err := yaml.Unmarshal([]byte(toYAML(t, out)), &gotObj); err != nil {
		t.Fatalf("unmarshal got: %v", err)
	}
	var wantObj any
	if err := yaml.Unmarshal([]byte("a:\n  b: 9\n  c: 2\n  d: 3\n"), &wantObj); err != nil {
		t.Fatalf("unmarshal want: %v", err)
	}
	if !equalYAML(gotObj, wantObj) {
		t.Fatalf("objects differ\nwant: %#v\ngot:  %#v", wantObj, gotObj)
	}
}

func TestDeepMerge_ArraysReplaced(t *testing.T) {
	a := mustNode(t, "a: [1,2,3]\n")
	b := mustNode(t, "a: [9]\n")

	out, err := DeepMerge(a, b)
	if err != nil {
		t.Fatalf("DeepMerge: %v", err)
	}

	var gotObj any
	if err := yaml.Unmarshal([]byte(toYAML(t, out)), &gotObj); err != nil {
		t.Fatalf("unmarshal got: %v", err)
	}
	var wantObj any
	if err := yaml.Unmarshal([]byte("a: [9]\n"), &wantObj); err != nil {
		t.Fatalf("unmarshal want: %v", err)
	}
	if !equalYAML(gotObj, wantObj) {
		t.Fatalf("objects differ\nwant: %#v\ngot:  %#v", wantObj, gotObj)
	}
}

func TestDeepMerge_TypeMismatch_BWins(t *testing.T) {
	a := mustNode(t, "a: {b: 1}\n")
	b := mustNode(t, "a: 2\n")

	out, err := DeepMerge(a, b)
	if err != nil {
		t.Fatalf("DeepMerge: %v", err)
	}

	got := toYAML(t, out)
	want := "a: 2\n"
	if got != want {
		t.Fatalf("want\n%s\ngot\n%s", want, got)
	}
}

func TestDeepMerge_StableKeyOrder(t *testing.T) {
	a := mustNode(t, "a: 1\nb: 2\n")
	b := mustNode(t, "c: 3\na: 9\n")

	out, err := DeepMerge(a, b)
	if err != nil {
		t.Fatalf("DeepMerge: %v", err)
	}

	got := toYAML(t, out)
	// Keys from a first in their order, then new keys from b.
	want := "a: 9\nb: 2\nc: 3\n"
	if got != want {
		t.Fatalf("want\n%s\ngot\n%s", want, got)
	}
}

func equalYAML(a, b any) bool {
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, v := range av {
			if !equalYAML(v, bv[k]) {
				return false
			}
		}
		return true
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !equalYAML(av[i], bv[i]) {
				return false
			}
		}
		return true
	default:
		return av == b
	}
}
