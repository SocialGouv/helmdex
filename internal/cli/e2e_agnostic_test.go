package cli

import (
	"crypto/sha256"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"helmdex/internal/testutil"
)

// A helmdex-agnostic repo (no helmdex.yaml) must work without helmdex adding
// or rewriting a single file it does not own.

func TestCLI_Agnostic_DiscoversLayoutAndStaysOutOfTheRepo(t *testing.T) {
	r := newCLIRepo(t, testutil.RepoOpts{Agnostic: true})

	fingerprint := hashTree(t, r.Root)

	out := r.run("instance", "list")
	if !strings.Contains(out, "demo-preprod") {
		t.Fatalf("layout discovery did not find the instance:\n%s", out)
	}

	out = r.run("instance", "templates", "list")
	if !strings.Contains(out, "demo-review") {
		t.Fatalf("blueprint discovery failed:\n%s", out)
	}

	// Read-only commands must leave the repo byte-identical.
	if got := hashTree(t, r.Root); !sameTree(fingerprint, got) {
		t.Fatalf("read-only commands modified the repo\nbefore: %v\nafter: %v", fingerprint, got)
	}
	if r.Exists(t, ".helmdex") {
		t.Fatal("helmdex wrote in-repo state into an agnostic repo")
	}
}

func TestCLI_Agnostic_StateLivesOutsideTheRepo(t *testing.T) {
	r := newCLIRepo(t, testutil.RepoOpts{Agnostic: true})

	// An operation that needs state: relock writes helm envs and caches.
	r.run("instance", "apply", "demo-preprod")

	if r.Exists(t, ".helmdex") {
		t.Fatal("apply wrote in-repo state into an agnostic repo")
	}
	cacheDir := os.Getenv("HELMDEX_CACHE_DIR")
	if cacheDir == "" {
		t.Fatal("HELMDEX_CACHE_DIR not isolated by the test harness")
	}
	entries, err := os.ReadDir(filepath.Join(cacheDir, "repos"))
	if err != nil || len(entries) == 0 {
		t.Fatalf("expected external state under %s/repos: %v", cacheDir, err)
	}
}

func TestCLI_Agnostic_ApplyReconcilesWithoutGeneratingValues(t *testing.T) {
	r := newCLIRepo(t, testutil.RepoOpts{Agnostic: true})

	valuesBefore := r.Read(t, "apps", "demo-preprod", "values.yaml")
	deployBefore := r.Read(t, "apps", "demo-preprod", "values.deploy.yaml")
	extraBefore := r.Read(t, "apps", "demo-preprod", "atlas-env.yaml")

	r.run("instance", "apply", "demo-preprod")

	// The chart is locked and vendored...
	if !r.Exists(t, "apps", "demo-preprod", "Chart.lock") {
		t.Fatal("apply must lock the declared OCI dependency")
	}
	// ...but no user-owned file is touched.
	if got := r.Read(t, "apps", "demo-preprod", "values.yaml"); got != valuesBefore {
		t.Fatalf("apply regenerated a user-owned values.yaml:\n%s", got)
	}
	if got := r.Read(t, "apps", "demo-preprod", "values.deploy.yaml"); got != deployBefore {
		t.Fatalf("apply rewrote values.deploy.yaml:\n%s", got)
	}
	if got := r.Read(t, "apps", "demo-preprod", "atlas-env.yaml"); got != extraBefore {
		t.Fatalf("apply rewrote a non-helm file:\n%s", got)
	}
	if r.Exists(t, "apps", "demo-preprod", "values.instance.yaml") {
		t.Fatal("apply created a managed layer in a direct-mode instance")
	}
}

func TestCLI_Agnostic_CreateStaysDirectMode(t *testing.T) {
	r := newCLIRepo(t, testutil.RepoOpts{Agnostic: true})

	r.run("instance", "create", "demo-review-42")

	if !r.Exists(t, "apps", "demo-review-42", "values.yaml") {
		t.Fatal("a direct-mode instance owns values.yaml")
	}
	if r.Exists(t, "apps", "demo-review-42", "values.instance.yaml") {
		t.Fatal("an agnostic repo must not get managed layer files")
	}
	if r.Exists(t, "helmdex.yaml") {
		t.Fatal("creating an instance must not opt the repo in")
	}
}

func TestCLI_Agnostic_CreateFromTemplate(t *testing.T) {
	r := newCLIRepo(t, testutil.RepoOpts{Agnostic: true})

	r.run("instance", "create", "demo-review-7", "--from-template", "demo-review")

	chart := r.Read(t, "apps", "demo-review-7", "Chart.yaml")
	if !strings.Contains(chart, "name: demo-review-7") {
		t.Fatalf("the copied chart was not renamed:\n%s", chart)
	}
	if !r.Exists(t, "apps", "demo-review-7", "templates", "netpol.yaml") {
		t.Fatal("blueprint template files were not copied")
	}
	if r.Exists(t, "apps", "demo-review-7", "values.instance.yaml") {
		t.Fatal("instantiating a blueprint must not add helmdex-specific files")
	}
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
		sum := sha256.Sum256(b)
		out[rel] = string(sum[:])
		return nil
	})
	if err != nil {
		t.Fatalf("hash tree %s: %v", root, err)
	}
	return out
}

func sameTree(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
