package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"helmdex/internal/testutil"
	"helmdex/internal/yamlchart"
)

func TestRepoInfo_ReportsOptInAndLayout(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceGit})

	var info repoInfo
	ts.get("/api/repo").expect(http.StatusOK).decode(&info)

	if info.Root != ts.Repo.Root {
		t.Fatalf("root = %q, want %q", info.Root, ts.Repo.Root)
	}
	if !info.OptedIn {
		t.Fatal("a repo with helmdex.yaml must report optedIn")
	}
	if info.AppsDir != "apps" {
		t.Fatalf("appsDir = %q, want apps", info.AppsDir)
	}
	if info.ConfigError != "" {
		t.Fatalf("unexpected configError: %s", info.ConfigError)
	}
}

func TestRepoInfo_AgnosticRepoIsNotOptedIn(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{Agnostic: true})

	var info repoInfo
	ts.get("/api/repo").expect(http.StatusOK).decode(&info)

	if info.OptedIn {
		t.Fatal("a repo without helmdex.yaml must not report optedIn")
	}
	if ts.Repo.Exists(t, ".helmdex") {
		t.Fatal("helmdex must not create in-repo state in an agnostic repo")
	}
}

func TestInstanceLifecycle(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceGit})

	var list []instanceInfo
	ts.get("/api/instances").expect(http.StatusOK).decode(&list)
	if len(list) != 0 {
		t.Fatalf("expected no instances initially, got %+v", list)
	}

	// --- create ---
	created := ts.createInstance("alpha")
	if created.Name != "alpha" || !created.Managed {
		t.Fatalf("unexpected created instance: %+v", created)
	}
	if !ts.Repo.Exists(t, "apps", "alpha", "Chart.yaml") {
		t.Fatal("Chart.yaml not written")
	}
	if !ts.Repo.Exists(t, "apps", "alpha", "values.instance.yaml") {
		t.Fatal("managed instance must own a values.instance.yaml")
	}

	chart, err := yamlchart.ReadChart(filepath.Join(ts.instancePath("alpha"), "Chart.yaml"))
	if err != nil {
		t.Fatalf("read chart: %v", err)
	}
	if chart.Name != "alpha" {
		t.Fatalf("chart name = %q, want alpha", chart.Name)
	}

	// --- get ---
	var got instanceInfo
	ts.get("/api/instances/alpha").expect(http.StatusOK).decode(&got)
	if got.Name != "alpha" || len(got.Deps) != 0 {
		t.Fatalf("unexpected instance: %+v", got)
	}

	// --- rename ---
	var renamed instanceInfo
	ts.post("/api/instances/alpha/rename", map[string]string{"newName": "beta"}).
		expect(http.StatusOK).decode(&renamed)
	if renamed.Name != "beta" {
		t.Fatalf("renamed name = %q, want beta", renamed.Name)
	}
	if ts.Repo.Exists(t, "apps", "alpha") {
		t.Fatal("old instance directory still present after rename")
	}
	chart, err = yamlchart.ReadChart(filepath.Join(ts.instancePath("beta"), "Chart.yaml"))
	if err != nil {
		t.Fatalf("read renamed chart: %v", err)
	}
	if chart.Name != "beta" {
		t.Fatalf("chart name after rename = %q, want beta", chart.Name)
	}

	// --- apply (no deps: nothing for Helm to do) ---
	ts.post("/api/instances/beta/apply", nil).expect(http.StatusOK)
	if calls := testutil.FakeHelmCalls(t, ts.HelmLog); len(calls) != 0 {
		t.Fatalf("apply on a dependency-less instance must not run helm, got %v", calls)
	}
	if !ts.Repo.Exists(t, "apps", "beta", "values.yaml") {
		t.Fatal("apply must generate values.yaml for a managed instance")
	}

	// --- delete ---
	ts.del("/api/instances/beta", nil).expect(http.StatusNoContent)
	if ts.Repo.Exists(t, "apps", "beta") {
		t.Fatal("instance directory still present after delete")
	}
	ts.get("/api/instances/beta").expect(http.StatusNotFound)
}

func TestInstanceCreate_RejectsBadInput(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	ts.createInstance("alpha")

	tests := []struct {
		name string
		body any
		want int
	}{
		{"empty name", map[string]string{"name": ""}, http.StatusBadRequest},
		{"duplicate", map[string]string{"name": "alpha"}, http.StatusBadRequest},
		{"unknown field", map[string]string{"name": "x", "nope": "y"}, http.StatusBadRequest},
		{"malformed json", "{", http.StatusBadRequest},
		{"unknown template", map[string]string{"name": "x", "fromTemplate": "absent"}, http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ts.as(t).post("/api/instances", tc.body).expect(tc.want)
		})
	}
}

func TestInstanceRename_RejectsCollisionAndBadName(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	ts.createInstance("alpha")
	ts.createInstance("beta")

	ts.post("/api/instances/alpha/rename", map[string]string{"newName": "beta"}).
		expect(http.StatusBadRequest)
	ts.post("/api/instances/alpha/rename", map[string]string{"newName": ""}).
		expect(http.StatusBadRequest)

	// Both instances must survive a rejected rename.
	ts.get("/api/instances/alpha").expect(http.StatusOK)
	ts.get("/api/instances/beta").expect(http.StatusOK)
}

func TestInstanceMissing_Returns404(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})

	for _, path := range []string{
		"/api/instances/ghost",
		"/api/instances/ghost/files",
		"/api/instances/ghost/file?path=values.yaml",
		"/api/instances/ghost/values",
		"/api/instances/ghost/deps",
		"/api/instances/ghost/sets",
	} {
		t.Run(path, func(t *testing.T) {
			ts.as(t).get(path).expect(http.StatusNotFound)
		})
	}
	ts.del("/api/instances/ghost", nil).expect(http.StatusNotFound)
	ts.post("/api/instances/ghost/apply", nil).expect(http.StatusNotFound)
}

func TestTemplates_ListAndInstantiate(t *testing.T) {
	// Layout discovery runs once at startup, so the blueprint must exist
	// before the server is built.
	ts := newTestServer(t, testutil.RepoOpts{
		SourceMode: testutil.SourceNone,
		Seed: func(t *testing.T, r testutil.Repo) {
			r.Write(t, "apiVersion: v2\nname: review\nversion: 0.1.0\n",
				"templates", "review", "Chart.yaml")
			r.Write(t, "# blueprint overrides\nreplicaCount: 2\n",
				"templates", "review", "values.instance.yaml")
		},
	})

	var list []map[string]string
	ts.get("/api/templates").expect(http.StatusOK).decode(&list)
	if len(list) != 1 || list[0]["name"] != "review" {
		t.Fatalf("unexpected templates: %+v", list)
	}

	var created instanceInfo
	ts.post("/api/instances", map[string]string{"name": "review-42", "fromTemplate": "review"}).
		expect(http.StatusCreated).decode(&created)
	if created.Name != "review-42" {
		t.Fatalf("created = %+v", created)
	}
	if got := ts.Repo.Read(t, "apps", "review-42", "values.instance.yaml"); !strings.Contains(got, "replicaCount: 2") {
		t.Fatalf("template values not carried over:\n%s", got)
	}
}

func TestFiles_ListReadWrite(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	ts.createInstance("alpha")

	var files []fileInfo
	ts.get("/api/instances/alpha/files").expect(http.StatusOK).decode(&files)
	names := map[string]bool{}
	for _, f := range files {
		names[f.Path] = true
	}
	for _, want := range []string{"Chart.yaml", "values.instance.yaml"} {
		if !names[want] {
			t.Fatalf("file listing missing %q: %+v", want, files)
		}
	}

	body := ts.get("/api/instances/alpha/file?path=Chart.yaml").expect(http.StatusOK).text()
	if !strings.Contains(body, "name: alpha") {
		t.Fatalf("unexpected Chart.yaml body:\n%s", body)
	}

	ts.put("/api/instances/alpha/file?path=values.instance.yaml", "replicaCount: 3\n").
		expect(http.StatusNoContent)
	if got := ts.Repo.Read(t, "apps", "alpha", "values.instance.yaml"); got != "replicaCount: 3\n" {
		t.Fatalf("file not written verbatim: %q", got)
	}
	// Writing a managed layer regenerates the merged output.
	if got := ts.Repo.Read(t, "apps", "alpha", "values.yaml"); !strings.Contains(got, "replicaCount: 3") {
		t.Fatalf("values.yaml not regenerated after layer write:\n%s", got)
	}

	ts.get("/api/instances/alpha/file?path=absent.yaml").expect(http.StatusNotFound)
	ts.get("/api/instances/alpha/file").expect(http.StatusBadRequest)
}

// Malformed YAML/JSON must be rejected before it reaches disk — otherwise a
// direct-mode instance persists a broken values file silently.
func TestFiles_WriteRejectsBrokenSyntax(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	ts.createInstance("alpha")

	good := ts.Repo.Read(t, "apps", "alpha", "values.instance.yaml")

	// Broken YAML (unbalanced bracket) is refused, and the file is untouched.
	msg := ts.put("/api/instances/alpha/file?path=values.instance.yaml", "replicaCount: [1, 2\nfoo: :bar\n").
		expect(http.StatusBadRequest).errorMessage()
	if !strings.Contains(msg, "invalid YAML") {
		t.Fatalf("expected an 'invalid YAML' error, got %q", msg)
	}
	if got := ts.Repo.Read(t, "apps", "alpha", "values.instance.yaml"); got != good {
		t.Fatalf("broken write must not touch the file on disk, got %q", got)
	}

	// Broken JSON is refused too.
	ts.put("/api/instances/alpha/file?path=values.schema.json", "{not json").
		expect(http.StatusBadRequest)

	// A values file that parses as a bare scalar or a list is valid YAML but
	// broken config — dropping a ':' does exactly this — and must be refused.
	for _, body := range []string{"just a broken string", "- a\n- b\n"} {
		ts.put("/api/instances/alpha/file?path=values.instance.yaml", body).
			expect(http.StatusBadRequest)
	}
	if got := ts.Repo.Read(t, "apps", "alpha", "values.instance.yaml"); got != good {
		t.Fatalf("a rejected scalar/list write must not touch the file, got %q", got)
	}

	// Empty (null) values are allowed.
	ts.put("/api/instances/alpha/file?path=values.instance.yaml", "").expect(http.StatusNoContent)

	// A Helm template (under templates/) is a Go template, not plain YAML —
	// its {{ }} must not be rejected as invalid YAML.
	ts.put("/api/instances/alpha/file?path=templates/cm.yaml", "data:\n  x: {{ .Values.x }}\n").
		expect(http.StatusNoContent)

	// Valid YAML still writes.
	ts.put("/api/instances/alpha/file?path=values.instance.yaml", "replicaCount: 3\n").
		expect(http.StatusNoContent)
}

func TestFiles_ListSkipsVendoredChartsContents(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	ts.createInstance("alpha")

	chartsDir := filepath.Join(ts.instancePath("alpha"), "charts")
	if err := os.MkdirAll(chartsDir, 0o755); err != nil {
		t.Fatalf("mkdir charts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(chartsDir, "postgresql-15.5.0.tgz"), []byte("bulk"), 0o644); err != nil {
		t.Fatalf("write vendored chart: %v", err)
	}

	var files []fileInfo
	ts.get("/api/instances/alpha/files").expect(http.StatusOK).decode(&files)
	for _, f := range files {
		if strings.HasPrefix(f.Path, "charts"+string(filepath.Separator)) {
			t.Fatalf("vendored charts contents must not be listed, got %q", f.Path)
		}
	}
}
