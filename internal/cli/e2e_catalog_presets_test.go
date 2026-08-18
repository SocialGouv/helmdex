package cli

import (
	"strings"
	"testing"

	"helmdex/internal/testutil"
)

func TestCLI_Catalog_SyncListGet(t *testing.T) {
	r := newCLIRepo(t, testutil.RepoOpts{SourceMode: testutil.SourceGit})

	// Before syncing, the local catalog cache is empty.
	if out := r.run("catalog", "list"); strings.Contains(out, "bitnami-nginx") {
		t.Fatalf("catalog listed entries before a sync:\n%s", out)
	}

	r.run("catalog", "sync")
	if !r.Exists(t, ".helmdex", "catalog", "Example.yaml") {
		t.Fatal("sync did not write the catalog snapshot")
	}

	out := r.run("catalog", "list")
	for _, want := range []string{"bitnami-postgresql-15.5.0", "bitnami-nginx-15.0.0"} {
		if !strings.Contains(out, want) {
			t.Fatalf("catalog list missing %q:\n%s", want, out)
		}
	}

	out = r.run("catalog", "get", "bitnami-nginx-15.0.0")
	for _, want := range []string{"nginx", "15.0.0"} {
		if !strings.Contains(out, want) {
			t.Fatalf("catalog get missing %q:\n%s", want, out)
		}
	}

	if out := r.run("catalog", "list", "--format", "table"); !strings.Contains(out, "bitnami-nginx-15.0.0") {
		t.Fatalf("table format:\n%s", out)
	}

	r.mustFail("catalog", "get", "does-not-exist")
}

func TestCLI_Catalog_SyncIsIdempotent(t *testing.T) {
	r := newCLIRepo(t, testutil.RepoOpts{SourceMode: testutil.SourceGit})

	r.run("catalog", "sync")
	first := r.Read(t, ".helmdex", "catalog", "Example.yaml")
	r.run("catalog", "sync")
	if second := r.Read(t, ".helmdex", "catalog", "Example.yaml"); second != first {
		t.Fatalf("a second sync changed the snapshot:\n%s\n---\n%s", first, second)
	}
}

func TestCLI_Catalog_SyncFailsLoudlyOnABadSource(t *testing.T) {
	r := newCLIRepo(t, testutil.RepoOpts{SourceMode: testutil.SourceGit})

	// Point the source at a path that is not a git repository.
	cfg := r.Read(t, "helmdex.yaml")
	r.Write(t, strings.Replace(cfg, r.Source, r.Path("absent"), 1), "helmdex.yaml")

	if msg := r.mustFail("catalog", "sync"); strings.TrimSpace(msg) == "" {
		t.Fatal("a failed sync must explain why")
	}
}

func TestCLI_Presets_ResolveAndResolveDep(t *testing.T) {
	r := newCLIRepo(t, testutil.RepoOpts{SourceMode: testutil.SourceGit})
	r.run("catalog", "sync")
	r.run("instance", "create", "alpha")
	r.run("instance", "dep", "add-from-catalog", "alpha", "--id", "bitnami-nginx-15.0.0")

	// The fixture publishes values.default / values.platform.eks /
	// values.set.* for nginx 15.0.0.
	out := r.run("instance", "presets", "resolve", "alpha")
	for _, want := range []string{"nginx", "default"} {
		if !strings.Contains(out, want) {
			t.Fatalf("presets resolve missing %q:\n%s", want, out)
		}
	}

	out = r.run("instance", "presets", "resolve-dep", "alpha", "nginx")
	if !strings.Contains(out, "eks") {
		t.Fatalf("resolve-dep did not report the platform layer:\n%s", out)
	}
	if !strings.Contains(out, "ha-production") || !strings.Contains(out, "dev") {
		t.Fatalf("resolve-dep did not report the available sets:\n%s", out)
	}

	r.mustFail("instance", "presets", "resolve-dep", "alpha", "ghost")
}

func TestCLI_Presets_ImportedLayersFeedGeneratedValues(t *testing.T) {
	r := newCLIRepo(t, testutil.RepoOpts{SourceMode: testutil.SourceGit})
	r.run("catalog", "sync")
	r.run("instance", "create", "alpha")
	r.run("instance", "dep", "add-from-catalog", "alpha", "--id", "bitnami-nginx-15.0.0", "--apply")

	// apply imports the remote preset layers into the instance...
	if !r.Exists(t, "apps", "alpha", "values.default.yaml") {
		t.Fatal("apply did not import the default preset layer")
	}
	if !r.Exists(t, "apps", "alpha", "values.platform.yaml") {
		t.Fatal("apply did not import the platform preset layer")
	}
	// ...and merges them into the generated output.
	merged := r.Read(t, "apps", "alpha", "values.yaml")
	if strings.TrimSpace(merged) == "" || strings.TrimSpace(merged) == "{}" {
		t.Fatalf("generated values.yaml is empty:\n%s", merged)
	}
}

// `cache clear` targets the Helm query caches, not the synced source clones —
// those are re-cloned by `catalog sync` and are cheap to keep.
func TestCLI_Cache_Clear(t *testing.T) {
	r := newCLIRepo(t, testutil.RepoOpts{SourceMode: testutil.SourceGit})
	r.run("catalog", "sync")
	r.run("instance", "create", "alpha")
	r.run("instance", "dep", "add", "alpha",
		"--repo", testChartRepo, "--name", "nginx", "--version", "15.0.0")
	// Inspecting a dependency populates the helm show cache.
	r.run("instance", "dep", "inspect", "values", "alpha", "nginx")

	if !r.Exists(t, ".helmdex", "cache", "helmshow") {
		t.Fatal("expected a populated helm show cache")
	}

	r.run("cache", "clear")
	if r.Exists(t, ".helmdex", "cache", "helmshow") {
		t.Fatal("cache clear did not remove the helm show cache")
	}
	if !r.Exists(t, ".helmdex", "cache", "Example") {
		t.Fatal("cache clear must not drop synced source clones")
	}

	// --helm additionally wipes the isolated Helm environments.
	if !r.Exists(t, ".helmdex", "helm") {
		t.Fatal("expected isolated helm envs to exist")
	}
	r.run("cache", "clear", "--helm")
	if r.Exists(t, ".helmdex", "helm") {
		t.Fatal("cache clear --helm did not remove the helm envs")
	}
}

func TestCLI_Init_WritesConfigOnce(t *testing.T) {
	r := newCLIRepo(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})

	// The harness already wrote a helmdex.yaml, so a plain init must refuse.
	r.mustFail("init")

	r.run("init", "--force")
	cfg := r.Read(t, "helmdex.yaml")
	if !strings.Contains(cfg, "kind: HelmdexConfig") {
		t.Fatalf("init did not write a valid config:\n%s", cfg)
	}
}
