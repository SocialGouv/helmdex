package helmutil

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
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

// The doubled-path observation may only ride on an answer that concerns the
// resource itself. Telling a rate-limited user to edit their repository would
// send them to break a working configuration.
func TestOCIChartVersions_HintOnlyWhenTheStatusConcernsTheResource(t *testing.T) {
	const repo = "oci://registry.example.invalid/demo" // namespace ends with the chart name
	cases := []struct {
		name     string
		status   int
		wantHint bool
	}{
		{name: "resource forbidden", status: http.StatusForbidden, wantHint: true},
		{name: "resource absent", status: http.StatusNotFound, wantHint: true},
		{name: "credentials refused", status: http.StatusUnauthorized, wantHint: true},
		{name: "rate limited", status: http.StatusTooManyRequests, wantHint: false},
		{name: "blocked for legal reasons", status: http.StatusUnavailableForLegalReasons, wantHint: false},
		{name: "request timed out", status: http.StatusRequestTimeout, wantHint: false},
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
			hinted := strings.Contains(err.Error(), "Helm appends the chart name")
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
	if strings.Contains(err.Error(), "Helm appends the chart name") {
		t.Fatalf("a connection failure must not be reported as a misconfiguration: %v", err)
	}
}

// A registry that challenges Basic with no credential stored produces the
// client's own refusal rather than a relayed status. That is exactly the shape
// the doubled-path case takes on such a registry, so the observation must
// still reach the user.
func TestOCIChartVersions_HintOnABasicChallengeWithoutCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Basic realm="registry"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()
	t.Setenv(ociregistry.FakeRegistryEnv, srv.URL)
	t.Setenv("HELMDEX_CREDENTIALS", filepath.Join(t.TempDir(), "absent.yaml"))

	_, err := ociChartVersions(context.Background(), t.TempDir(), "oci://registry.example.invalid/demo", "demo")
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "Helm appends the chart name") {
		t.Fatalf("the doubled-path observation must survive a Basic challenge: %v", err)
	}
}

// A registry cannot distinguish "absent" from "forbidden", so the observation
// must read as a possibility rather than an instruction — a namespace really
// may end with the chart name.
func TestOCIFullRefHint_ReadsAsAPossibilityNotAnInstruction(t *testing.T) {
	hint := OCIFullRefHint("oci://reg.test/nginx", "nginx")
	for _, want := range []string{"asked the registry for", "nginx/nginx", "if the chart is not published"} {
		if !strings.Contains(hint, want) {
			t.Fatalf("hint must state what was requested and stay conditional, missing %q: %s", want, hint)
		}
	}
}

// A proxy demanding credentials is not the registry answering about the path.
// creds.IsAuthError matches it — rightly, for offering a sign-in — so the
// resource check must be narrower, or the explanation claims the registry was
// asked for a path the request never reached.
func TestWithOCIRefHint_IgnoresFailuresBeforeTheRegistry(t *testing.T) {
	const repo = "oci://reg.test/demo"
	cases := map[string]struct {
		err      error
		wantHint bool
	}{
		"proxy demanded credentials": {
			err:      errors.New("helm pull failed: proxyconnect tcp: proxy returned status 407 Proxy Authentication Required"),
			wantHint: false,
		},
		"registry refused the path": {
			err:      errors.New("helm pull failed: 403 denied: requested access to the resource is denied"),
			wantHint: true,
		},
		"registry has no such repository": {
			err:      errors.New("helm pull failed: 404 name unknown"),
			wantHint: true,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := WithOCIRefHint(tc.err, repo, "demo")
			hinted := strings.Contains(got.Error(), "Helm appends the chart name")
			if hinted != tc.wantHint {
				t.Fatalf("hint present = %v, want %v: %v", hinted, tc.wantHint, got)
			}
		})
	}
}

// Wrapping twice would repeat the sentence verbatim.
func TestWithOCIRefHint_IsIdempotent(t *testing.T) {
	base := errors.New("list tags: 403 denied: requested access to the resource is denied")
	once := WithOCIRefHint(base, "oci://reg.test/demo", "demo")
	twice := WithOCIRefHint(once, "oci://reg.test/demo", "demo")
	if strings.Count(twice.Error(), "Helm appends the chart name") != 1 {
		t.Fatalf("explanation repeated: %v", twice)
	}
}

// Helm reports a registry 404 with no status code at all, so the text shapes it
// actually emits have to be matched explicitly — otherwise the doubled-path
// explanation vanishes on exactly the registries that answer 404.
func TestWithOCIRefHint_RecognisesTheShapesHelmEmits(t *testing.T) {
	const repo = "oci://reg.test/demo"
	rejected := map[string]string{
		// Captured verbatim from helm v4.1.1 against a 404 registry.
		"oras fetch reference":          `helm pull failed: Error: failed to perform "FetchReference" on source: 127.0.0.1:8791/org/demo/demo:1.0.0: not found`,
		"distribution name unknown":     "helm pull failed: NAME_UNKNOWN: repository name not known to registry",
		"distribution manifest unknown": "helm pull failed: MANIFEST_UNKNOWN: manifest unknown",
		"denied":                        "helm pull failed: 403 denied: requested access to the resource is denied",
	}
	for name, text := range rejected {
		t.Run(name, func(t *testing.T) {
			got := WithOCIRefHint(errors.New(text), repo, "demo")
			if !strings.Contains(got.Error(), "Helm appends the chart name") {
				t.Fatalf("no explanation attached to a registry rejection: %v", got)
			}
		})
	}

	// A bare "not found" from somewhere else must not be treated as one.
	unrelated := errors.New("helm pull failed: chart values file not found in archive")
	if got := WithOCIRefHint(unrelated, repo, "demo"); strings.Contains(got.Error(), "Helm appends the chart name") {
		t.Fatalf("explanation attached to an unrelated failure: %v", got)
	}
}
