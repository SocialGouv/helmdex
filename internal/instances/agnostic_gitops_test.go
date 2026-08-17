package instances

import (
	"crypto/sha256"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"helmdex/internal/config"
	"helmdex/internal/presets"
	"helmdex/internal/values"
	"helmdex/internal/yamlchart"
)

// The agnostic-gitops fixture mirrors an atlas-v2 style gitops repo:
// apps/<env>/ wrapper charts with hand-written values.yaml + values.deploy.yaml
// + extra metadata files, and templates/<blueprint>/ review gabarits. Repos
// like this never opt into helmdex; helmdex must adapt without adding or
// rewriting a single file it does not own.

func copyFixtureRepo(t *testing.T) string {
	t.Helper()
	src, err := filepath.Abs(filepath.Join("..", "..", "fixtures", "agnostic-gitops"))
	if err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	if err := copyTree(src, dst); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	// Isolate helmdex state for the test.
	t.Setenv("HELMDEX_CACHE_DIR", t.TempDir())
	return dst
}

func hashTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		h := sha256.Sum256(b)
		out[rel] = string(h[:])
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestAgnosticRepo_DiscoveryAndListing(t *testing.T) {
	repo := copyFixtureRepo(t)

	layout := DiscoverLayout(repo, "", "")
	if layout.AppsDir != "apps" {
		t.Fatalf("AppsDir = %q, want apps", layout.AppsDir)
	}
	if layout.TemplatesDir != "templates" {
		t.Fatalf("TemplatesDir = %q, want templates", layout.TemplatesDir)
	}

	insts, err := List(repo, layout.AppsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(insts) != 1 || insts[0].Name != "demo-preprod" {
		t.Fatalf("instances = %+v", insts)
	}

	tmpls, err := ListTemplates(repo, layout.TemplatesDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(tmpls) != 1 || tmpls[0].Name != "demo-review" {
		t.Fatalf("templates = %+v", tmpls)
	}
}

func TestAgnosticRepo_ReadOnlyOpsAreByteIdentical(t *testing.T) {
	repo := copyFixtureRepo(t)
	before := hashTree(t, repo)

	layout := DiscoverLayout(repo, "", "")
	insts, err := List(repo, layout.AppsDir)
	if err != nil {
		t.Fatal(err)
	}
	inst := insts[0]

	// Direct mode: generation and presets import must be strict no-ops.
	if values.IsManaged(inst.Path) {
		t.Fatalf("fixture instance must be direct-mode")
	}
	if err := values.GenerateIfManaged(inst.Path); err != nil {
		t.Fatal(err)
	}
	c, err := yamlchart.ReadChart(filepath.Join(inst.Path, "Chart.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig()
	if _, err := presets.Import(presets.ImportParams{RepoRoot: repo, InstancePath: inst.Path, Config: cfg, Dependencies: c.Dependencies}); err != nil {
		t.Fatal(err)
	}

	after := hashTree(t, repo)
	if len(before) != len(after) {
		t.Fatalf("file count changed: %d -> %d", len(before), len(after))
	}
	for rel, h := range before {
		if after[rel] != h {
			t.Fatalf("file %s was modified", rel)
		}
	}
}

func TestAgnosticRepo_DirectEditPreservesComments(t *testing.T) {
	repo := copyFixtureRepo(t)
	inst := filepath.Join(repo, "apps", "demo-preprod")

	if got := values.EditFileName(inst); got != "values.yaml" {
		t.Fatalf("EditFileName = %q, want values.yaml", got)
	}

	p, err := values.ParsePath("$.demo.replicaCount")
	if err != nil {
		t.Fatal(err)
	}
	if err := values.SetInFile(values.EditFilePath(inst), p, 3); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(filepath.Join(inst, "values.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		"# Wrapper defaults (hand-written, user-owned — helmdex must never regenerate this).",
		"# keep small in preprod",
		"# NetworkPolicies toggle.",
		"replicaCount: 3",
		"registry.example.invalid/org/demo",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("values.yaml lost %q:\n%s", want, s)
		}
	}
}

func TestAgnosticRepo_CreateFromTemplate(t *testing.T) {
	repo := copyFixtureRepo(t)
	before := hashTree(t, repo)

	inst, err := CreateFromTemplate(repo, "apps", "templates", "demo-review", "demo-review-featx")
	if err != nil {
		t.Fatal(err)
	}
	c, err := yamlchart.ReadChart(filepath.Join(inst.Path, "Chart.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "demo-review-featx" {
		t.Fatalf("chart name = %q", c.Name)
	}
	if _, err := os.Stat(filepath.Join(inst.Path, "templates", "netpol.yaml")); err != nil {
		t.Fatalf("blueprint templates not copied: %v", err)
	}

	// Pre-existing files stay byte-identical.
	after := hashTree(t, repo)
	for rel, h := range before {
		if after[rel] != h {
			t.Fatalf("pre-existing file %s was modified", rel)
		}
	}
}

func TestAgnosticRepo_DirectCreateWritesOnlyChartAndValues(t *testing.T) {
	repo := copyFixtureRepo(t)

	inst, err := Create(repo, "apps", "fresh", false)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(inst.Path)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 2 || names[0] != "Chart.yaml" || names[1] != "values.yaml" {
		t.Fatalf("direct create produced %v, want [Chart.yaml values.yaml]", names)
	}
	if values.IsManaged(inst.Path) {
		t.Fatalf("direct create must not produce a managed instance")
	}
}
