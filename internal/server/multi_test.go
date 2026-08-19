package server

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"helmdex/internal/config"
	"helmdex/internal/instances"
	"helmdex/internal/testutil"
)

// testMulti is a Multi mounted on a real HTTP listener with helpers to open
// throwaway repo workspaces.
type testMulti struct {
	*httptest.Server
	t   *testing.T
	mgr *Multi
}

func newTestMulti(t *testing.T) *testMulti {
	t.Helper()
	testutil.Hermetic(t)
	mgr := NewMulti()
	listener := httptest.NewServer(mgr.Handler())
	t.Cleanup(listener.Close)
	return &testMulti{Server: listener, t: t, mgr: mgr}
}

// openRepo creates a throwaway repo and opens a workspace for it, resolving
// config exactly like the desktop shell does.
func (tm *testMulti) openRepo() (id string, root string) {
	tm.t.Helper()
	repo := testutil.NewRepo(tm.t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	res, err := config.Resolve(repo.Root, "")
	if err != nil {
		tm.t.Fatalf("resolve config: %v", err)
	}
	id, existed := tm.mgr.Open(Params{
		RepoRoot: repo.Root,
		Config:   instances.ApplyLayout(repo.Root, res),
		Resolved: res,
	})
	if existed {
		tm.t.Fatalf("fresh repo %s reported as already open", repo.Root)
	}
	return id, repo.Root
}

func (tm *testMulti) getJSON(path string, v any) int {
	tm.t.Helper()
	res, err := tm.Client().Get(tm.URL + path)
	if err != nil {
		tm.t.Fatalf("GET %s: %v", path, err)
	}
	defer res.Body.Close() //nolint:errcheck // response body close
	if v != nil {
		if err := json.NewDecoder(res.Body).Decode(v); err != nil {
			tm.t.Fatalf("decode %s: %v", path, err)
		}
	}
	return res.StatusCode
}

func TestMulti_RoutesEachWorkspaceToItsRepo(t *testing.T) {
	tm := newTestMulti(t)
	idA, rootA := tm.openRepo()
	idB, rootB := tm.openRepo()

	var repoA, repoB struct {
		Root string `json:"root"`
	}
	if status := tm.getJSON("/ws/"+idA+"/api/repo", &repoA); status != http.StatusOK {
		t.Fatalf("workspace A repo status = %d", status)
	}
	if status := tm.getJSON("/ws/"+idB+"/api/repo", &repoB); status != http.StatusOK {
		t.Fatalf("workspace B repo status = %d", status)
	}
	if repoA.Root != rootA || repoB.Root != rootB {
		t.Fatalf("routing mixed workspaces: A=%q (want %q), B=%q (want %q)",
			repoA.Root, rootA, repoB.Root, rootB)
	}
}

func TestMulti_WorkspacesAreIsolated(t *testing.T) {
	tm := newTestMulti(t)
	idA, _ := tm.openRepo()
	idB, _ := tm.openRepo()

	// Create an instance in A through the prefixed API.
	res, err := tm.Client().Post(tm.URL+"/ws/"+idA+"/api/instances", "application/json",
		strings.NewReader(`{"name":"alpha"}`))
	if err != nil {
		t.Fatalf("create instance: %v", err)
	}
	res.Body.Close() //nolint:errcheck // response body close
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create instance status = %d", res.StatusCode)
	}

	var listA, listB []struct {
		Name string `json:"name"`
	}
	tm.getJSON("/ws/"+idA+"/api/instances", &listA)
	tm.getJSON("/ws/"+idB+"/api/instances", &listB)
	if len(listA) != 1 || listA[0].Name != "alpha" {
		t.Fatalf("workspace A instances = %+v, want [alpha]", listA)
	}
	if len(listB) != 0 {
		t.Fatalf("workspace B sees A's instances: %+v", listB)
	}
}

func TestMulti_OpenDedupsByRoot(t *testing.T) {
	tm := newTestMulti(t)
	idA, rootA := tm.openRepo()

	id2, existed := tm.mgr.Open(Params{RepoRoot: rootA + "/."})
	if !existed || id2 != idA {
		t.Fatalf("re-open returned (%q, existed=%v), want (%q, true)", id2, existed, idA)
	}
	if got := len(tm.mgr.List()); got != 1 {
		t.Fatalf("workspaces after dedup = %d, want 1", got)
	}
}

func TestMulti_OpenDedupsAcrossSymlinkAliases(t *testing.T) {
	tm := newTestMulti(t)
	idA, rootA := tm.openRepo()

	link := filepath.Join(t.TempDir(), "linked-repo")
	if err := os.Symlink(rootA, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	// Two aliases of one physical folder must share ONE workspace — separate
	// workspaces would hold separate write locks over the same files.
	id2, existed := tm.mgr.Open(Params{RepoRoot: link})
	if !existed || id2 != idA {
		t.Fatalf("symlink alias opened a second workspace: (%q, existed=%v), want (%q, true)",
			id2, existed, idA)
	}
}

func TestMulti_HelmEventForwardingStopIsIdempotent(t *testing.T) {
	mgr := NewMulti()
	stop := mgr.StartHelmEventForwarding()
	stop()
	stop() // second call must not panic (close of closed channel)
}

func TestMulti_CloseRemovesWorkspace(t *testing.T) {
	tm := newTestMulti(t)
	idA, rootA := tm.openRepo()
	idB, rootB := tm.openRepo()

	if err := tm.mgr.Close(idA); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := tm.mgr.Close(idA); err == nil {
		t.Fatal("closing an unknown workspace should error")
	}

	list := tm.mgr.List()
	if len(list) != 1 || list[0].ID != idB || list[0].Root != rootB {
		t.Fatalf("list after close = %+v, want [{%s %s}]", list, idB, rootB)
	}
	if status := tm.getJSON("/ws/"+idA+"/api/repo", nil); status != http.StatusNotFound {
		t.Fatalf("closed workspace status = %d, want 404", status)
	}

	// The root can be reopened afterwards, under a fresh id.
	idNew, existed := tm.mgr.Open(Params{RepoRoot: rootA})
	if existed || idNew == idA {
		t.Fatalf("reopen after close = (%q, existed=%v), want fresh id", idNew, existed)
	}
}

func TestMulti_UnknownWorkspaceAndBareAPIAre404(t *testing.T) {
	tm := newTestMulti(t)
	tm.openRepo()

	var apiErr struct {
		Error string `json:"error"`
	}
	if status := tm.getJSON("/ws/w999/api/repo", &apiErr); status != http.StatusNotFound {
		t.Fatalf("unknown workspace status = %d, want 404", status)
	}
	if !strings.Contains(apiErr.Error, "unknown workspace") {
		t.Fatalf("unknown workspace error = %q", apiErr.Error)
	}

	// Un-prefixed API calls are a caller bug — explicit 404, not the SPA
	// fallback silently serving index.html.
	if status := tm.getJSON("/api/repo", &apiErr); status != http.StatusNotFound {
		t.Fatalf("bare /api status = %d, want 404", status)
	}
	if !strings.Contains(apiErr.Error, "/ws/<id>") {
		t.Fatalf("bare /api error should point at the prefixed form, got %q", apiErr.Error)
	}
}

func TestMulti_ServesSPAAtRoot(t *testing.T) {
	tm := newTestMulti(t)

	res, err := tm.Client().Get(tm.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer res.Body.Close() //nolint:errcheck // response body close
	if res.StatusCode != http.StatusOK {
		t.Fatalf("SPA status = %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("SPA Content-Type = %q", ct)
	}
}

func TestMulti_EventsFanOutToEveryWorkspace(t *testing.T) {
	tm := newTestMulti(t)
	idA, _ := tm.openRepo()
	idB, _ := tm.openRepo()

	esA := tm.openEvents(idA)
	esB := tm.openEvents(idB)

	tm.mgr.publishAll(event{Type: "helm.download_start", Message: "v9.9.9"})

	for name, es := range map[string]*multiEventStream{"A": esA, "B": esB} {
		ev := es.next(5 * time.Second)
		if ev.Type != "helm.download_start" || ev.Message != "v9.9.9" {
			t.Fatalf("workspace %s event = %+v", name, ev)
		}
	}
}

// multiEventStream reads one workspace's SSE stream through the Multi handler.
type multiEventStream struct {
	t     *testing.T
	lines chan string
}

func (tm *testMulti) openEvents(id string) *multiEventStream {
	tm.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	tm.t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tm.URL+"/ws/"+id+"/api/events", nil)
	if err != nil {
		tm.t.Fatalf("build events request: %v", err)
	}
	res, err := tm.Client().Do(req)
	if err != nil {
		tm.t.Fatalf("GET events for %s: %v", id, err)
	}
	if res.StatusCode != http.StatusOK {
		tm.t.Fatalf("events status for %s = %d", id, res.StatusCode)
	}

	es := &multiEventStream{t: tm.t, lines: make(chan string, 16)}
	ready := make(chan struct{})
	go func() {
		defer res.Body.Close() //nolint:errcheck // stream body close
		scanner := bufio.NewScanner(res.Body)
		opened := false
		for scanner.Scan() {
			line := scanner.Text()
			if !opened && strings.HasPrefix(line, ":") {
				opened = true
				close(ready)
				continue
			}
			if data, ok := strings.CutPrefix(line, "data: "); ok {
				es.lines <- data
			}
		}
	}()
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		tm.t.Fatalf("timed out waiting for %s event stream preamble", id)
	}
	return es
}

func (es *multiEventStream) next(timeout time.Duration) event {
	es.t.Helper()
	select {
	case raw := <-es.lines:
		var ev event
		if err := json.Unmarshal([]byte(raw), &ev); err != nil {
			es.t.Fatalf("decode event %q: %v", raw, err)
		}
		return ev
	case <-time.After(timeout):
		es.t.Fatal("timed out waiting for an event")
		return event{}
	}
}
