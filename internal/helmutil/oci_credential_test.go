package helmutil

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"helmdex/internal/creds"
	"helmdex/internal/ociregistry"
)

// registryRequiringBasicAuth stands in for a private registry: it answers a
// Basic challenge until the exact expected pair arrives, and records what it
// actually received so a test can pin the credential to the wire.
func registryRequiringBasicAuth(t *testing.T, wantUser, wantSecret string, tags []string) (srv *httptest.Server, got *struct{ user, secret string }) {
	t.Helper()
	got = &struct{ user, secret string }{}
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, secret, ok := r.BasicAuth()
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="registry"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		got.user, got.secret = user, secret
		if user != wantUser || secret != wantSecret {
			http.Error(w, "denied: requested access to the resource is denied", http.StatusForbidden)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"tags": tags})
	}))
	t.Cleanup(srv.Close)
	t.Setenv(ociregistry.FakeRegistryEnv, srv.URL)
	return srv, got
}

// A credential stored through `helmdex auth` must be presented to the registry
// when listing versions — otherwise a private registry silently degrades to
// anonymous and asks the user to sign in for a host they already configured.
func TestOCIChartVersions_UsesStoredCredential(t *testing.T) {
	_, got := registryRequiringBasicAuth(t, "alice", "s3cret", []string{"1.0.0", "2.0.0"})
	t.Setenv("HELMDEX_CREDENTIALS", filepath.Join(t.TempDir(), "credentials.yaml"))
	if err := creds.Upsert(creds.Credential{
		Host:     "registry.example.invalid",
		Kind:     creds.KindOCI,
		Username: "alice",
		Secret:   "s3cret",
	}); err != nil {
		t.Fatalf("seed credential: %v", err)
	}

	vs, err := ociChartVersions(context.Background(), t.TempDir(), "oci://registry.example.invalid/org", "demo")
	if err != nil {
		t.Fatalf("ociChartVersions: %v", err)
	}
	if strings.Join(vs, ",") != "2.0.0,1.0.0" {
		t.Fatalf("versions = %v", vs)
	}
	if got.user != "alice" || got.secret != "s3cret" {
		t.Fatalf("registry received user=%q secret=%q; the stored credential must reach the wire in the right role", got.user, got.secret)
	}
}

// Credentials a bare `helm registry login` wrote live in the env's registry
// config, not in helmdex's store; listing must honour those too.
func TestOCIChartVersions_UsesRegistryConfigCredential(t *testing.T) {
	_, got := registryRequiringBasicAuth(t, "bob", "hunter2", []string{"3.0.0"})
	t.Setenv("HELMDEX_CREDENTIALS", filepath.Join(t.TempDir(), "absent.yaml"))

	repoRoot := t.TempDir()
	cfgPath := EnvForRepoURL(repoRoot, "oci://registry.example.invalid/org").RegistryConfig
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	auth := base64.StdEncoding.EncodeToString([]byte("bob:hunter2"))
	cfg := `{"auths":{"registry.example.invalid":{"auth":"` + auth + `"}}}`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	vs, err := ociChartVersions(context.Background(), repoRoot, "oci://registry.example.invalid/org", "demo")
	if err != nil {
		t.Fatalf("ociChartVersions: %v", err)
	}
	if strings.Join(vs, ",") != "3.0.0" {
		t.Fatalf("versions = %v", vs)
	}
	if got.user != "bob" || got.secret != "hunter2" {
		t.Fatalf("registry received user=%q secret=%q", got.user, got.secret)
	}
}

// Public charts must keep working with no credential at all.
func TestOCIChartVersions_AnonymousWhenNoCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			http.Error(w, "unexpected credential on an anonymous listing", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"tags": []string{"1.2.3"}})
	}))
	defer srv.Close()
	t.Setenv(ociregistry.FakeRegistryEnv, srv.URL)
	t.Setenv("HELMDEX_CREDENTIALS", filepath.Join(t.TempDir(), "absent.yaml"))

	vs, err := ociChartVersions(context.Background(), t.TempDir(), "oci://registry.example.invalid/org", "demo")
	if err != nil {
		t.Fatalf("ociChartVersions: %v", err)
	}
	if strings.Join(vs, ",") != "1.2.3" {
		t.Fatalf("versions = %v", vs)
	}
}

// The full-ref hint is advice about a path, so it may only ride on an answer
// that says something about the path. Telling a user their URL is wrong when
// the registry is merely down would send them to break a working config.
func TestOCIChartVersions_HintOnlyOnA4xx(t *testing.T) {
	const repo = "oci://registry.example.invalid/demo" // namespace ends with the chart name
	cases := []struct {
		name     string
		status   int
		wantHint bool
	}{
		{name: "path rejected", status: http.StatusForbidden, wantHint: true},
		{name: "path absent", status: http.StatusNotFound, wantHint: true},
		{name: "registry unwell", status: http.StatusServiceUnavailable, wantHint: false},
		{name: "registry erroring", status: http.StatusInternalServerError, wantHint: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "nope", tc.status)
			}))
			defer srv.Close()
			t.Setenv(ociregistry.FakeRegistryEnv, srv.URL)
			t.Setenv("HELMDEX_CREDENTIALS", filepath.Join(t.TempDir(), "absent.yaml"))

			_, err := ociChartVersions(context.Background(), t.TempDir(), repo, "demo")
			if err == nil {
				t.Fatal("want an error")
			}
			hinted := strings.Contains(err.Error(), "already ends with the chart name")
			if hinted != tc.wantHint {
				t.Fatalf("hint present = %v, want %v: %v", hinted, tc.wantHint, err)
			}
		})
	}
}

// An unreachable registry says nothing about the path either.
func TestOCIChartVersions_NoHintWhenRegistryUnreachable(t *testing.T) {
	t.Setenv(ociregistry.FakeRegistryEnv, "http://127.0.0.1:1")
	t.Setenv("HELMDEX_CREDENTIALS", filepath.Join(t.TempDir(), "absent.yaml"))

	_, err := ociChartVersions(context.Background(), t.TempDir(), "oci://registry.example.invalid/demo", "demo")
	if err == nil {
		t.Fatal("want an error")
	}
	if strings.Contains(err.Error(), "already ends with the chart name") {
		t.Fatalf("a connection failure must not be reported as a misconfiguration: %v", err)
	}
}
