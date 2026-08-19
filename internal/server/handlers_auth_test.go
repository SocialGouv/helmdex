package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"helmdex/internal/creds"
	"helmdex/internal/testutil"
)

// registryConfigPath is the shared registry config of the test repo (opted-in
// repos keep state under <root>/.helmdex).
func registryConfigPath(ts *testServer) string {
	return filepath.Join(ts.Repo.Root, ".helmdex", "helm", "registry", "config.json")
}

func registryAuths(t *testing.T, path string) map[string]json.RawMessage {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read %s: %v", path, err)
	}
	var cfg struct {
		Auths map[string]json.RawMessage `json:"auths"`
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return cfg.Auths
}

func TestAuth_LoginManualOCIAndList(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})

	// No credentials yet.
	var list []creds.Credential
	ts.get("/api/auth/creds").expect(http.StatusOK).decode(&list)
	if len(list) != 0 {
		t.Fatalf("expected empty credential list, got %+v", list)
	}

	var res struct {
		Host     string `json:"host"`
		Username string `json:"username"`
		Verified bool   `json:"verified"`
	}
	ts.post("/api/auth/login", map[string]any{
		"host": "Reg.Example.Test", "kind": "oci", "method": "manual",
		"username": "jo", "secret": "glpat-tok",
	}).expect(http.StatusOK).decode(&res)
	if res.Host != "reg.example.test" || !res.Verified || res.Username != "jo" {
		t.Fatalf("unexpected login result: %+v", res)
	}

	// helm registry login (fakehelm) persisted the auth in the shared
	// registry config of this repo.
	auths := registryAuths(t, registryConfigPath(ts))
	if _, ok := auths["reg.example.test"]; !ok {
		t.Fatalf("registry config missing entry: %v", auths)
	}

	// The credential is listed, and the secret never leaves the server.
	body := ts.get("/api/auth/creds").expect(http.StatusOK)
	if strings.Contains(body.text(), "glpat-tok") {
		t.Fatal("secret leaked in credential listing")
	}
	list = nil
	body.decode(&list)
	if len(list) != 1 || list[0].Host != "reg.example.test" || list[0].Kind != creds.KindOCI {
		t.Fatalf("unexpected credential list: %+v", list)
	}

	if calls := testutil.FakeHelmCalls(t, ts.HelmLog); len(calls) == 0 || !strings.Contains(strings.Join(calls, "\n"), "registry login reg.example.test") {
		t.Fatalf("expected a registry login helm call, got %v", calls)
	}
}

func TestAuth_LoginRejectedByRegistry(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	t.Setenv("HELMDEX_FAKE_HELM_FAIL", "registry login=401 Unauthorized")

	msg := ts.post("/api/auth/login", map[string]any{
		"host": "reg.example.test", "kind": "oci", "method": "manual",
		"username": "jo", "secret": "bad",
	}).expect(http.StatusBadRequest).errorMessage()
	if !strings.Contains(msg, "401") {
		t.Fatalf("error should carry the registry response: %q", msg)
	}
	// A rejected credential must not be stored.
	if _, ok := creds.ForHost("reg.example.test", creds.KindOCI); ok {
		t.Fatal("rejected credential was stored")
	}
}

func TestAuth_LoginValidation(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	ts.post("/api/auth/login", map[string]any{"host": "", "kind": "oci", "method": "manual", "secret": "x"}).
		expect(http.StatusBadRequest)
	ts.post("/api/auth/login", map[string]any{"host": "h.test", "kind": "nope", "method": "manual", "secret": "x"}).
		expect(http.StatusBadRequest)
	ts.post("/api/auth/login", map[string]any{"host": "h.test", "kind": "git", "method": "manual"}).
		expect(http.StatusBadRequest)
	// SSH key path must exist.
	ts.post("/api/auth/login", map[string]any{
		"host": "h.test", "kind": "git", "method": "manual", "sshKeyPath": "/does/not/exist",
	}).expect(http.StatusBadRequest)
}

func TestAuth_LoginGitSSHKeyStoredWithoutURL(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	key := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(key, []byte("stub"), 0o600); err != nil {
		t.Fatal(err)
	}
	var res struct {
		Verified bool   `json:"verified"`
		Message  string `json:"message"`
	}
	ts.post("/api/auth/login", map[string]any{
		"host": "gitlab.example.test", "kind": "git", "method": "manual", "sshKeyPath": key,
	}).expect(http.StatusOK).decode(&res)
	if res.Verified {
		t.Fatal("ssh key without URL cannot be verified")
	}
	c, ok := creds.ForHost("gitlab.example.test", creds.KindGit)
	if !ok || c.SSHKeyPath != key {
		t.Fatalf("ssh credential not stored: %+v ok=%v", c, ok)
	}
}

func TestAuth_DetectAndDetectedLogin(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	testutil.WriteDockerConfig(t, "reg.example.test", "jo", "docker-secret")

	var det struct {
		Candidates []creds.Candidate `json:"candidates"`
		TokenPage  creds.TokenPage   `json:"tokenPage"`
	}
	ts.post("/api/auth/detect", map[string]any{"host": "reg.example.test", "kind": "oci"}).
		expect(http.StatusOK).decode(&det)
	var found *creds.Candidate
	for i := range det.Candidates {
		if det.Candidates[i].Source == creds.SourceDockerConfig {
			found = &det.Candidates[i]
		}
	}
	if found == nil || found.Username != "jo" {
		t.Fatalf("docker-config candidate not detected: %+v", det.Candidates)
	}
	if det.TokenPage.URL == "" {
		t.Fatal("token page URL missing")
	}

	// Login from the detected source: the secret is resolved server-side.
	var res struct {
		Username string `json:"username"`
		Verified bool   `json:"verified"`
		Source   string `json:"source"`
	}
	ts.post("/api/auth/login", map[string]any{
		"host": "reg.example.test", "kind": "oci", "method": "detected",
		"source": found.Source, "sourceHost": found.Host,
	}).expect(http.StatusOK).decode(&res)
	if !res.Verified || res.Username != "jo" || res.Source != creds.SourceDockerConfig {
		t.Fatalf("unexpected detected login result: %+v", res)
	}
	c, _ := creds.ForHost("reg.example.test", creds.KindOCI)
	if c.Secret != "docker-secret" {
		t.Fatal("detected login did not store the resolved secret")
	}
}

func TestAuth_RemoveCredential(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	ts.post("/api/auth/login", map[string]any{
		"host": "reg.example.test", "kind": "oci", "method": "manual",
		"username": "jo", "secret": "tok",
	}).expect(http.StatusOK)
	if auths := registryAuths(t, registryConfigPath(ts)); len(auths) == 0 {
		t.Fatal("registry config not written by login")
	}

	ts.del("/api/auth/creds/reg.example.test?kind=oci", nil).expect(http.StatusNoContent)
	if _, ok := creds.ForHost("reg.example.test", creds.KindOCI); ok {
		t.Fatal("credential still stored after delete")
	}
	if auths := registryAuths(t, registryConfigPath(ts)); len(auths) != 0 {
		t.Fatalf("registry config still holds an auth entry: %v", auths)
	}
	// Idempotence surfaces as 404, and bad kinds are rejected.
	ts.del("/api/auth/creds/reg.example.test?kind=oci", nil).expect(http.StatusNotFound)
	ts.del("/api/auth/creds/reg.example.test?kind=zzz", nil).expect(http.StatusBadRequest)
}

func TestAuth_HostsListsWorkspaceRemotes(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	ts.createInstance("alpha")
	addDep(ts, "alpha", map[string]any{
		"name": "app", "repository": "oci://reg.example.test/org/app", "version": "1.0.0",
	})
	addDep(ts, "alpha", map[string]any{
		"name": "postgresql", "repository": fixtureRepoURL, "version": "15.5.0",
	})

	var hosts []struct {
		Host          string `json:"host"`
		Kind          string `json:"kind"`
		HasCredential bool   `json:"hasCredential"`
	}
	ts.get("/api/auth/hosts").expect(http.StatusOK).decode(&hosts)
	byKey := map[string]bool{}
	for _, h := range hosts {
		byKey[h.Host+"|"+h.Kind] = h.HasCredential
	}
	if has, ok := byKey["reg.example.test|oci"]; !ok || has {
		t.Fatalf("oci host missing or wrongly credentialed: %v", byKey)
	}
	if _, ok := byKey["example.invalid|helm-repo"]; !ok {
		t.Fatalf("helm-repo host missing: %v", byKey)
	}

	ts.post("/api/auth/login", map[string]any{
		"host": "reg.example.test", "kind": "oci", "method": "manual",
		"username": "jo", "secret": "tok",
	}).expect(http.StatusOK)
	hosts = nil
	ts.get("/api/auth/hosts").expect(http.StatusOK).decode(&hosts)
	for _, h := range hosts {
		if h.Host == "reg.example.test" && !h.HasCredential {
			t.Fatal("hasCredential should be true after login")
		}
	}
}

func TestAuth_TokenPage(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	var page creds.TokenPage
	// open is intentionally false: tests must not launch a browser.
	ts.post("/api/auth/token-page", map[string]any{"host": "pic-registry.example.test", "kind": "oci"}).
		expect(http.StatusOK).decode(&page)
	if page.Provider != "gitlab" || page.Host != "pic.example.test" ||
		!strings.HasPrefix(page.URL, "https://pic.example.test/-/user_settings/personal_access_tokens?") {
		t.Fatalf("unexpected token page: %+v", page)
	}
	ts.post("/api/auth/token-page", map[string]any{"host": ""}).expect(http.StatusBadRequest)
}

func TestAuth_DepVersionsSurfacesAuthRequired(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	ts.createInstance("alpha")
	info := addDep(ts, "alpha", map[string]any{
		"name": "postgresql", "repository": fixtureRepoURL, "version": "15.5.0",
	})
	depID := info.Deps[0].ID

	t.Setenv("HELMDEX_FAKE_HELM_FAIL", "repo add=401 Unauthorized: authentication required")

	var payload struct {
		Error        string `json:"error"`
		AuthRequired *struct {
			Candidates []creds.AuthCandidate `json:"candidates"`
		} `json:"authRequired"`
	}
	ts.get("/api/instances/alpha/deps/" + depID + "/versions").
		expect(http.StatusUnauthorized).decode(&payload)
	if payload.AuthRequired == nil || len(payload.AuthRequired.Candidates) != 1 {
		t.Fatalf("authRequired missing: %+v (%s)", payload, payload.Error)
	}
	c := payload.AuthRequired.Candidates[0]
	if c.Host != "example.invalid" || c.Kind != creds.KindHelmRepo || c.URL != fixtureRepoURL {
		t.Fatalf("unexpected candidate: %+v", c)
	}
}

func TestAuth_RepoAddUsesStoredHelmRepoCredential(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	ts.createInstance("alpha")
	info := addDep(ts, "alpha", map[string]any{
		"name": "postgresql", "repository": fixtureRepoURL, "version": "15.5.0",
	})
	depID := info.Deps[0].ID

	// Store a classic-repo credential (no URL: skips network verification).
	var res struct {
		Verified bool `json:"verified"`
	}
	ts.post("/api/auth/login", map[string]any{
		"host": "example.invalid", "kind": "helm-repo", "method": "manual",
		"username": "jo", "secret": "repo-pass",
	}).expect(http.StatusOK).decode(&res)
	if res.Verified {
		t.Fatal("helm-repo login without URL cannot be verified")
	}

	ts.get("/api/instances/alpha/deps/" + depID + "/versions").expect(http.StatusOK)

	calls := strings.Join(testutil.FakeHelmCalls(t, ts.HelmLog), "\n")
	if !strings.Contains(calls, "--username jo --password-stdin") {
		t.Fatalf("repo add did not pass the stored credential:\n%s", calls)
	}
	if strings.Contains(calls, "repo-pass") {
		t.Fatal("secret leaked into helm argv")
	}
}
