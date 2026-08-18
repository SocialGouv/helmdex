package server

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"helmdex/internal/testutil"
)

// eventStream is an open /api/events subscription.
type eventStream struct {
	t      *testing.T
	cancel context.CancelFunc
	lines  chan string
	errs   chan error
}

// openEvents subscribes and waits for the stream to be live, so a mutation
// issued afterwards is guaranteed to be observed.
func openEvents(ts *testServer) *eventStream {
	ts.t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/events", nil)
	if err != nil {
		cancel()
		ts.t.Fatalf("build events request: %v", err)
	}
	recordRoute(ts.API.mux, req)
	res, err := ts.Client().Do(req)
	if err != nil {
		cancel()
		ts.t.Fatalf("GET /api/events: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		cancel()
		ts.t.Fatalf("events status = %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		cancel()
		ts.t.Fatalf("events Content-Type = %q", ct)
	}

	es := &eventStream{t: ts.t, cancel: cancel, lines: make(chan string, 64), errs: make(chan error, 1)}
	ready := make(chan struct{})
	go func() {
		defer res.Body.Close() //nolint:errcheck // stream body close
		scanner := bufio.NewScanner(res.Body)
		opened := false
		for scanner.Scan() {
			line := scanner.Text()
			if !opened && strings.HasPrefix(line, ":") {
				// The handler's ": connected" preamble is flushed after the
				// subscription exists.
				opened = true
				close(ready)
				continue
			}
			if data, ok := strings.CutPrefix(line, "data: "); ok {
				es.lines <- data
			}
		}
		if err := scanner.Err(); err != nil && ctx.Err() == nil {
			es.errs <- err
		}
		close(es.lines)
	}()

	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		cancel()
		ts.t.Fatal("timed out waiting for the event stream preamble")
	}
	ts.t.Cleanup(cancel)
	return es
}

// waitFor collects events until one of the wanted types arrives, and returns
// every event seen along the way.
func (es *eventStream) waitFor(wantType string, timeout time.Duration) []event {
	es.t.Helper()
	deadline := time.After(timeout)
	seen := []event{}
	for {
		select {
		case raw, ok := <-es.lines:
			if !ok {
				es.t.Fatalf("event stream closed while waiting for %q; saw %+v", wantType, seen)
			}
			var ev event
			if err := json.Unmarshal([]byte(raw), &ev); err != nil {
				es.t.Fatalf("decode event %q: %v", raw, err)
			}
			seen = append(seen, ev)
			if ev.Type == wantType {
				return seen
			}
		case err := <-es.errs:
			es.t.Fatalf("event stream error: %v", err)
		case <-deadline:
			es.t.Fatalf("timed out waiting for %q; saw %+v", wantType, seen)
		}
	}
}

func TestEvents_ApplyPublishesStartAndDone(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	ts.createInstance("alpha")

	es := openEvents(ts)
	ts.post("/api/instances/alpha/apply", nil).expect(http.StatusOK)

	seen := es.waitFor("apply.done", 10*time.Second)
	if len(seen) < 2 || seen[0].Type != "apply.start" {
		t.Fatalf("expected apply.start then apply.done, got %+v", seen)
	}
	for _, ev := range seen {
		if ev.Instance != "alpha" {
			t.Fatalf("event not scoped to the instance: %+v", ev)
		}
	}
}

func TestEvents_ApplyFailurePublishesError(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	ts.createInstance("alpha")
	addDep(ts, "alpha", map[string]any{
		"name":       "postgresql",
		"repository": fixtureRepoURL,
		"version":    "15.5.0",
	})
	t.Setenv("HELMDEX_FAKE_HELM_FAIL", "dependency=simulated outage")

	es := openEvents(ts)
	ts.post("/api/instances/alpha/apply", nil).expect(http.StatusInternalServerError)

	seen := es.waitFor("apply.error", 10*time.Second)
	last := seen[len(seen)-1]
	if !strings.Contains(last.Message, "simulated outage") {
		t.Fatalf("error event lost the cause: %+v", last)
	}
}

func TestEvents_WorkspaceChangePublishes(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})

	es := openEvents(ts)
	other := testutil.NewRepo(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	ts.API.SetWorkspace(Params{RepoRoot: other.Root, Config: ts.API.Workspace().Config})

	seen := es.waitFor("workspace.changed", 5*time.Second)
	last := seen[len(seen)-1]
	if last.Message != other.Root {
		t.Fatalf("workspace event = %+v, want message %q", last, other.Root)
	}
}

func TestEvents_DisconnectUnsubscribes(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})

	es := openEvents(ts)
	if got := subscriberCount(ts); got != 1 {
		t.Fatalf("subscribers = %d, want 1", got)
	}

	es.cancel()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if subscriberCount(ts) == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("subscriber still registered after disconnect: %d", subscriberCount(ts))
}

func subscriberCount(ts *testServer) int {
	ts.API.events.mu.Lock()
	defer ts.API.events.mu.Unlock()
	return len(ts.API.events.subs)
}
