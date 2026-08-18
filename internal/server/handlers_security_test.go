package server

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"helmdex/internal/testutil"
)

// The HTTP layer is where instance names stop being opaque tokens: net/http
// decodes %2F inside a path segment, so `{name}` can carry separators and
// `..`. instances.ValidateName is the guard; these tests pin it at the layer
// that made it necessary.

// seedSentinel plants a directory inside the repo but outside the apps dir.
// `apps/<name>` with name="../sentinel" resolves onto it, so any handler that
// skips validation would read, write or delete it.
func seedSentinel(t *testing.T, r testutil.Repo) {
	r.Write(t, "apiVersion: v2\nname: sentinel\nversion: 0.1.0\n", "sentinel", "Chart.yaml")
	r.Write(t, "token: s3cret\n", "sentinel", "secret.yaml")
}

func newSecurityServer(t *testing.T) *testServer {
	t.Helper()
	return newTestServer(t, testutil.RepoOpts{
		SourceMode: testutil.SourceNone,
		Seed:       seedSentinel,
	})
}

// traversalNames are percent-encoded names that survive URL path
// normalization and reach the handler as a single segment containing
// separators or "..". Unencoded "." and ".." are collapsed by the client and
// ServeMux before routing, so they never test the handler guard.
var traversalNames = []string{
	"..%2Fsentinel",
	"..%2F..%2Fetc",
	"%2E%2E%2Fsentinel",
	"a%2Fb",
	"%2Fetc%2Fpasswd",
	"%2E%2E",
}

func TestTraversal_InstanceReadIsRejected(t *testing.T) {
	ts := newSecurityServer(t)

	for _, name := range traversalNames {
		t.Run(name, func(t *testing.T) {
			ts := ts.as(t)
			ts.get("/api/instances/" + name).expect(http.StatusNotFound)
			ts.get("/api/instances/" + name + "/files").expect(http.StatusNotFound)
			ts.get("/api/instances/" + name + "/file?path=secret.yaml").expect(http.StatusNotFound)
			ts.get("/api/instances/" + name + "/values").expect(http.StatusNotFound)
			ts.get("/api/instances/" + name + "/deps").expect(http.StatusNotFound)
			ts.get("/api/instances/" + name + "/sets").expect(http.StatusNotFound)
		})
	}

	if !ts.Repo.Exists(t, "sentinel", "secret.yaml") {
		t.Fatal("sentinel disappeared")
	}
}

func TestTraversal_InstanceDeleteDoesNotEscapeAppsDir(t *testing.T) {
	ts := newSecurityServer(t)

	for _, name := range traversalNames {
		t.Run(name, func(t *testing.T) {
			ts := ts.as(t)
			res := ts.del("/api/instances/"+name, nil)
			if res.Status != http.StatusNotFound && res.Status != http.StatusBadRequest {
				t.Fatalf("status = %d, want 404 or 400; body: %s", res.Status, res.Body)
			}
			if !ts.Repo.Exists(t, "sentinel", "secret.yaml") {
				t.Fatalf("DELETE with name %q removed files outside the apps dir", name)
			}
		})
	}
}

func TestTraversal_InstanceApplyDoesNotEscapeAppsDir(t *testing.T) {
	ts := newSecurityServer(t)

	for _, name := range traversalNames {
		t.Run(name, func(t *testing.T) {
			ts := ts.as(t)
			ts.post("/api/instances/"+name+"/apply", nil).expect(http.StatusNotFound)
		})
	}
	// Apply on a chart dir would relock and vendor charts into it.
	if ts.Repo.Exists(t, "sentinel", "Chart.lock") {
		t.Fatal("apply reached a chart outside the apps dir")
	}
}

func TestTraversal_InstanceRenameDoesNotEscapeAppsDir(t *testing.T) {
	ts := newSecurityServer(t)
	ts.createInstance("alpha")

	// Escaping through the source name.
	ts.post("/api/instances/..%2Fsentinel/rename", map[string]string{"newName": "stolen"}).
		expect(http.StatusBadRequest)
	if ts.Repo.Exists(t, "apps", "stolen") || ts.Repo.Exists(t, "stolen") {
		t.Fatal("rename escaped the apps dir")
	}
	if !ts.Repo.Exists(t, "sentinel", "Chart.yaml") {
		t.Fatal("rename moved a directory outside the apps dir")
	}

	// Escaping through the target name.
	ts.post("/api/instances/alpha/rename", map[string]string{"newName": "../escaped"}).
		expect(http.StatusBadRequest)
	if ts.Repo.Exists(t, "escaped") {
		t.Fatal("rename wrote outside the apps dir")
	}
	if !ts.Repo.Exists(t, "apps", "alpha", "Chart.yaml") {
		t.Fatal("a rejected rename must leave the source instance intact")
	}
}

func TestTraversal_InstanceCreateIsRejected(t *testing.T) {
	ts := newSecurityServer(t)

	for _, name := range []string{"../escaped", "..", "a/b", `a\b`, "x/../y", "."} {
		t.Run(name, func(t *testing.T) {
			ts := ts.as(t)
			ts.post("/api/instances", map[string]string{"name": name}).
				expect(http.StatusBadRequest)
		})
	}
	if ts.Repo.Exists(t, "escaped") {
		t.Fatal("create wrote outside the apps dir")
	}
}

// resolveInstanceFile does not reject traversal, it clamps: the path is
// re-rooted at the instance directory, so "../../helmdex.yaml" addresses
// <instance>/helmdex.yaml. What matters is that nothing outside the instance
// is ever read or written.
func TestTraversal_FilePathIsClampedToInstance(t *testing.T) {
	ts := newSecurityServer(t)
	ts.createInstance("alpha")

	const canary = "owned: true\n"
	for _, p := range []string{
		"../../helmdex.yaml",
		"..%2F..%2Fhelmdex.yaml",
		"../sentinel/secret.yaml",
		"a/../../../helmdex.yaml",
		"/etc/passwd",
	} {
		t.Run(p, func(t *testing.T) {
			ts := ts.as(t)
			ts.put("/api/instances/alpha/file?path="+p, canary)
		})
	}

	if got := ts.Repo.Read(t, "helmdex.yaml"); got == canary {
		t.Fatal("the repo config was overwritten through a traversal path")
	}
	if got := ts.Repo.Read(t, "sentinel", "secret.yaml"); got != "token: s3cret\n" {
		t.Fatalf("sentinel file was modified: %q", got)
	}
	// The writes landed inside the instance, re-rooted.
	if !ts.Repo.Exists(t, "apps", "alpha", "helmdex.yaml") {
		t.Fatal("expected the traversal write to be clamped into the instance dir")
	}
	if !ts.Repo.Exists(t, "apps", "alpha", "sentinel", "secret.yaml") {
		t.Fatal("expected the traversal write to be clamped into the instance dir")
	}
}

// The dependency id is the second user-supplied path component: it names the
// depmeta file and the values.dep-set markers. It arrives either as a URL
// segment or as the `alias` of an added dependency, and both reached the
// filesystem unvalidated.
func TestTraversal_DependencyIDCannotEscapeStateDir(t *testing.T) {
	ts := newSecurityServer(t)
	ts.createInstance("alpha")

	// A canary outside the repo entirely: depmeta lives under the state dir,
	// which for an opted-in repo is <repo>/.helmdex.
	outside := filepath.Join(t.TempDir(), "canary.yaml")
	if err := os.WriteFile(outside, []byte("untouched\n"), 0o644); err != nil {
		t.Fatalf("seed canary: %v", err)
	}
	rel, err := filepath.Rel(filepath.Join(ts.Repo.Root, ".helmdex", "depmeta", "alpha"), outside)
	if err != nil {
		t.Fatalf("relative canary path: %v", err)
	}
	escaping := strings.TrimSuffix(rel, ".yaml")

	// --- through the alias of an added dependency ---
	res := ts.post("/api/instances/alpha/deps", map[string]any{
		"name":       "nginx",
		"repository": "https://example.invalid/charts",
		"version":    "15.0.0",
		"alias":      escaping,
	}).expect(http.StatusBadRequest)
	if !strings.Contains(res.errorMessage(), "dependency id") {
		t.Fatalf("unexpected error: %s", res.errorMessage())
	}
	// Rejected before anything is written: no partial state.
	if chart := ts.Repo.Read(t, "apps", "alpha", "Chart.yaml"); strings.Contains(chart, "nginx") {
		t.Fatalf("a rejected alias still reached Chart.yaml:\n%s", chart)
	}

	// --- through the {depID} URL segment ---
	for _, path := range []string{
		"/api/instances/alpha/deps/" + url.PathEscape(escaping) + "/detach",
		"/api/instances/alpha/deps/" + url.PathEscape(escaping) + "/version",
	} {
		res := ts.post(path, map[string]any{"version": "1.0.0"})
		if res.Status == http.StatusOK {
			t.Fatalf("%s accepted an escaping dependency id", path)
		}
	}

	if got := readFile(t, outside); got != "untouched\n" {
		t.Fatalf("canary outside the repo was written: %q", got)
	}
}

// The same class, third channel: a source name becomes a directory and a file
// name in the state dir, and it is settable over the API.
func TestTraversal_SourceNameCannotEscapeStateDir(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceGit})

	var info sourcesInfo
	ts.get("/api/config/sources").expect(http.StatusOK).decode(&info)

	outsideDir := t.TempDir()
	canary := filepath.Join(outsideDir, "hijacked.yaml")
	if err := os.WriteFile(canary, []byte("untouched\n"), 0o644); err != nil {
		t.Fatalf("seed canary: %v", err)
	}
	rel, err := filepath.Rel(filepath.Join(ts.Repo.Root, ".helmdex", "catalog"), canary)
	if err != nil {
		t.Fatalf("relative canary path: %v", err)
	}

	info.Sources[0].Name = strings.TrimSuffix(rel, ".yaml")
	res := ts.put("/api/config/sources", sourcesPutRequest{Platform: info.Platform, Sources: info.Sources}).
		expect(http.StatusBadRequest)
	if !strings.Contains(res.errorMessage(), "sources[].name") {
		t.Fatalf("unexpected error: %s", res.errorMessage())
	}

	// The config was not persisted, so a later sync cannot pick it up either.
	if cfg := ts.Repo.Read(t, "helmdex.yaml"); strings.Contains(cfg, "hijacked") {
		t.Fatalf("an escaping source name was written to the config:\n%s", cfg)
	}
	ts.post("/api/catalog/sync", nil)
	if got := readFile(t, canary); got != "untouched\n" {
		t.Fatalf("canary outside the repo was written: %q", got)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func TestResolveInstanceFile(t *testing.T) {
	inst := filepath.Join("/repo", "apps", "alpha")

	tests := []struct {
		name    string
		rel     string
		want    string
		wantErr bool
	}{
		{"plain file", "values.yaml", filepath.Join(inst, "values.yaml"), false},
		{"nested file", "templates/netpol.yaml", filepath.Join(inst, "templates", "netpol.yaml"), false},
		{"empty is rejected", "", "", true},
		// Everything below is clamped back into the instance directory.
		{"parent", "../secret.yaml", filepath.Join(inst, "secret.yaml"), false},
		{"deep parent", "a/../../secret.yaml", filepath.Join(inst, "secret.yaml"), false},
		{"absolute", "/etc/passwd", filepath.Join(inst, "etc", "passwd"), false},
		{"dot slash", "./values.yaml", filepath.Join(inst, "values.yaml"), false},
		{"climbs to root", "a/b/../..", inst, false},
		{"bare parent", "..", inst, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveInstanceFile(inst, tc.rel)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSetMarkerPath_RejectsSeparators(t *testing.T) {
	inst := filepath.Join("/repo", "apps", "alpha")

	for _, req := range []setRequest{
		{Set: ""},
		{Set: "  "},
		{Set: "../evil"},
		{Set: `..\evil`},
		{Set: "ok", DepID: "../evil"},
	} {
		t.Run(req.Set+"|"+req.DepID, func(t *testing.T) {
			if _, err := setMarkerPath(inst, req); err == nil {
				t.Fatalf("expected %+v to be rejected", req)
			}
		})
	}

	got, err := setMarkerPath(inst, setRequest{Set: "prod"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := filepath.Join(inst, "values.set.prod.yaml"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSets_TraversalIsRejectedOverHTTP(t *testing.T) {
	ts := newSecurityServer(t)
	ts.createInstance("alpha")

	ts.post("/api/instances/alpha/sets", map[string]string{"set": "../escaped"}).
		expect(http.StatusBadRequest)
	if ts.Repo.Exists(t, "apps", "values.set.escaped.yaml") {
		t.Fatal("set marker written outside the instance dir")
	}

	ts.del("/api/instances/alpha/sets", map[string]string{"set": "..", "depID": "../x"}).
		expect(http.StatusBadRequest)
}
