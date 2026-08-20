package testutil

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"helmdex/internal/ociregistry"
)

// FakeRegistry starts a stand-in OCI registry serving tags per repository path
// (e.g. "org/demo" for oci://any-host/org/demo) and redirects every registry
// host at it for the duration of the test, so chart version listing stays
// hermetic.
//
// Unknown repository paths answer 404, which is what a caller should see for a
// chart that does not exist.
func FakeRegistry(t *testing.T, tags map[string][]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v2/") || !strings.HasSuffix(r.URL.Path, "/tags/list") {
			http.Error(w, "unsupported registry endpoint", http.StatusNotFound)
			return
		}
		repoPath := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v2/"), "/tags/list")
		list, ok := tags[repoPath]
		if !ok {
			http.Error(w, "repository "+repoPath+" not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"name": repoPath, "tags": list})
	}))
	t.Cleanup(srv.Close)
	t.Setenv(ociregistry.FakeRegistryEnv, srv.URL)
	return srv
}
