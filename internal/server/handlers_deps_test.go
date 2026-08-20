package server

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"helmdex/internal/helmutil"
	"helmdex/internal/ociregistry"
	"helmdex/internal/testutil"
)

const fixtureRepoURL = "https://example.invalid/charts"

// addDep adds a dependency through the API and returns the refreshed instance.
func addDep(ts *testServer, instance string, body map[string]any) instanceInfo {
	ts.t.Helper()
	var info instanceInfo
	ts.post("/api/instances/"+instance+"/deps", body).
		expect(http.StatusCreated).decode(&info)
	return info
}

func TestDeps_AddListRemove(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	ts.createInstance("alpha")

	info := addDep(ts, "alpha", map[string]any{
		"name":          "postgresql",
		"repository":    fixtureRepoURL,
		"version":       "15.5.0",
		"sourceKind":    "catalog",
		"catalogID":     "bitnami-postgresql-15.5.0",
		"catalogSource": "Example",
	})
	if len(info.Deps) != 1 {
		t.Fatalf("expected one dependency, got %+v", info.Deps)
	}
	dep := info.Deps[0]
	if dep.ID != "postgresql" || dep.Version != "15.5.0" {
		t.Fatalf("unexpected dep: %+v", dep)
	}
	if dep.SourceKind != "catalog" || dep.CatalogID != "bitnami-postgresql-15.5.0" {
		t.Fatalf("catalog attribution lost: %+v", dep)
	}

	// The dependency is in Chart.yaml, not just in the response.
	chart := ts.Repo.Read(t, "apps", "alpha", "Chart.yaml")
	if !strings.Contains(chart, "name: postgresql") || !strings.Contains(chart, "version: 15.5.0") {
		t.Fatalf("dependency not persisted:\n%s", chart)
	}

	var deps []depInfo
	ts.get("/api/instances/alpha/deps").expect(http.StatusOK).decode(&deps)
	if len(deps) != 1 || deps[0].ID != "postgresql" {
		t.Fatalf("unexpected deps listing: %+v", deps)
	}

	ts.del("/api/instances/alpha/deps/postgresql", nil).expect(http.StatusNoContent)
	if chart := ts.Repo.Read(t, "apps", "alpha", "Chart.yaml"); strings.Contains(chart, "postgresql") {
		t.Fatalf("dependency not removed:\n%s", chart)
	}
	// The id is free again: leftover attribution would be inherited by the
	// next dependency taking that id.
	if ts.Repo.Exists(t, ".helmdex", "depmeta", "alpha", "postgresql.yaml") {
		t.Fatal("removing a dependency must drop its source metadata")
	}
	ts.del("/api/instances/alpha/deps/postgresql", nil).expect(http.StatusNotFound)
}

func TestDeps_AliasBecomesTheDepID(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	ts.createInstance("alpha")

	info := addDep(ts, "alpha", map[string]any{
		"name":       "postgresql",
		"repository": fixtureRepoURL,
		"version":    "15.5.0",
		"alias":      "primary-db",
	})
	if info.Deps[0].ID != "primary-db" || info.Deps[0].Name != "postgresql" {
		t.Fatalf("alias must become the dep id: %+v", info.Deps[0])
	}

	// The same chart can be added twice under a different alias.
	info = addDep(ts, "alpha", map[string]any{
		"name":       "postgresql",
		"repository": fixtureRepoURL,
		"version":    "15.6.0",
		"alias":      "replica-db",
	})
	if len(info.Deps) != 2 {
		t.Fatalf("expected two aliased dependencies, got %+v", info.Deps)
	}
}

func TestDeps_AddRejectsIncompleteRequests(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	ts.createInstance("alpha")

	for _, body := range []map[string]any{
		{"repository": fixtureRepoURL, "version": "1.0.0"},
		{"name": "x", "version": "1.0.0"},
		{"name": "x", "repository": fixtureRepoURL},
	} {
		ts.post("/api/instances/alpha/deps", body).expect(http.StatusBadRequest)
	}
}

func TestDeps_SetVersionValidatesAgainstTheRegistry(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	ts.createInstance("alpha")
	addDep(ts, "alpha", map[string]any{
		"name":       "postgresql",
		"repository": fixtureRepoURL,
		"version":    "15.5.0",
	})

	// A version the registry knows.
	var info instanceInfo
	ts.post("/api/instances/alpha/deps/postgresql/version",
		map[string]any{"version": "15.6.0", "validate": true}).
		expect(http.StatusOK).decode(&info)
	if info.Deps[0].Version != "15.6.0" {
		t.Fatalf("version not updated: %+v", info.Deps[0])
	}
	if chart := ts.Repo.Read(t, "apps", "alpha", "Chart.yaml"); !strings.Contains(chart, "version: 15.6.0") {
		t.Fatalf("version not persisted:\n%s", chart)
	}

	// A version it does not.
	res := ts.post("/api/instances/alpha/deps/postgresql/version",
		map[string]any{"version": "99.0.0", "validate": true}).
		expect(http.StatusBadRequest)
	if !strings.Contains(res.errorMessage(), "invalid version") {
		t.Fatalf("unexpected error: %s", res.errorMessage())
	}
	if chart := ts.Repo.Read(t, "apps", "alpha", "Chart.yaml"); strings.Contains(chart, "99.0.0") {
		t.Fatal("a rejected version must not be persisted")
	}

	ts.post("/api/instances/alpha/deps/postgresql/version", map[string]any{"version": ""}).
		expect(http.StatusBadRequest)
	ts.post("/api/instances/alpha/deps/ghost/version", map[string]any{"version": "1.0.0"}).
		expect(http.StatusNotFound)
}

func TestDeps_VersionsListing(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	ts.createInstance("alpha")
	addDep(ts, "alpha", map[string]any{
		"name":       "postgresql",
		"repository": fixtureRepoURL,
		"version":    "15.5.0",
	})

	var got struct {
		Current    string   `json:"current"`
		Versions   []string `json:"versions"`
		BestStable string   `json:"bestStable"`
	}
	ts.get("/api/instances/alpha/deps/postgresql/versions").expect(http.StatusOK).decode(&got)

	if got.Current != "15.5.0" {
		t.Fatalf("current = %q", got.Current)
	}
	if len(got.Versions) == 0 || got.Versions[0] != "16.0.0" {
		t.Fatalf("versions must be newest-first: %v", got.Versions)
	}
	if got.BestStable != "16.0.0" {
		t.Fatalf("bestStable = %q, want 16.0.0", got.BestStable)
	}
	for _, v := range got.Versions {
		if strings.Contains(v, "-rc") {
			t.Fatalf("pre-releases must not appear in the default listing: %v", got.Versions)
		}
	}
}

// OCI repositories have no index.yaml: their versions are registry tags, and
// the listing must apply the same policy as a classic repo — junk tags out,
// pre-releases hidden, newest first.
func TestDeps_VersionsForOCI(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	testutil.FakeRegistry(t, map[string][]string{
		"org/demo": {"0.1.0", "1.0.0", "artifacthub.io", "1.1.0-rc.1", "0.9.0"},
	})
	ts.createInstance("alpha")
	addDep(ts, "alpha", map[string]any{
		"name":       "demo",
		"repository": "oci://registry.example.invalid/org",
		"version":    "0.1.0",
	})

	var got struct {
		Current    string   `json:"current"`
		Versions   []string `json:"versions"`
		BestStable string   `json:"bestStable"`
	}
	ts.get("/api/instances/alpha/deps/demo/versions").expect(http.StatusOK).decode(&got)

	if strings.Join(got.Versions, ",") != "1.0.0,0.9.0,0.1.0" {
		t.Fatalf("versions = %v", got.Versions)
	}
	if got.Current != "0.1.0" {
		t.Fatalf("current = %q", got.Current)
	}
	if got.BestStable != "1.0.0" {
		t.Fatalf("bestStable = %q", got.BestStable)
	}
}

// A registry that refuses the listing must reach the UI as an auth prompt for
// that host, not as an opaque failure.
func TestDeps_VersionsOCIUnauthorized(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Basic realm="registry"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer registry.Close()
	t.Setenv(ociregistry.FakeRegistryEnv, registry.URL)

	ts.createInstance("alpha")
	addDep(ts, "alpha", map[string]any{
		"name":       "demo",
		"repository": "oci://registry.example.invalid/org",
		"version":    "0.1.0",
	})

	var got struct {
		Error        string `json:"error"`
		AuthRequired *struct {
			Candidates []struct {
				Host string `json:"host"`
				Kind string `json:"kind"`
			} `json:"candidates"`
		} `json:"authRequired"`
	}
	ts.get("/api/instances/alpha/deps/demo/versions").expect(http.StatusUnauthorized).decode(&got)

	if got.AuthRequired == nil || len(got.AuthRequired.Candidates) == 0 {
		t.Fatalf("no auth candidate offered: %+v", got)
	}
	c := got.AuthRequired.Candidates[0]
	if c.Host != "registry.example.invalid" || c.Kind != "oci" {
		t.Fatalf("candidate = %+v", c)
	}
}

func TestDeps_Inspect(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	ts.createInstance("alpha")
	addDep(ts, "alpha", map[string]any{
		"name":       "nginx",
		"repository": fixtureRepoURL,
		"version":    "15.0.0",
	})

	tests := map[string]string{
		"readme": "# nginx",
		"values": "repository: fake/nginx",
		"schema": `"replicaCount"`,
	}
	for kind, want := range tests {
		t.Run(kind, func(t *testing.T) {
			body := ts.as(t).get("/api/instances/alpha/deps/nginx/inspect?kind=" + kind).
				expect(http.StatusOK).text()
			if !strings.Contains(body, want) {
				t.Fatalf("inspect %s missing %q:\n%s", kind, want, body)
			}
		})
	}

	// A candidate version can be inspected without changing the pinned one.
	body := ts.get("/api/instances/alpha/deps/nginx/inspect?kind=values&version=15.2.0").
		expect(http.StatusOK).text()
	if !strings.Contains(body, `tag: "15.2.0"`) {
		t.Fatalf("version override ignored:\n%s", body)
	}
	if chart := ts.Repo.Read(t, "apps", "alpha", "Chart.yaml"); !strings.Contains(chart, "version: 15.0.0") {
		t.Fatal("inspecting a candidate version must not repin the dependency")
	}

	ts.get("/api/instances/alpha/deps/nginx/inspect?kind=bogus").expect(http.StatusBadRequest)
	ts.get("/api/instances/alpha/deps/ghost/inspect?kind=values").expect(http.StatusNotFound)
}

// A chart that ships no README/schema must report the artifact as genuinely
// absent (404 + a clear message), distinct from a fetch failure — so the tab
// reads as empty-by-design, not broken.
func TestDeps_InspectArtifactAbsent(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	ts.createInstance("alpha")
	const name, version = "bare", "1.0.0"
	addDep(ts, "alpha", map[string]any{"name": name, "repository": fixtureRepoURL, "version": version})

	// Seed the isolated per-URL env cache with an archive holding only
	// values.yaml, so the inspect flow reads it without any network.
	env := helmutil.EnvForRepoURL(ts.Repo.Root, fixtureRepoURL)
	tgz := filepath.Join(env.CacheHome, "repository", name+"-"+version+".tgz")
	writeTestChartArchive(t, tgz, name, map[string]string{
		"Chart.yaml":  "apiVersion: v2\nname: " + name + "\nversion: " + version + "\n",
		"values.yaml": "replicaCount: 1\n",
	})

	// values load from the archive…
	ts.get("/api/instances/alpha/deps/" + name + "/inspect?kind=values").expect(http.StatusOK)
	// …readme and schema report a clear, non-alarming absence.
	for _, kind := range []string{"readme", "schema"} {
		msg := ts.as(t).get("/api/instances/alpha/deps/" + name + "/inspect?kind=" + kind).
			expect(http.StatusNotFound).errorMessage()
		if !strings.Contains(msg, "does not ship") {
			t.Fatalf("inspect %s: want an 'absent' message, got %q", kind, msg)
		}
	}
}

func writeTestChartArchive(t *testing.T, dest, chartName string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: chartName + "/" + name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDeps_DetachRequiresCatalogAttachment(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	ts.createInstance("alpha")

	addDep(ts, "alpha", map[string]any{
		"name":       "nginx",
		"repository": fixtureRepoURL,
		"version":    "15.0.0",
	})
	ts.post("/api/instances/alpha/deps/nginx/detach", nil).expect(http.StatusBadRequest)

	addDep(ts, "alpha", map[string]any{
		"name":          "postgresql",
		"repository":    fixtureRepoURL,
		"version":       "15.5.0",
		"sourceKind":    "catalog",
		"catalogID":     "bitnami-postgresql-15.5.0",
		"catalogSource": "Example",
	})
	var info instanceInfo
	ts.post("/api/instances/alpha/deps/postgresql/detach", nil).expect(http.StatusOK).decode(&info)

	for _, d := range info.Deps {
		if d.ID == "postgresql" && d.SourceKind != "arbitrary" {
			t.Fatalf("detach must clear catalog attachment: %+v", d)
		}
	}
	// Detaching twice is no longer possible: it is no longer catalog-attached.
	ts.post("/api/instances/alpha/deps/postgresql/detach", nil).expect(http.StatusBadRequest)
}

// Dep IDs are used as literal map keys in the values file. A dotted or
// bracketed id must not be parsed as a values path.
func TestDeps_DottedDepIDIsALiteralValuesKey(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	ts.createInstance("alpha")

	const depID = "my.dotted.dep"
	addDep(ts, "alpha", map[string]any{
		"name":       "nginx",
		"repository": fixtureRepoURL,
		"version":    "15.0.0",
		"alias":      depID,
	})

	escaped := url.PathEscape(depID)
	ts.put("/api/instances/alpha/deps/"+escaped+"/values",
		map[string]any{"value": map[string]any{"replicaCount": 2}}).
		expect(http.StatusNoContent)

	// The key is written whole, not split into nested maps.
	layer := ts.Repo.Read(t, "apps", "alpha", "values.instance.yaml")
	if !strings.Contains(layer, depID+":") {
		t.Fatalf("dep id was not used as a literal key:\n%s", layer)
	}
	if strings.Contains(layer, "dotted:") {
		t.Fatalf("dep id was split into a nested path:\n%s", layer)
	}

	var got map[string]any
	ts.get("/api/instances/alpha/deps/" + escaped + "/values").expect(http.StatusOK).decode(&got)
	if got["found"] != true {
		t.Fatalf("per-dep override not readable back: %v", got)
	}
	value, ok := got["value"].(map[string]any)
	if !ok || value["replicaCount"] != float64(2) {
		t.Fatalf("unexpected override value: %v", got["value"])
	}
}

func TestDeps_ApplyRelocksAndVendors(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	ts.createInstance("alpha")
	addDep(ts, "alpha", map[string]any{
		"name":       "postgresql",
		"repository": fixtureRepoURL,
		"version":    "15.5.0",
	})

	ts.post("/api/instances/alpha/apply", map[string]any{"relock": false}).expect(http.StatusOK)

	if !ts.Repo.Exists(t, "apps", "alpha", "Chart.lock") {
		t.Fatal("apply must lock a chart that declares dependencies")
	}
	if !ts.Repo.Exists(t, "apps", "alpha", "charts", "postgresql-15.5.0.tgz") {
		t.Fatal("apply must vendor the dependency archive")
	}

	calls := testutil.FakeHelmCalls(t, ts.HelmLog)
	if len(calls) == 0 {
		t.Fatal("apply with dependencies must invoke helm")
	}

	// Chart.yaml and Chart.lock now agree: a second apply reconciles without
	// re-locking.
	before := len(calls)
	ts.post("/api/instances/alpha/apply", nil).expect(http.StatusOK)
	after := testutil.FakeHelmCalls(t, ts.HelmLog)
	for _, c := range after[before:] {
		if strings.HasPrefix(c, "dependency") {
			t.Fatalf("unchanged dependencies must not trigger a relock, got %v", after[before:])
		}
	}
}

func TestDeps_ApplyForcedRelockRunsHelm(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	ts.createInstance("alpha")
	addDep(ts, "alpha", map[string]any{
		"name":       "postgresql",
		"repository": fixtureRepoURL,
		"version":    "15.5.0",
	})
	ts.post("/api/instances/alpha/apply", nil).expect(http.StatusOK)

	before := len(testutil.FakeHelmCalls(t, ts.HelmLog))
	ts.post("/api/instances/alpha/apply", map[string]any{"relock": true}).expect(http.StatusOK)

	forced := testutil.FakeHelmCalls(t, ts.HelmLog)[before:]
	found := false
	for _, c := range forced {
		if strings.HasPrefix(c, "dependency") {
			found = true
		}
	}
	if !found {
		t.Fatalf("relock:true must re-run helm dependency, got %v", forced)
	}
}

func TestDeps_ApplySurfacesHelmFailures(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	ts.createInstance("alpha")
	addDep(ts, "alpha", map[string]any{
		"name":       "postgresql",
		"repository": fixtureRepoURL,
		"version":    "15.5.0",
	})

	t.Setenv("HELMDEX_FAKE_HELM_FAIL", "dependency=simulated registry outage")

	res := ts.post("/api/instances/alpha/apply", nil).expect(http.StatusInternalServerError)
	if !strings.Contains(res.errorMessage(), "simulated registry outage") {
		t.Fatalf("helm failure not surfaced: %s", res.errorMessage())
	}
	if ts.Repo.Exists(t, "apps", "alpha", "Chart.lock") {
		t.Fatal("a failed relock must not leave a lock file behind")
	}
}

// A repository naming the chart instead of its namespace addresses
// <chart>/<chart>, which the registry rejects. The failure must carry the
// reason, so the user is not left guessing at the version picker.
func TestDeps_VersionsOCIFullRefExplainsItself(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	testutil.FakeRegistry(t, map[string][]string{"org/demo": {"1.0.0"}})
	ts.createInstance("alpha")
	addDep(ts, "alpha", map[string]any{
		"name":       "demo",
		"repository": "oci://registry.example.invalid/org/demo",
		"version":    "0.1.0",
	})

	msg := ts.get("/api/instances/alpha/deps/demo/versions").expect(http.StatusBadGateway).errorMessage()
	if !strings.Contains(msg, `set repository to "oci://registry.example.invalid/org"`) {
		t.Fatalf("the error must name the repository to use: %s", msg)
	}
}

// A namespace may legitimately end with the chart name (oci://reg/nginx holding
// nginx/nginx). When such a registry demands credentials, the sign-in prompt
// must survive: a shape heuristic must never outrank a real auth failure, or
// the user is told to break a working configuration.
func TestDeps_VersionsOCIAuthWinsOverTheFullRefHint(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"errors":[{"code":"DENIED","message":"denied: requested access to the resource is denied"}]}`, http.StatusForbidden)
	}))
	defer registry.Close()
	t.Setenv(ociregistry.FakeRegistryEnv, registry.URL)

	ts.createInstance("alpha")
	addDep(ts, "alpha", map[string]any{
		"name":       "nginx",
		"repository": "oci://reg.private.invalid/nginx",
		"version":    "1.0.0",
	})

	var got struct {
		Error        string `json:"error"`
		AuthRequired *struct {
			Candidates []struct {
				Host string `json:"host"`
			} `json:"candidates"`
		} `json:"authRequired"`
	}
	ts.get("/api/instances/alpha/deps/nginx/versions").expect(http.StatusUnauthorized).decode(&got)

	if got.AuthRequired == nil || len(got.AuthRequired.Candidates) == 0 {
		t.Fatalf("the sign-in prompt must survive the hint: %+v", got)
	}
	if got.AuthRequired.Candidates[0].Host != "reg.private.invalid" {
		t.Fatalf("candidate = %+v", got.AuthRequired.Candidates[0])
	}
}
