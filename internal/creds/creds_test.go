package creds

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func withStore(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "credentials.yaml")
	t.Setenv("HELMDEX_CREDENTIALS", p)
	return p
}

func TestStoreRoundtripAndPerms(t *testing.T) {
	p := withStore(t)

	if err := Upsert(Credential{Host: "Pic-Registry.Example.org", Kind: KindOCI, Username: "jo", Secret: "s3cret", Source: SourceManual}); err != nil {
		t.Fatal(err)
	}
	if err := Upsert(Credential{Host: "pic.example.org", Kind: KindGit, Username: "jo", Secret: "tok"}); err != nil {
		t.Fatal(err)
	}
	// Replace the first one.
	if err := Upsert(Credential{Host: "pic-registry.example.org", Kind: KindOCI, Username: "jo2", Secret: "new"}); err != nil {
		t.Fatal(err)
	}

	c, ok := ForHost("pic-registry.example.org", KindOCI)
	if !ok || c.Username != "jo2" || c.Secret != "new" {
		t.Fatalf("unexpected cred: %+v ok=%v", c, ok)
	}
	if _, ok := ForHost("pic-registry.example.org", KindGit); ok {
		t.Fatal("kind must be part of the key")
	}
	if c, ok := ForURL("oci://pic-registry.example.org/org/chart", KindOCI); !ok || c.Secret != "new" {
		t.Fatalf("ForURL failed: %+v ok=%v", c, ok)
	}

	if runtime.GOOS != "windows" {
		st, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm() != 0o600 {
			t.Fatalf("credentials file mode = %o, want 0600", st.Mode().Perm())
		}
	}

	removed, err := Remove("pic-registry.example.org", KindOCI)
	if err != nil || !removed {
		t.Fatalf("remove: %v removed=%v", err, removed)
	}
	if _, ok := ForHost("pic-registry.example.org", KindOCI); ok {
		t.Fatal("credential still present after remove")
	}
	if _, ok := ForHost("pic.example.org", KindGit); !ok {
		t.Fatal("unrelated credential was removed")
	}
}

func TestUpsertRejectsInvalid(t *testing.T) {
	withStore(t)
	if err := Upsert(Credential{Host: "", Kind: KindOCI}); err == nil {
		t.Fatal("empty host must be rejected")
	}
	if err := Upsert(Credential{Host: "x.example.org", Kind: "nope"}); err == nil {
		t.Fatal("invalid kind must be rejected")
	}
}

func TestHostOf(t *testing.T) {
	cases := map[string]string{
		"oci://pic-registry.sg.social.gouv.fr/org/chart": "pic-registry.sg.social.gouv.fr",
		"https://charts.example.org/stable":              "charts.example.org",
		"https://user:pass@charts.example.org/x":         "charts.example.org",
		"http://localhost:8080/repo":                     "localhost",
		"ssh://git@gitlab.example.org/grp/repo.git":      "gitlab.example.org",
		"git@gitlab.example.org:grp/repo.git":            "gitlab.example.org",
		"gitlab.example.org:grp/repo.git":                "gitlab.example.org",
		"file:///tmp/chart":                              "",
		"/local/path":                                    "",
		"./relative":                                     "",
		"C:\\local\\path":                                "",
		"":                                               "",
	}
	for in, want := range cases {
		if got := HostOf(in); got != want {
			t.Errorf("HostOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsSSHURL(t *testing.T) {
	if !IsSSHURL("git@gitlab.example.org:grp/repo.git") || !IsSSHURL("ssh://git@x.org/r.git") {
		t.Fatal("ssh urls not detected")
	}
	if IsSSHURL("https://gitlab.example.org/grp/repo.git") {
		t.Fatal("https detected as ssh")
	}
}

func TestRelatedHosts(t *testing.T) {
	if got := RelatedHosts("pic-registry.sg.social.gouv.fr"); len(got) != 1 || got[0] != "pic.sg.social.gouv.fr" {
		t.Fatalf("RelatedHosts = %v", got)
	}
	if got := RelatedHosts("registry.gitlab.example.org"); len(got) != 1 || got[0] != "gitlab.example.org" {
		t.Fatalf("RelatedHosts = %v", got)
	}
	if got := RelatedHosts("gitlab.example.org"); len(got) != 1 || got[0] != "registry.gitlab.example.org" {
		t.Fatalf("RelatedHosts = %v", got)
	}
}

func TestIsAuthError(t *testing.T) {
	authErrs := []string{
		`helm pull failed: failed to authorize: failed to fetch oauth token: unexpected status from GET request to https://pic-registry.sg.social.gouv.fr/jwt/auth: 401 Unauthorized`,
		`unauthorized: authentication required`,
		`fatal: could not read Username for 'https://pic.sg.social.gouv.fr': terminal prompts disabled`,
		`remote: HTTP Basic: Access denied.`,
		`Permission denied (publickey).`,
		`denied: requested access to the resource is denied`,
		`error: 403 Forbidden fetching index.yaml`,
	}
	for _, s := range authErrs {
		if !IsAuthError(s) {
			t.Errorf("IsAuthError(%q) = false, want true", s)
		}
	}
	notAuth := []string{
		`no such host: charts.exemple.invalid`,
		`context deadline exceeded`,
		`open /tmp/x: permission denied`,
		`chart "foo" version "1.2.3" not found`,
	}
	for _, s := range notAuth {
		if IsAuthError(s) {
			t.Errorf("IsAuthError(%q) = true, want false", s)
		}
	}
}

func TestCandidatesAndMatch(t *testing.T) {
	oci, ok := CandidateForRepo("oci://reg.example.org/org/chart")
	if !ok || oci.Kind != KindOCI || oci.Host != "reg.example.org" {
		t.Fatalf("oci candidate: %+v ok=%v", oci, ok)
	}
	classic, ok := CandidateForRepo("https://charts.example.org/stable")
	if !ok || classic.Kind != KindHelmRepo {
		t.Fatalf("classic candidate: %+v", classic)
	}
	if _, ok := CandidateForRepo("file://../local"); ok {
		t.Fatal("file:// must not produce a candidate")
	}
	git, ok := CandidateForGit("git@gitlab.example.org:grp/repo.git")
	if !ok || git.Kind != KindGit || git.Host != "gitlab.example.org" {
		t.Fatalf("git candidate: %+v", git)
	}

	cands := []AuthCandidate{oci, classic}
	got := MatchCandidates("401 unauthorized from reg.example.org/v2/", cands)
	if len(got) != 1 || got[0].Host != "reg.example.org" {
		t.Fatalf("MatchCandidates = %v", got)
	}
	got = MatchCandidates("plain 401", cands)
	if len(got) != 2 {
		t.Fatalf("MatchCandidates fallback = %v", got)
	}
}

func TestDetectDockerConfigAndResolve(t *testing.T) {
	withStore(t)
	dir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", dir)
	auth := base64.StdEncoding.EncodeToString([]byte("jo:glpat-abc"))
	cfg := map[string]any{"auths": map[string]any{"pic-registry.example.org": map[string]string{"auth": auth}}}
	b, _ := json.Marshal(cfg)
	if err := os.WriteFile(filepath.Join(dir, "config.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	// Keep the other probes inert.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	t.Setenv("PATH", t.TempDir())

	cands := Detect(context.Background(), "pic-registry.example.org", KindOCI)
	found := false
	for _, c := range cands {
		if c.Source == SourceDockerConfig && c.Host == "pic-registry.example.org" && c.Username == "jo" {
			found = true
		}
	}
	if !found {
		t.Fatalf("docker-config candidate not detected: %+v", cands)
	}

	u, s, err := ResolveCandidate(context.Background(), SourceDockerConfig, "pic-registry.example.org")
	if err != nil || u != "jo" || s != "glpat-abc" {
		t.Fatalf("resolve: %q %v", u, err)
	}
}

func TestDetectGlab(t *testing.T) {
	withStore(t)
	dir := t.TempDir()
	t.Setenv("GLAB_CONFIG_DIR", dir)
	t.Setenv("DOCKER_CONFIG", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	glab := "hosts:\n  pic.example.org:\n    token: glpat-xyz\n    user: jo\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(glab), 0o600); err != nil {
		t.Fatal(err)
	}

	// Registry host: the glab entry for the related git host must surface.
	cands := Detect(context.Background(), "pic-registry.example.org", KindOCI)
	found := false
	for _, c := range cands {
		if c.Source == SourceGlabCLI && c.Host == "pic.example.org" && c.Username == "jo" {
			found = true
		}
	}
	if !found {
		t.Fatalf("glab candidate not detected: %+v", cands)
	}
	u, s, err := ResolveCandidate(context.Background(), SourceGlabCLI, "pic.example.org")
	if err != nil || u != "jo" || s != "glpat-xyz" {
		t.Fatalf("resolve glab: %q %v", u, err)
	}
}

func TestGitCredentialFillStub(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell stub")
	}
	withStore(t)
	bin := t.TempDir()
	stub := `#!/bin/sh
# fake git: only 'credential fill' is supported
if [ "$1" = "credential" ] && [ "$2" = "fill" ]; then
  cat >/dev/null
  echo "protocol=https"
  echo "host=pic.example.org"
  echo "username=jo"
  echo "password=fill-pass"
  exit 0
fi
exit 1
`
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("DOCKER_CONFIG", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	t.Setenv("GLAB_CONFIG_DIR", t.TempDir())

	u, s, err := ResolveCandidate(context.Background(), SourceGitCredential, "pic.example.org")
	if err != nil || u != "jo" || s != "fill-pass" {
		t.Fatalf("git credential fill: %q %q %v", u, s, err)
	}

	cands := Detect(context.Background(), "pic.example.org", KindGit)
	found := false
	for _, c := range cands {
		if c.Source == SourceGitCredential && c.Host == "pic.example.org" {
			found = true
		}
	}
	if !found {
		t.Fatalf("git-credential candidate not detected: %+v", cands)
	}
}

func TestSyncRegistryAuths(t *testing.T) {
	storePath := withStore(t)
	if err := Upsert(Credential{Host: "reg.example.org", Kind: KindOCI, Username: "jo", Secret: "tok"}); err != nil {
		t.Fatal(err)
	}
	// A git credential must not leak into the registry config.
	if err := Upsert(Credential{Host: "git.example.org", Kind: KindGit, Username: "jo", Secret: "x"}); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(t.TempDir(), "registry", "config.json")
	if err := SyncRegistryAuths(cfgPath); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Auths map[string]struct {
			Auth string `json:"auth"`
		} `json:"auths"`
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	e, ok := cfg.Auths["reg.example.org"]
	if !ok {
		t.Fatalf("auths missing entry: %s", b)
	}
	raw, _ := base64.StdEncoding.DecodeString(e.Auth)
	if string(raw) != "jo:tok" {
		t.Fatalf("auth = %q", raw)
	}
	if _, ok := cfg.Auths["git.example.org"]; ok {
		t.Fatal("git credential leaked into registry config")
	}
	if runtime.GOOS != "windows" {
		st, _ := os.Stat(cfgPath)
		if st.Mode().Perm() != 0o600 {
			t.Fatalf("registry config mode = %o, want 0600", st.Mode().Perm())
		}
	}

	// Freshness guard: nothing to do while the config is newer than the store.
	before, _ := os.Stat(cfgPath)
	if err := SyncRegistryAuths(cfgPath); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(cfgPath)
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("sync rewrote an up-to-date config")
	}

	// Removing the credential and the auth entry.
	if _, err := Remove("reg.example.org", KindOCI); err != nil {
		t.Fatal(err)
	}
	if err := RemoveRegistryAuth(cfgPath, "reg.example.org"); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(cfgPath)
	cfg.Auths = nil
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Auths["reg.example.org"]; ok {
		t.Fatal("auth entry still present after removal")
	}
	_ = storePath
}

func TestTokenPageFor(t *testing.T) {
	p := TokenPageFor("pic-registry.sg.social.gouv.fr", KindOCI)
	if p.Provider != "gitlab" || p.Host != "pic.sg.social.gouv.fr" {
		t.Fatalf("gitlab token page: %+v", p)
	}
	if want := "https://pic.sg.social.gouv.fr/-/user_settings/personal_access_tokens?"; len(p.URL) == 0 || p.URL[:len(want)] != want {
		t.Fatalf("gitlab token url: %s", p.URL)
	}
	g := TokenPageFor("ghcr.io", KindOCI)
	if g.Provider != "github" || g.Host != "github.com" {
		t.Fatalf("github token page: %+v", g)
	}
}
