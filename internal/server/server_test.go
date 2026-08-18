package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"helmdex/internal/config"
	"helmdex/internal/instances"
	"helmdex/internal/testutil"
)

// testServer is a Server mounted on a real HTTP listener over a throwaway
// repo. Going through a real listener matters: several behaviours under test
// (percent-encoded path segments, event streaming) only exist once requests
// travel the net/http stack.
type testServer struct {
	*httptest.Server

	t       *testing.T
	Repo    testutil.Repo
	API     *Server
	HelmLog string
}

// newTestServer wires a Server exactly like `helmdex ui` does: resolve the
// config chain, apply layout discovery, serve.
func newTestServer(t *testing.T, opts testutil.RepoOpts) *testServer {
	t.Helper()

	helmLog := testutil.Hermetic(t)
	repo := testutil.NewRepo(t, opts)

	// Resolve with no explicit path, like `helmdex ui` without --config: the
	// chain picks up the repo's helmdex.yaml, and only then does ApplyLayout
	// run layout discovery (an explicit --config is trusted as-is).
	res, err := config.Resolve(repo.Root, "")
	if err != nil {
		t.Fatalf("resolve config: %v", err)
	}
	api := New(Params{
		RepoRoot: repo.Root,
		Config:   instances.ApplyLayout(repo.Root, res),
		Resolved: res,
	})

	listener := httptest.NewServer(api.Handler())
	t.Cleanup(listener.Close)
	// Assert on what each handler returns, not on what a redirect chain ends
	// up doing — following a 301 would silently turn a DELETE into a GET.
	listener.Client().CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	return &testServer{Server: listener, t: t, Repo: repo, API: api, HelmLog: helmLog}
}

// response is one API exchange, kept as raw bytes so tests can assert both
// status codes and payloads.
type response struct {
	t      *testing.T
	Status int
	Body   []byte
	Header http.Header
}

// do issues a request. A nil body sends no payload; a string or []byte body is
// sent verbatim; anything else is JSON-encoded.
func (ts *testServer) do(method, path string, body any) *response {
	ts.t.Helper()

	var reader io.Reader
	switch b := body.(type) {
	case nil:
	case []byte:
		reader = bytes.NewReader(b)
	case string:
		reader = strings.NewReader(b)
	default:
		buf, err := json.Marshal(b)
		if err != nil {
			ts.t.Fatalf("marshal request body: %v", err)
		}
		reader = bytes.NewReader(buf)
	}

	req, err := http.NewRequest(method, ts.URL+path, reader)
	if err != nil {
		ts.t.Fatalf("build request %s %s: %v", method, path, err)
	}
	if reader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	recordRoute(ts.API.mux, req)
	res, err := ts.Client().Do(req)
	if err != nil {
		ts.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close() //nolint:errcheck // response body close
	b, err := io.ReadAll(res.Body)
	if err != nil {
		ts.t.Fatalf("read %s %s body: %v", method, path, err)
	}
	return &response{t: ts.t, Status: res.StatusCode, Body: b, Header: res.Header}
}

func (ts *testServer) get(path string) *response { return ts.do(http.MethodGet, path, nil) }

func (ts *testServer) post(path string, body any) *response {
	return ts.do(http.MethodPost, path, body)
}

func (ts *testServer) put(path string, body any) *response {
	return ts.do(http.MethodPut, path, body)
}

func (ts *testServer) del(path string, body any) *response {
	return ts.do(http.MethodDelete, path, body)
}

// expect asserts the status code, reporting the body on mismatch since it
// carries the API's error message.
func (r *response) expect(status int) *response {
	r.t.Helper()
	if r.Status != status {
		r.t.Fatalf("status = %d, want %d; body: %s", r.Status, status, r.Body)
	}
	return r
}

// decode unmarshals a JSON response into v.
func (r *response) decode(v any) *response {
	r.t.Helper()
	if err := json.Unmarshal(r.Body, v); err != nil {
		r.t.Fatalf("decode response %q: %v", r.Body, err)
	}
	return r
}

// errorMessage returns the `error` field of an API error payload.
func (r *response) errorMessage() string {
	r.t.Helper()
	var e apiError
	if err := json.Unmarshal(r.Body, &e); err != nil {
		r.t.Fatalf("decode error response %q: %v", r.Body, err)
	}
	return e.Error
}

func (r *response) text() string { return string(r.Body) }

// as rebinds the harness to a subtest, so its failures are reported against
// that subtest. Calling Fatalf on the parent from inside t.Run stops the whole
// table after the first failing case and hides the rest.
func (ts *testServer) as(t *testing.T) *testServer {
	sub := *ts
	sub.t = t
	return &sub
}

// instancePath is the on-disk directory of an instance, for filesystem
// assertions.
func (ts *testServer) instancePath(name string) string {
	return ts.Repo.InstancePath(name)
}

// createInstance creates an instance through the API and returns its info.
func (ts *testServer) createInstance(name string) instanceInfo {
	ts.t.Helper()
	var info instanceInfo
	ts.post("/api/instances", map[string]string{"name": name}).
		expect(http.StatusCreated).
		decode(&info)
	return info
}
