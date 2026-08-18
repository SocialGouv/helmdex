package server

import (
	"net/http"
	"strings"
	"testing"

	"helmdex/internal/testutil"
)

func TestSets_EnableListDisable(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	ts.createInstance("alpha")

	var info setsInfo
	ts.get("/api/instances/alpha/sets").expect(http.StatusOK).decode(&info)
	if !info.Managed || len(info.Sets) != 0 || len(info.DepSets) != 0 {
		t.Fatalf("unexpected initial sets: %+v", info)
	}

	ts.post("/api/instances/alpha/sets", map[string]string{"set": "prod"}).
		expect(http.StatusCreated)
	if !ts.Repo.Exists(t, "apps", "alpha", "values.set.prod.yaml") {
		t.Fatal("set marker not written")
	}
	// Enabling twice is idempotent.
	ts.post("/api/instances/alpha/sets", map[string]string{"set": "prod"}).
		expect(http.StatusNoContent)

	ts.post("/api/instances/alpha/sets", map[string]string{"set": "ha", "depID": "postgresql"}).
		expect(http.StatusCreated)
	if !ts.Repo.Exists(t, "apps", "alpha", "values.dep-set.postgresql--ha.yaml") {
		t.Fatal("dep-scoped set marker not written")
	}

	ts.get("/api/instances/alpha/sets").expect(http.StatusOK).decode(&info)
	if len(info.Sets) != 1 || info.Sets[0] != "prod" {
		t.Fatalf("sets = %v", info.Sets)
	}
	if got := info.DepSets["postgresql"]; len(got) != 1 || got[0] != "ha" {
		t.Fatalf("depSets = %v", info.DepSets)
	}

	ts.del("/api/instances/alpha/sets", map[string]string{"set": "prod"}).
		expect(http.StatusNoContent)
	if ts.Repo.Exists(t, "apps", "alpha", "values.set.prod.yaml") {
		t.Fatal("set marker not removed")
	}
	// Disabling an absent set is a no-op, not an error.
	ts.del("/api/instances/alpha/sets", map[string]string{"set": "prod"}).
		expect(http.StatusNoContent)
}

func TestSets_EnabledSetIsMergedIntoGeneratedValues(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	ts.createInstance("alpha")

	ts.post("/api/instances/alpha/sets", map[string]string{"set": "prod"}).
		expect(http.StatusCreated)
	ts.put("/api/instances/alpha/file?path=values.set.prod.yaml", "replicaCount: 9\n").
		expect(http.StatusNoContent)

	merged := ts.Repo.Read(t, "apps", "alpha", "values.yaml")
	if !strings.Contains(merged, "replicaCount: 9") {
		t.Fatalf("enabled set not merged into values.yaml:\n%s", merged)
	}

	ts.del("/api/instances/alpha/sets", map[string]string{"set": "prod"}).
		expect(http.StatusNoContent)
	merged = ts.Repo.Read(t, "apps", "alpha", "values.yaml")
	if strings.Contains(merged, "replicaCount: 9") {
		t.Fatalf("disabling a set must regenerate values.yaml without it:\n%s", merged)
	}
}

func TestSets_RejectedInDirectMode(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{Agnostic: true})

	var info setsInfo
	ts.get("/api/instances/demo-preprod/sets").expect(http.StatusOK).decode(&info)
	if info.Managed {
		t.Fatal("agnostic instance reported as managed")
	}

	res := ts.post("/api/instances/demo-preprod/sets", map[string]string{"set": "prod"}).
		expect(http.StatusBadRequest)
	if !strings.Contains(res.errorMessage(), "managed-mode") {
		t.Fatalf("unexpected error: %s", res.errorMessage())
	}
	if ts.Repo.Exists(t, "apps", "demo-preprod", "values.set.prod.yaml") {
		t.Fatal("a set marker was written into a direct-mode instance")
	}
}
