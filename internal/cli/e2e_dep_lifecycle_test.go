package cli

import (
	"strings"
	"testing"

	"helmdex/internal/testutil"
)

const testChartRepo = "https://example.invalid/charts"

func TestCLI_Dep_AddListSetVersionUpgradeRemove(t *testing.T) {
	r := newCLIRepo(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	r.run("instance", "create", "alpha")

	r.run("instance", "dep", "add", "alpha",
		"--repo", testChartRepo, "--name", "postgresql", "--version", "15.4.0")

	chart := r.Read(t, "apps", "alpha", "Chart.yaml")
	if !strings.Contains(chart, "name: postgresql") || !strings.Contains(chart, "version: 15.4.0") {
		t.Fatalf("dependency not written:\n%s", chart)
	}

	out := r.run("instance", "dep", "list", "alpha")
	if !strings.Contains(out, "postgresql") {
		t.Fatalf("dep list:\n%s", out)
	}

	// versions lists what the registry publishes, pre-releases excluded.
	var versions []string
	r.runJSON(&versions, "instance", "dep", "versions", "alpha", "postgresql")
	if len(versions) == 0 {
		t.Fatal("no versions returned")
	}
	for _, v := range versions {
		if strings.Contains(v, "-rc") {
			t.Fatalf("pre-releases must not be listed: %v", versions)
		}
	}
	if !strings.Contains(strings.Join(versions, ","), "16.0.0") {
		t.Fatalf("versions = %v", versions)
	}

	// set-version validates against the registry by default.
	r.run("instance", "dep", "set-version", "alpha", "postgresql", "--version", "15.6.0")
	if chart := r.Read(t, "apps", "alpha", "Chart.yaml"); !strings.Contains(chart, "version: 15.6.0") {
		t.Fatalf("set-version not persisted:\n%s", chart)
	}
	if msg := r.mustFail("instance", "dep", "set-version", "alpha", "postgresql", "--version", "99.0.0"); !strings.Contains(msg, "99.0.0") {
		t.Fatalf("unexpected error: %s", msg)
	}
	if chart := r.Read(t, "apps", "alpha", "Chart.yaml"); strings.Contains(chart, "99.0.0") {
		t.Fatal("a rejected version must not be persisted")
	}

	// upgrade moves to the best stable version.
	r.run("instance", "dep", "upgrade", "alpha", "postgresql")
	chart = r.Read(t, "apps", "alpha", "Chart.yaml")
	if !strings.Contains(chart, "version: 16.0.0") {
		t.Fatalf("upgrade did not move to the best stable version:\n%s", chart)
	}
	if strings.Contains(chart, "-rc") {
		t.Fatalf("upgrade must not select a pre-release:\n%s", chart)
	}

	r.run("instance", "dep", "rm", "alpha", "postgresql")
	if chart := r.Read(t, "apps", "alpha", "Chart.yaml"); strings.Contains(chart, "postgresql") {
		t.Fatalf("dependency not removed:\n%s", chart)
	}
	r.mustFail("instance", "dep", "rm", "alpha", "postgresql")
}

func TestCLI_Dep_AddFromCatalogAppliesDefaultSets(t *testing.T) {
	r := newCLIRepo(t, testutil.RepoOpts{SourceMode: testutil.SourceGit})
	r.run("catalog", "sync")
	r.run("instance", "create", "alpha")

	r.run("instance", "dep", "add-from-catalog", "alpha", "--id", "bitnami-postgresql-15.5.0")

	chart := r.Read(t, "apps", "alpha", "Chart.yaml")
	if !strings.Contains(chart, "name: postgresql") || !strings.Contains(chart, "version: 15.5.0") {
		t.Fatalf("catalog dependency not pinned:\n%s", chart)
	}
	// The catalog entry declares defaultSets: [dev]. add-from-catalog scopes
	// them to the dependency; the global values.set.<set>.yaml form belongs to
	// `dep add --set` and must not be accepted here.
	if !r.Exists(t, "apps", "alpha", "values.dep-set.postgresql--dev.yaml") {
		t.Fatal("catalog defaultSets were not selected as per-dependency markers")
	}

	// --no-default-sets opts out.
	r.run("instance", "create", "beta")
	r.run("instance", "dep", "add-from-catalog", "beta", "--id", "bitnami-nginx-15.0.0", "--no-default-sets")
	if r.Exists(t, "apps", "beta", "values.dep-set.nginx--dev.yaml") ||
		r.Exists(t, "apps", "beta", "values.set.dev.yaml") {
		t.Fatal("--no-default-sets still selected the catalog defaults")
	}

	r.mustFail("instance", "dep", "add-from-catalog", "alpha", "--id", "does-not-exist")
}

func TestCLI_Dep_AddWithSetsAndApply(t *testing.T) {
	r := newCLIRepo(t, testutil.RepoOpts{SourceMode: testutil.SourceGit})
	r.run("catalog", "sync")
	r.run("instance", "create", "alpha")

	r.run("instance", "dep", "add-from-catalog", "alpha",
		"--id", "bitnami-nginx-15.0.0", "--apply")

	if !r.Exists(t, "apps", "alpha", "Chart.lock") {
		t.Fatal("--apply must lock the chart")
	}
	if !r.Exists(t, "apps", "alpha", "charts", "nginx-15.0.0.tgz") {
		t.Fatal("--apply must vendor the dependency")
	}

	calls := r.helmCalls()
	if len(calls) == 0 {
		t.Fatal("--apply with dependencies must invoke helm")
	}
}

func TestCLI_Dep_ValuesGetSetUnset(t *testing.T) {
	r := newCLIRepo(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	r.run("instance", "create", "alpha")
	r.run("instance", "dep", "add", "alpha",
		"--repo", testChartRepo, "--name", "nginx", "--version", "15.0.0")

	r.run("instance", "dep", "values", "set", "alpha", "nginx",
		"--path", "$.replicaCount", "--value-yaml", "4")

	var got float64
	r.runJSON(&got, "instance", "dep", "values", "get", "alpha", "nginx", "--path", "$.replicaCount")
	if got != 4 {
		t.Fatalf("dep override = %v", got)
	}

	// Overrides live under the dep id in the instance edit file.
	layer := r.Read(t, "apps", "alpha", "values.instance.yaml")
	if !strings.Contains(layer, "nginx:") {
		t.Fatalf("dep override not keyed by dep id:\n%s", layer)
	}

	r.run("instance", "dep", "values", "unset", "alpha", "nginx", "--path", "$")
	if layer := r.Read(t, "apps", "alpha", "values.instance.yaml"); strings.Contains(layer, "replicaCount: 4") {
		t.Fatalf("unset $ did not drop the whole override:\n%s", layer)
	}
}

// A dep id containing dots must be used as a literal key, not parsed as a
// values path.
func TestCLI_Dep_DottedAliasIsALiteralKey(t *testing.T) {
	r := newCLIRepo(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	r.run("instance", "create", "alpha")
	r.run("instance", "dep", "add", "alpha",
		"--repo", testChartRepo, "--name", "nginx", "--version", "15.0.0",
		"--alias", "web.edge")

	r.run("instance", "dep", "values", "set", "alpha", "web.edge",
		"--path", "$.replicaCount", "--value-yaml", "2")

	layer := r.Read(t, "apps", "alpha", "values.instance.yaml")
	if !strings.Contains(layer, "web.edge:") {
		t.Fatalf("dotted dep id was not used as a literal key:\n%s", layer)
	}
	if strings.Contains(layer, "\n  edge:") {
		t.Fatalf("dotted dep id was split into a nested path:\n%s", layer)
	}

	var got float64
	r.runJSON(&got, "instance", "dep", "values", "get", "alpha", "web.edge", "--path", "$.replicaCount")
	if got != 2 {
		t.Fatalf("override not readable back: %v", got)
	}
}

func TestCLI_Dep_Inspect(t *testing.T) {
	r := newCLIRepo(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	r.run("instance", "create", "alpha")
	r.run("instance", "dep", "add", "alpha",
		"--repo", testChartRepo, "--name", "nginx", "--version", "15.0.0")

	if out := r.run("instance", "dep", "inspect", "readme", "alpha", "nginx"); !strings.Contains(out, "# nginx") {
		t.Fatalf("readme:\n%s", out)
	}
	if out := r.run("instance", "dep", "inspect", "values", "alpha", "nginx"); !strings.Contains(out, "repository: fake/nginx") {
		t.Fatalf("values:\n%s", out)
	}
	if out := r.run("instance", "dep", "inspect", "schema", "alpha", "nginx"); !strings.Contains(out, `"replicaCount"`) {
		t.Fatalf("schema:\n%s", out)
	}

	r.mustFail("instance", "dep", "inspect", "values", "alpha", "ghost")
}

// A catalog dependency must be recorded as catalog-attached whichever
// interface added it, otherwise detach has nothing to detach from and the
// web UI shows it as arbitrary.
func TestCLI_Dep_AddFromCatalogRecordsAttribution(t *testing.T) {
	r := newCLIRepo(t, testutil.RepoOpts{SourceMode: testutil.SourceGit})
	r.run("catalog", "sync")
	r.run("instance", "create", "alpha")
	r.run("instance", "dep", "add-from-catalog", "alpha", "--id", "bitnami-nginx-15.0.0")

	meta := r.Read(t, ".helmdex", "depmeta", "alpha", "nginx.yaml")
	for _, want := range []string{"kind: catalog", "bitnami-nginx-15.0.0", "Example"} {
		if !strings.Contains(meta, want) {
			t.Fatalf("depmeta missing %q:\n%s", want, meta)
		}
	}

	r.run("instance", "dep", "detach", "alpha", "nginx")
	if meta := r.Read(t, ".helmdex", "depmeta", "alpha", "nginx.yaml"); !strings.Contains(meta, "kind: arbitrary") {
		t.Fatalf("detach did not clear the attachment:\n%s", meta)
	}
	// A detached dependency is no longer catalog-attached.
	r.mustFail("instance", "dep", "detach", "alpha", "nginx")
}

func TestCLI_Dep_AddArbitraryIsNotCatalogAttached(t *testing.T) {
	r := newCLIRepo(t, testutil.RepoOpts{SourceMode: testutil.SourceGit})
	r.run("catalog", "sync")
	r.run("instance", "create", "alpha")

	r.run("instance", "dep", "add", "alpha",
		"--repo", testChartRepo, "--name", "nginx", "--version", "15.0.0")
	if meta := r.Read(t, ".helmdex", "depmeta", "alpha", "nginx.yaml"); !strings.Contains(meta, "kind: arbitrary") {
		t.Fatalf("an arbitrary dependency must be recorded as such:\n%s", meta)
	}
	r.mustFail("instance", "dep", "detach", "alpha", "nginx")
}

// Removing a dependency frees its id. Attribution left behind would be
// inherited by whatever takes that id next — a dependency pointing anywhere
// would then read as coming from the catalog.
func TestCLI_Dep_RemoveClearsAttribution(t *testing.T) {
	r := newCLIRepo(t, testutil.RepoOpts{SourceMode: testutil.SourceGit})
	r.run("catalog", "sync")
	r.run("instance", "create", "alpha")

	r.run("instance", "dep", "add-from-catalog", "alpha", "--id", "bitnami-nginx-15.0.0", "--no-default-sets")
	if !r.Exists(t, ".helmdex", "depmeta", "alpha", "nginx.yaml") {
		t.Fatal("expected catalog attribution to be recorded")
	}

	r.run("instance", "dep", "rm", "alpha", "nginx")
	if r.Exists(t, ".helmdex", "depmeta", "alpha", "nginx.yaml") {
		t.Fatal("removing a dependency must drop its source metadata")
	}

	// The same id, now pointing somewhere else entirely.
	r.run("instance", "dep", "add", "alpha",
		"--repo", "https://elsewhere.invalid/charts", "--name", "nginx", "--version", "1.0.0")
	meta := r.Read(t, ".helmdex", "depmeta", "alpha", "nginx.yaml")
	if strings.Contains(meta, "catalog") {
		t.Fatalf("an arbitrary dependency inherited a catalog attribution:\n%s", meta)
	}
	r.mustFail("instance", "dep", "detach", "alpha", "nginx")
}

// A refused command must leave nothing behind: the mode check runs before the
// first write.
func TestCLI_Dep_AddFromCatalogRefusalLeavesNoPartialState(t *testing.T) {
	r := newCLIRepo(t, testutil.RepoOpts{SourceMode: testutil.SourceGit})
	r.run("catalog", "sync")
	// A direct-mode instance: no values.instance.yaml.
	r.Write(t, "apiVersion: v2\nname: direct\nversion: 0.1.0\n", "apps", "direct", "Chart.yaml")
	r.Write(t, "{}\n", "apps", "direct", "values.yaml")

	// The fixture entry declares defaultSets, which direct mode cannot hold.
	msg := r.mustFail("instance", "dep", "add-from-catalog", "direct", "--id", "bitnami-nginx-15.0.0")
	if !strings.Contains(msg, "direct-mode") {
		t.Fatalf("unexpected error: %s", msg)
	}

	if chart := r.Read(t, "apps", "direct", "Chart.yaml"); strings.Contains(chart, "nginx") {
		t.Fatalf("a refused add still wrote the dependency:\n%s", chart)
	}
	if r.Exists(t, ".helmdex", "depmeta", "direct", "nginx.yaml") {
		t.Fatal("a refused add still recorded source metadata")
	}
}

func TestCLI_Dep_SyncPresets(t *testing.T) {
	r := newCLIRepo(t, testutil.RepoOpts{SourceMode: testutil.SourceGit})
	r.run("catalog", "sync")
	r.run("instance", "create", "alpha")
	r.run("instance", "dep", "add-from-catalog", "alpha", "--id", "bitnami-nginx-15.0.0")

	// Set markers that no preset backs must be pruned.
	r.Write(t, "{}\n", "apps", "alpha", "values.dep-set.nginx--does-not-exist.yaml")
	r.run("instance", "dep", "sync-presets", "alpha", "nginx")

	if r.Exists(t, "apps", "alpha", "values.dep-set.nginx--does-not-exist.yaml") {
		t.Fatal("sync-presets must prune orphan set markers")
	}
	if !r.Exists(t, "apps", "alpha", "values.yaml") {
		t.Fatal("sync-presets must regenerate merged values")
	}

	r.mustFail("instance", "dep", "sync-presets", "alpha", "ghost")
}

// OCI dependencies list versions from the registry's tags, so `dep versions`
// works on them like on any classic repository.
func TestCLI_Dep_VersionsForOCI(t *testing.T) {
	r := newCLIRepo(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	testutil.FakeRegistry(t, map[string][]string{
		"org/demo": {"0.1.0", "1.0.0", "artifacthub.io", "1.1.0-rc.1"},
	})
	r.run("instance", "create", "alpha")
	r.run("instance", "dep", "add", "alpha",
		"--repo", "oci://registry.example.invalid/org", "--name", "demo", "--version", "0.1.0")

	out := r.run("instance", "dep", "versions", "alpha", "demo", "--format", "table")
	if got := strings.Join(strings.Fields(out), ","); got != "1.0.0,0.1.0" {
		t.Fatalf("versions = %q, want newest-first without the junk tag", got)
	}

	// --format json is the documented contract: an array, newest-first.
	var versions []string
	r.runJSON(&versions, "instance", "dep", "versions", "alpha", "demo")
	if strings.Join(versions, ",") != "1.0.0,0.1.0" {
		t.Fatalf("json versions = %v", versions)
	}
}

// Auto-upgrade reaches OCI dependencies too, now that their versions can be
// listed. The pre-release must not be picked as "latest stable".
func TestCLI_Dep_UpgradeOCI(t *testing.T) {
	r := newCLIRepo(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	testutil.FakeRegistry(t, map[string][]string{
		"org/demo": {"0.1.0", "1.0.0", "1.1.0-rc.1"},
	})
	r.run("instance", "create", "alpha")
	r.run("instance", "dep", "add", "alpha",
		"--repo", "oci://registry.example.invalid/org", "--name", "demo", "--version", "0.1.0")

	r.run("instance", "dep", "upgrade", "alpha", "demo")

	if chart := r.Read(t, "apps", "alpha", "Chart.yaml"); !strings.Contains(chart, "version: 1.0.0") {
		t.Fatalf("dependency not upgraded to the latest stable:\n%s", chart)
	}
}

// A registry that cannot be listed must fail loudly rather than yield an empty
// version list.
func TestCLI_Dep_VersionsOCIFailureIsSurfaced(t *testing.T) {
	r := newCLIRepo(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	testutil.FakeRegistry(t, map[string][]string{})
	r.run("instance", "create", "alpha")
	r.run("instance", "dep", "add", "alpha",
		"--repo", "oci://registry.example.invalid/org", "--name", "demo", "--version", "0.1.0")

	if msg := r.mustFail("instance", "dep", "versions", "alpha", "demo"); !strings.Contains(msg, "404") {
		t.Fatalf("unexpected error: %s", msg)
	}
}

func TestCLI_Dep_HelmFailureIsSurfaced(t *testing.T) {
	r := newCLIRepo(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	r.run("instance", "create", "alpha")
	r.run("instance", "dep", "add", "alpha",
		"--repo", testChartRepo, "--name", "nginx", "--version", "15.0.0")

	t.Setenv("HELMDEX_FAKE_HELM_FAIL", "dependency=simulated registry outage")

	msg := r.mustFail("instance", "apply", "alpha")
	if !strings.Contains(msg, "simulated registry outage") {
		t.Fatalf("helm failure not surfaced: %s", msg)
	}
	if r.Exists(t, "apps", "alpha", "Chart.lock") {
		t.Fatal("a failed relock must not leave a lock behind")
	}
}

// A registry publishing only non-SemVer tags yields no versions; the JSON
// contract is an empty array, not null.
func TestCLI_Dep_VersionsForOCIEmptyIsAnArray(t *testing.T) {
	r := newCLIRepo(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	testutil.FakeRegistry(t, map[string][]string{"org/demo": {"latest", "main", "artifacthub.io"}})
	r.run("instance", "create", "alpha")
	r.run("instance", "dep", "add", "alpha",
		"--repo", "oci://registry.example.invalid/org", "--name", "demo", "--version", "0.1.0")

	if out := strings.TrimSpace(r.run("instance", "dep", "versions", "alpha", "demo")); out != "[]" {
		t.Fatalf("json output = %q, want an empty array", out)
	}
}
