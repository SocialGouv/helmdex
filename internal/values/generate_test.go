package values

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

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

func writeFile(t *testing.T, dir, name, contents string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func readFile(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

func TestGenerateMergedValues_Precedence(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "values.default.yaml", "app:\n  replicas: 1\n  image: default\n")
	writeFile(t, dir, "values.platform.yaml", "app:\n  replicas: 2\n")
	writeFile(t, dir, "values.set.prod.yaml", "app:\n  image: prod\n")
	writeFile(t, dir, "values.instance.yaml", "app:\n  replicas: 3\n")

	if err := GenerateMergedValues(dir); err != nil {
		t.Fatalf("GenerateMergedValues: %v", err)
	}

	got := readFile(t, dir, "values.yaml")
	// Compare structurally (yaml.Marshal doesn't guarantee key order).
	var gotObj any
	if err := yaml.Unmarshal([]byte(got), &gotObj); err != nil {
		t.Fatalf("unmarshal got: %v", err)
	}
	var wantObj any
	if err := yaml.Unmarshal([]byte("app:\n  image: prod\n  replicas: 3\n"), &wantObj); err != nil {
		t.Fatalf("unmarshal want: %v", err)
	}
	if !equalYAML(gotObj, wantObj) {
		t.Fatalf("objects differ\nwant: %#v\ngot:  %#v", wantObj, gotObj)
	}
}

func TestGenerateMergedValues_MissingOptionalLayers_OK(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "values.instance.yaml", "{}\n")

	if err := GenerateMergedValues(dir); err != nil {
		t.Fatalf("GenerateMergedValues: %v", err)
	}

	got := readFile(t, dir, "values.yaml")
	var gotObj any
	if err := yaml.Unmarshal([]byte(got), &gotObj); err != nil {
		t.Fatalf("unmarshal got: %v", err)
	}
	var wantObj any
	if err := yaml.Unmarshal([]byte("{}\n"), &wantObj); err != nil {
		t.Fatalf("unmarshal want: %v", err)
	}
	if !equalYAML(gotObj, wantObj) {
		t.Fatalf("objects differ\nwant: %#v\ngot:  %#v", wantObj, gotObj)
	}
}

func TestGenerateMergedValues_MissingInstanceFile_Fails(t *testing.T) {
	dir := t.TempDir()
	if err := GenerateMergedValues(dir); err == nil {
		t.Fatalf("expected error")
	}
}
