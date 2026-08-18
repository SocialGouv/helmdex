package values

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func yamlUnmarshal(b []byte, v any) error { return yaml.Unmarshal(b, v) }

const gitopsStyleValues = `# Wrapper defaults (hand-written).

# Postgres cluster.
cnpg:
  name: demo-db
  instances: 1 # keep small
  storageSize: 5Gi

externalSecrets:
  appPath: app              # aligned comment
  s3Path: ""                # another aligned comment
`

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "values.yaml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func mustPath(t *testing.T, s string) Path {
	t.Helper()
	p, err := ParsePath(s)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSetInFile_MinimalDiff(t *testing.T) {
	p := writeTemp(t, gitopsStyleValues)

	if err := SetInFile(p, mustPath(t, "$.cnpg.instances"), 2); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	want := strings.Replace(gitopsStyleValues, "instances: 1 # keep small", "instances: 2 # keep small", 1)
	if got != want {
		t.Fatalf("diff not minimal.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestSetInFile_NumbersStayNumbers(t *testing.T) {
	p := writeTemp(t, "a: 1\n")
	if err := SetInFile(p, mustPath(t, "$.a"), int64(2)); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if string(b) != "a: 2\n" {
		t.Fatalf("got %q", string(b))
	}
}

func TestSetInFile_CreatesMissingPathAndFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "values.yaml")
	if err := SetInFile(p, mustPath(t, "$.a.b[1].c"), "x"); err != nil {
		t.Fatal(err)
	}
	v, ok, err := GetInFile(p, mustPath(t, "$.a.b[1].c"))
	if err != nil || !ok || v != "x" {
		t.Fatalf("v=%v ok=%v err=%v", v, ok, err)
	}
	// Index 0 padded with null.
	v0, ok, err := GetInFile(p, mustPath(t, "$.a.b[0]"))
	if err != nil || !ok || v0 != nil {
		t.Fatalf("v0=%v ok=%v err=%v", v0, ok, err)
	}
}

func TestSetInFile_DeleteKey(t *testing.T) {
	p := writeTemp(t, gitopsStyleValues)
	if err := SetInFile(p, mustPath(t, "$.externalSecrets.s3Path"), nil); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	s := string(b)
	if strings.Contains(s, "s3Path") {
		t.Fatalf("s3Path not deleted:\n%s", s)
	}
	// Untouched aligned comment survives.
	if !strings.Contains(s, "appPath: app              # aligned comment") {
		t.Fatalf("untouched formatting lost:\n%s", s)
	}
}

func TestSetInFile_AppendsNewKey(t *testing.T) {
	p := writeTemp(t, gitopsStyleValues)
	if err := SetInFile(p, mustPath(t, "$.cnpg.owner"), "demo"); err != nil {
		t.Fatal(err)
	}
	v, ok, err := GetInFile(p, mustPath(t, "$.cnpg.owner"))
	if err != nil || !ok || v != "demo" {
		t.Fatalf("v=%v ok=%v err=%v", v, ok, err)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "# Wrapper defaults (hand-written).\n\n# Postgres cluster.") {
		t.Fatalf("blank line between comment blocks lost:\n%s", string(b))
	}
}

// --- adversarial regressions (found by review, must stay fixed) ---

func TestSetInFile_BlockScalarCommentNoCorruption(t *testing.T) {
	// A block-scalar value whose content lines look like comments must not
	// be de-indented into invalid YAML by the formatting reconstruction.
	p := writeTemp(t, "zz: 1\n# run\naa: 2\n")
	if err := SetInFile(p, mustPath(t, "$.zz"), "# run\nx\n"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	var got map[string]any
	if err := yamlUnmarshal(b, &got); err != nil {
		t.Fatalf("result does not parse: %v\n%s", err, string(b))
	}
	if got["zz"] != "# run\nx\n" {
		t.Fatalf("zz = %q, want %q", got["zz"], "# run\nx\n")
	}
	if got["aa"] != 2 {
		t.Fatalf("aa lost/changed: %v", got["aa"])
	}
}

func TestSetInFile_BlockScalarInlineHashNotDiscarded(t *testing.T) {
	// The requested value must actually be written even when it contains a
	// literal ` #` inside a block scalar.
	p := writeTemp(t, "a: |\n  echo hi   # x\nb: 1\n")
	if err := SetInFile(p, mustPath(t, "$.a"), "echo hi # x\n"); err != nil {
		t.Fatal(err)
	}
	v, ok, err := GetInFile(p, mustPath(t, "$.a"))
	if err != nil || !ok {
		t.Fatalf("get: %v ok=%v", err, ok)
	}
	if v != "echo hi # x\n" {
		t.Fatalf("value discarded: got %q", v)
	}
}

func TestSetInFile_MultiDocRejected(t *testing.T) {
	p := writeTemp(t, "demo:\n  n: 1\n---\nkeep: me\n")
	err := SetInFile(p, mustPath(t, "$.demo.n"), 2)
	if err == nil {
		t.Fatal("expected an error on a multi-document file, got nil")
	}
	// The original file must be untouched.
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "keep: me") {
		t.Fatalf("multi-doc file was mutated:\n%s", string(b))
	}
}

func TestSetInFile_SequenceNotReindented(t *testing.T) {
	// Compact list style ("- " at parent indent) must survive an edit to an
	// unrelated scalar — a 1-value change stays a 1-line diff.
	orig := "top:\n  items:\n  - name: A\n    value: \"1\"\n  - name: B\nscalar: 1\n"
	p := writeTemp(t, orig)
	if err := SetInFile(p, mustPath(t, "$.scalar"), 2); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	want := strings.Replace(orig, "scalar: 1", "scalar: 2", 1)
	if string(b) != want {
		t.Fatalf("sequence reindented.\n--- got ---\n%s\n--- want ---\n%s", string(b), want)
	}
}

func TestSetInFile_CRLFPreserved(t *testing.T) {
	p := writeTemp(t, "a: 1\r\nb: 2\r\n# c\r\nc: 3\r\n")
	if err := SetInFile(p, mustPath(t, "$.a"), 9); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if strings.Contains(strings.ReplaceAll(string(b), "\r\n", ""), "\n") {
		t.Fatalf("mixed line endings after edit:\n%q", string(b))
	}
}

func TestSetInFile_PreservesSymlinkAndMode(t *testing.T) {
	dir := t.TempDir()
	realFile := filepath.Join(dir, "shared.yaml")
	if err := os.WriteFile(realFile, []byte("a: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "values.yaml")
	if err := os.Symlink("shared.yaml", link); err != nil {
		t.Fatal(err)
	}
	if err := SetInFile(link, mustPath(t, "$.a"), 2); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink was replaced by a regular file")
	}
	st, _ := os.Stat(realFile)
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("mode changed to %o, want 600", st.Mode().Perm())
	}
	v, _, _ := GetInFile(realFile, mustPath(t, "$.a"))
	if v != 2 {
		t.Fatalf("edit did not reach the symlink target: %v", v)
	}
}

// A hand-written values file mixes blank lines with padded flow collections.
// Editing one key must not reflow the rest: in a helmdex-agnostic repo every
// stray line is a diff in someone's GitOps history.
func TestSetInFile_BlankLinesAndFlowPaddingSurvive(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "values.yaml")
	const orig = `# Wrapper defaults.
demo:
  replicaCount: 1 # keep small

  image:
    repository: registry.example.invalid/org/demo

  tolerations: [ ]

# Toggle.
networkPolicy:
  enabled: true
`
	if err := os.WriteFile(p, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}

	path, err := ParsePath("$.demo.replicaCount")
	if err != nil {
		t.Fatal(err)
	}
	if err := SetInFile(p, path, 3); err != nil {
		t.Fatalf("SetInFile: %v", err)
	}

	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)

	if !strings.Contains(got, "replicaCount: 3 # keep small") {
		t.Fatalf("edit or its trailing comment lost:\n%s", got)
	}
	if !strings.Contains(got, "tolerations: [ ]") {
		t.Fatalf("flow-collection padding was reflowed:\n%s", got)
	}
	before, after := strings.Split(orig, "\n"), strings.Split(got, "\n")
	if len(before) != len(after) {
		t.Fatalf("line count changed (%d -> %d), blank lines were dropped:\n%s", len(before), len(after), got)
	}
	changed := 0
	for i := range before {
		if before[i] != after[i] {
			changed++
		}
	}
	if changed != 1 {
		t.Fatalf("expected exactly one changed line, got %d:\n%s", changed, got)
	}
}
