package values

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
