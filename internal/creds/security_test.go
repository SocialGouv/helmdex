package creds

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// Upsert must reject control characters: a newline in a secret injects extra
// key=value lines into git's credential-helper protocol and breaks the
// docker-auth base64 line format.
func TestUpsertRejectsControlChars(t *testing.T) {
	withStore(t)
	if err := Upsert(Credential{Host: "h.test", Kind: KindGit, Secret: "real\npath=/injected"}); err == nil {
		t.Fatal("newline in secret must be rejected")
	}
	if err := Upsert(Credential{Host: "h.test", Kind: KindGit, Username: "u\x00", Secret: "s"}); err == nil {
		t.Fatal("NUL in username must be rejected")
	}
	if err := Upsert(Credential{Host: "h.test", Kind: KindGit, Secret: "clean"}); err != nil {
		t.Fatalf("clean secret must be accepted: %v", err)
	}
}

// Concurrent Upserts of distinct credentials must not lose updates
// (Load→mutate→Save without a lock silently drops all but one).
func TestUpsertConcurrentNoLostUpdate(t *testing.T) {
	withStore(t)
	const n = 25
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := Upsert(Credential{Host: fmt.Sprintf("h%02d.test", i), Kind: KindOCI, Secret: "s"}); err != nil {
				t.Errorf("upsert %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	list, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != n {
		t.Fatalf("lost updates: %d/%d credentials survived", len(list), n)
	}
}

// A stored OCI credential removed from the store must be pruned from the
// materialized registry config on the next sync — but hosts written by other
// tools (absent from the sidecar) must survive.
func TestSyncRegistryAuthsPrunesStaleButKeepsForeign(t *testing.T) {
	withStore(t)
	cfg := filepath.Join(t.TempDir(), "registry", "config.json")

	if err := Upsert(Credential{Host: "managed.test", Kind: KindOCI, Username: "jo", Secret: "tok"}); err != nil {
		t.Fatal(err)
	}
	if err := SyncRegistryAuths(cfg); err != nil {
		t.Fatal(err)
	}
	// Simulate `helm registry login` writing an entry helmdex never managed.
	injectAuth(t, cfg, "foreign.test", "someone:elsetoken")

	if _, ok := registryAuthsOf(t, cfg)["managed.test"]; !ok {
		t.Fatal("managed host missing after first sync")
	}

	// Remove the managed credential; force the store newer than the config so
	// the mtime guard lets the sync run.
	if _, err := Remove("managed.test", KindOCI); err != nil {
		t.Fatal(err)
	}
	bumpMtimeAfter(t, mustStorePath(t), cfg)
	if err := SyncRegistryAuths(cfg); err != nil {
		t.Fatal(err)
	}

	auths := registryAuthsOf(t, cfg)
	if _, ok := auths["managed.test"]; ok {
		t.Fatal("stale managed host was not pruned after removal")
	}
	if _, ok := auths["foreign.test"]; !ok {
		t.Fatal("foreign (helm registry login) host must not be pruned")
	}
}

// A sync must preserve sibling fields (identitytoken, custom) another tool
// wrote for a host, replacing only the base64 auth blob.
func TestMergeDockerAuthsPreservesSiblingFields(t *testing.T) {
	withStore(t)
	cfg := filepath.Join(t.TempDir(), "config.json")
	// Pre-existing entry with an identity token.
	raw := `{"auths":{"reg.test":{"auth":"b2xkOm9sZA==","identitytoken":"IDTOK","custom":"keep"}}}`
	if err := os.MkdirAll(filepath.Dir(cfg), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Upsert(Credential{Host: "reg.test", Kind: KindOCI, Username: "new", Secret: "new"}); err != nil {
		t.Fatal(err)
	}
	bumpMtimeAfter(t, mustStorePath(t), cfg)
	if err := SyncRegistryAuths(cfg); err != nil {
		t.Fatal(err)
	}

	b, _ := os.ReadFile(cfg)
	var parsed struct {
		Auths map[string]map[string]string `json:"auths"`
	}
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatal(err)
	}
	e := parsed.Auths["reg.test"]
	if e["identitytoken"] != "IDTOK" || e["custom"] != "keep" {
		t.Fatalf("sibling fields dropped: %+v", e)
	}
	if got, _ := base64.StdEncoding.DecodeString(e["auth"]); string(got) != "new:new" {
		t.Fatalf("auth not updated: %q", got)
	}
}

func TestIsGitHubHostNoSubstringMatch(t *testing.T) {
	yes := []string{"github.com", "ghcr.io", "tenant.ghe.com", "acme.github.com"}
	no := []string{"notgithub.com", "mygithub.attacker.example", "foo-github-bar.io", "github.io", "gitlab.com"}
	for _, h := range yes {
		if !IsGitHubHost(h) {
			t.Errorf("IsGitHubHost(%q) = false, want true", h)
		}
	}
	for _, h := range no {
		if IsGitHubHost(h) {
			t.Errorf("IsGitHubHost(%q) = true, want false", h)
		}
	}
}

func TestIsAuthErrorNoNumericFalsePositives(t *testing.T) {
	// Real auth failures still classify.
	for _, s := range []string{
		"response status code 403: denied: requested access to the resource is denied",
		"error: 403 Forbidden fetching index.yaml",
		"401 Unauthorized",
		"remote: HTTP Basic: Access denied.",
	} {
		if !IsAuthError(s) {
			t.Errorf("IsAuthError(%q) = false, want true", s)
		}
	}
	// Non-auth failures must NOT classify.
	for _, s := range []string{
		"chart version v1.403.0 not found",
		"no chart 401-foo found",
		"connection denied: firewall rejected the connection",
		"context deadline exceeded",
	} {
		if IsAuthError(s) {
			t.Errorf("IsAuthError(%q) = true, want false", s)
		}
	}
}

// --- helpers ---

func mustStorePath(t *testing.T) string {
	t.Helper()
	p, err := StorePath()
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func injectAuth(t *testing.T, cfg, host, userPass string) {
	t.Helper()
	b, _ := os.ReadFile(cfg)
	root := map[string]json.RawMessage{}
	_ = json.Unmarshal(b, &root)
	auths := map[string]json.RawMessage{}
	if raw, ok := root["auths"]; ok {
		_ = json.Unmarshal(raw, &auths)
	}
	entry, _ := json.Marshal(map[string]string{"auth": base64.StdEncoding.EncodeToString([]byte(userPass))})
	auths[host] = entry
	root["auths"], _ = json.Marshal(auths)
	out, _ := json.Marshal(root)
	if err := os.WriteFile(cfg, out, 0o600); err != nil {
		t.Fatal(err)
	}
}

func registryAuthsOf(t *testing.T, cfg string) map[string]json.RawMessage {
	t.Helper()
	b, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Auths map[string]json.RawMessage `json:"auths"`
	}
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatal(err)
	}
	return parsed.Auths
}

// bumpMtimeAfter sets newer's mtime to strictly after older's, so the
// store-vs-config freshness guard in SyncRegistryAuths lets the sync run.
func bumpMtimeAfter(t *testing.T, newer, older string) {
	t.Helper()
	oi, err := os.Stat(older)
	if err != nil {
		return // config not created yet: guard proceeds anyway
	}
	ts := oi.ModTime().Add(2 * time.Second)
	if err := os.Chtimes(newer, ts, ts); err != nil {
		t.Fatal(err)
	}
}

// A port identifies a distinct service: two registries can share a hostname.
// A credential stored for one must never be presented to the other.
func TestCredentialsAreKeyedPerEndpoint(t *testing.T) {
	t.Setenv("HELMDEX_CREDENTIALS", filepath.Join(t.TempDir(), "credentials.yaml"))
	if err := Upsert(Credential{
		Host: "reg.example.org", Kind: KindOCI, Username: "alice", Secret: "for-the-default-port",
	}); err != nil {
		t.Fatal(err)
	}

	if _, ok := ForURL("oci://reg.example.org/org/chart", KindOCI); !ok {
		t.Fatal("the credential must still serve its own endpoint")
	}
	if got, ok := ForURL("oci://reg.example.org:5000/org/chart", KindOCI); ok {
		t.Fatalf("credential for reg.example.org leaked to a different service on :5000 (%q)", got.Secret)
	}

	// And a per-endpoint credential is storable and found.
	if err := Upsert(Credential{
		Host: "reg.example.org:5000", Kind: KindOCI, Username: "bob", Secret: "for-5000",
	}); err != nil {
		t.Fatal(err)
	}
	got, ok := ForURL("oci://reg.example.org:5000/org/chart", KindOCI)
	if !ok || got.Secret != "for-5000" {
		t.Fatalf("per-endpoint credential not resolved: %+v ok=%v", got, ok)
	}
}

// Signing in for a ported registry must record that endpoint, so the stored
// credential is the one the registry will be asked for.
func TestCandidateForRepoKeepsThePort(t *testing.T) {
	c, ok := CandidateForRepo("oci://reg.example.org:5000/org/chart")
	if !ok || c.Host != "reg.example.org:5000" {
		t.Fatalf("candidate host = %q", c.Host)
	}
	// Git remotes keep identifying a host, not an endpoint.
	if got := HostKeyFor("ssh://git@git.example.org:2222/o/r.git", KindGit); got != "git.example.org" {
		t.Fatalf("git host key = %q", got)
	}
}

// Token pages and host-family checks are about identity, so a port must not
// reach them.
func TestIdentityHelpersIgnoreThePort(t *testing.T) {
	if got := AccountHostFor("registry.gitlab.example.org:5000"); got != "gitlab.example.org" {
		t.Fatalf("account host = %q", got)
	}
	if !IsGitHubHost("ghcr.io:443") {
		t.Fatal("ghcr.io with an explicit port is still GitHub")
	}
}
