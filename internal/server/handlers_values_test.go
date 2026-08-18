package server

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"helmdex/internal/testutil"
)

// Managed and direct instances differ in one decisive way: managed instances
// own a generated values.yaml, direct ones (the default in helmdex-agnostic
// repos) own every values file themselves and helmdex may only edit them in
// place, minimally.

func TestValues_ManagedEditsLayerAndRegenerates(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	ts.createInstance("alpha")

	var got map[string]any
	ts.get("/api/instances/alpha/values").expect(http.StatusOK).decode(&got)
	if got["file"] != "values.instance.yaml" {
		t.Fatalf("managed instances must be edited through values.instance.yaml, got %v", got["file"])
	}

	ts.put("/api/instances/alpha/values", map[string]any{
		"path":  "$.image.tag",
		"value": "1.2.3",
	}).expect(http.StatusNoContent)

	if layer := ts.Repo.Read(t, "apps", "alpha", "values.instance.yaml"); !strings.Contains(layer, "tag: 1.2.3") {
		t.Fatalf("value not written to the edit layer:\n%s", layer)
	}
	if merged := ts.Repo.Read(t, "apps", "alpha", "values.yaml"); !strings.Contains(merged, "tag: 1.2.3") {
		t.Fatalf("merged values.yaml not regenerated:\n%s", merged)
	}

	ts.get("/api/instances/alpha/values?path=$.image.tag").expect(http.StatusOK).decode(&got)
	if got["found"] != true || got["value"] != "1.2.3" {
		t.Fatalf("read back %v", got)
	}
}

func TestValues_RegenCanBeDeferred(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	ts.createInstance("alpha")

	ts.put("/api/instances/alpha/values", map[string]any{
		"path":  "$.replicaCount",
		"value": 4,
		"regen": false,
	}).expect(http.StatusNoContent)

	if merged := ts.Repo.Read(t, "apps", "alpha", "values.yaml"); strings.Contains(merged, "replicaCount: 4") {
		t.Fatalf("regen:false must leave values.yaml stale:\n%s", merged)
	}

	ts.post("/api/instances/alpha/values/regen", nil).expect(http.StatusNoContent)
	if merged := ts.Repo.Read(t, "apps", "alpha", "values.yaml"); !strings.Contains(merged, "replicaCount: 4") {
		t.Fatalf("explicit regen did not refresh values.yaml:\n%s", merged)
	}
}

func TestValues_DirectModeEditsInPlaceAndNeverGenerates(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{Agnostic: true})

	var info instanceInfo
	ts.get("/api/instances/demo-preprod").expect(http.StatusOK).decode(&info)
	if info.Managed {
		t.Fatal("an instance without values.instance.yaml must be direct-mode")
	}

	var got map[string]any
	ts.get("/api/instances/demo-preprod/values").expect(http.StatusOK).decode(&got)
	if got["file"] != "values.yaml" {
		t.Fatalf("direct instances are edited through values.yaml, got %v", got["file"])
	}

	before := ts.Repo.Read(t, "apps", "demo-preprod", "values.yaml")

	ts.put("/api/instances/demo-preprod/values", map[string]any{
		"path":  "$.demo.replicaCount",
		"value": 3,
	}).expect(http.StatusNoContent)

	after := ts.Repo.Read(t, "apps", "demo-preprod", "values.yaml")
	if after == before {
		t.Fatal("the edit was not applied")
	}
	if !strings.Contains(after, "replicaCount: 3") {
		t.Fatalf("edit not applied:\n%s", after)
	}

	// Minimal diff: only the touched line changes. Comments, key order and
	// untouched keys survive.
	for _, want := range []string{
		"# Wrapper defaults (hand-written, user-owned — helmdex must never regenerate this).",
		"# keep small in preprod",
		"repository: registry.example.invalid/org/demo",
		"# NetworkPolicies toggle.",
		"enabled: true",
	} {
		if !strings.Contains(after, want) {
			t.Fatalf("minimal-diff edit dropped %q:\n%s", want, after)
		}
	}
	if diff := countChangedLines(before, after); diff != 1 {
		t.Fatalf("expected exactly one changed line, got %d\nbefore:\n%s\nafter:\n%s", diff, before, after)
	}

	// helmdex must not add anything of its own to an agnostic repo.
	if ts.Repo.Exists(t, "apps", "demo-preprod", "values.instance.yaml") {
		t.Fatal("direct-mode instance gained a managed layer file")
	}
	if ts.Repo.Exists(t, ".helmdex") {
		t.Fatal("helmdex wrote in-repo state into an agnostic repo")
	}
}

func TestValues_DirectModeRegenIsANoop(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{Agnostic: true})

	before := ts.Repo.Read(t, "apps", "demo-preprod", "values.yaml")
	ts.post("/api/instances/demo-preprod/values/regen", nil).expect(http.StatusNoContent)
	if after := ts.Repo.Read(t, "apps", "demo-preprod", "values.yaml"); after != before {
		t.Fatalf("regen rewrote a user-owned values.yaml:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestValues_RejectsBadPath(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	ts.createInstance("alpha")

	ts.get("/api/instances/alpha/values?path=" + url.QueryEscape("$.a[")).expect(http.StatusBadRequest)
	ts.put("/api/instances/alpha/values", map[string]any{"path": "$.a[", "value": 1}).
		expect(http.StatusBadRequest)
}

func TestValues_MissingPathReportsNotFound(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	ts.createInstance("alpha")

	var got map[string]any
	ts.get("/api/instances/alpha/values?path=$.absent.key").expect(http.StatusOK).decode(&got)
	if got["found"] != false {
		t.Fatalf("expected found:false, got %v", got)
	}
}

// countChangedLines counts lines that differ between two texts, treating
// added or removed lines as changes.
func countChangedLines(before, after string) int {
	b := strings.Split(before, "\n")
	a := strings.Split(after, "\n")
	n := 0
	for i := 0; i < len(b) || i < len(a); i++ {
		var lb, la string
		if i < len(b) {
			lb = b[i]
		}
		if i < len(a) {
			la = a[i]
		}
		if lb != la {
			n++
		}
	}
	return n
}
