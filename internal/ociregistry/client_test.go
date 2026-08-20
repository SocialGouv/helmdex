package ociregistry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"helmdex/internal/creds"
)

// tagsHandler serves a tag list, optionally paginated, at /v2/<repo>/tags/list.
func tagsHandler(t *testing.T, pages [][]string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		page := 0
		if last := r.URL.Query().Get("last"); last != "" {
			for i, p := range pages {
				if len(p) > 0 && p[len(p)-1] == last {
					page = i + 1
				}
			}
		}
		if page >= len(pages) {
			http.Error(w, "page out of range", http.StatusNotFound)
			return
		}
		if page+1 < len(pages) {
			next := pages[page][len(pages[page])-1]
			w.Header().Set("Link", fmt.Sprintf(`<%s?last=%s&n=%d>; rel="next"`, r.URL.Path, next, pageSize))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"name": "org/chart", "tags": pages[page]})
	}
}

func TestTags_Anonymous(t *testing.T) {
	srv := httptest.NewServer(tagsHandler(t, [][]string{{"1.0.0", "1.1.0"}}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTPClient: srv.Client()}
	tags, err := c.Tags(context.Background(), "org/chart")
	if err != nil {
		t.Fatalf("Tags: %v", err)
	}
	if strings.Join(tags, ",") != "1.0.0,1.1.0" {
		t.Fatalf("tags = %v", tags)
	}
}

func TestTags_BearerChallenge(t *testing.T) {
	var gotScope, gotService, gotAuthz, gotTokenAuthz string
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		gotScope = r.URL.Query().Get("scope")
		gotService = r.URL.Query().Get("service")
		gotTokenAuthz = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "tok-123"})
	})
	mux.HandleFunc("/v2/org/chart/tags/list", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok-123" {
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(
				`Bearer realm="%s/token",service="registry.test",scope="repository:org/chart:pull"`, srv.URL))
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		gotAuthz = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{"tags": []string{"2.0.0"}})
	})

	c := &Client{BaseURL: srv.URL, HTTPClient: srv.Client(), Username: "alice", Secret: "s3cret"}
	tags, err := c.Tags(context.Background(), "org/chart")
	if err != nil {
		t.Fatalf("Tags: %v", err)
	}
	if len(tags) != 1 || tags[0] != "2.0.0" {
		t.Fatalf("tags = %v", tags)
	}
	if gotScope != "repository:org/chart:pull" {
		t.Fatalf("scope = %q", gotScope)
	}
	if gotService != "registry.test" {
		t.Fatalf("service = %q", gotService)
	}
	if gotAuthz != "Bearer tok-123" {
		t.Fatalf("registry authorization = %q", gotAuthz)
	}
	// The stored credential must be presented to the token endpoint, otherwise
	// private repositories would only ever get an anonymous token.
	if !strings.HasPrefix(gotTokenAuthz, "Basic ") {
		t.Fatalf("token endpoint authorization = %q, want basic auth", gotTokenAuthz)
	}
}

// A registry may answer the token request with `access_token` instead of
// `token`; both are in use.
func TestTags_BearerAccessTokenField(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "tok-alt"})
	})
	mux.HandleFunc("/v2/org/chart/tags/list", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok-alt" {
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token"`, srv.URL))
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"tags": []string{"3.0.0"}})
	})

	c := &Client{BaseURL: srv.URL, HTTPClient: srv.Client()}
	tags, err := c.Tags(context.Background(), "org/chart")
	if err != nil {
		t.Fatalf("Tags: %v", err)
	}
	if len(tags) != 1 || tags[0] != "3.0.0" {
		t.Fatalf("tags = %v", tags)
	}
}

func TestTags_BasicChallenge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="registry"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if user != "alice" || pass != "s3cret" {
			http.Error(w, "bad credential", http.StatusForbidden)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"tags": []string{"1.2.3"}})
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTPClient: srv.Client(), Username: "alice", Secret: "s3cret"}
	tags, err := c.Tags(context.Background(), "org/chart")
	if err != nil {
		t.Fatalf("Tags: %v", err)
	}
	if len(tags) != 1 || tags[0] != "1.2.3" {
		t.Fatalf("tags = %v", tags)
	}
}

// Without a credential a Basic challenge cannot be satisfied; the error must
// name the cause and be recognisable as an auth failure so the UI offers
// sign-in rather than reporting a generic gateway error.
func TestTags_BasicChallengeWithoutCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Basic realm="registry"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTPClient: srv.Client()}
	_, err := c.Tags(context.Background(), "org/chart")
	if err == nil {
		t.Fatal("want an error when no credential can satisfy the challenge")
	}
	if !creds.IsAuthError(err.Error()) {
		t.Fatalf("error must classify as an auth failure: %v", err)
	}
}

func TestTags_ForbiddenSurfacesStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"errors":[{"code":"DENIED","message":"denied: requested access to the resource is denied"}]}`, http.StatusForbidden)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTPClient: srv.Client()}
	_, err := c.Tags(context.Background(), "org/chart")
	if err == nil {
		t.Fatal("want an error on 403")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Fatalf("error must carry the HTTP status: %v", err)
	}
	if !creds.IsAuthError(err.Error()) {
		t.Fatalf("error must classify as an auth failure: %v", err)
	}
}

func TestTags_Pagination(t *testing.T) {
	srv := httptest.NewServer(tagsHandler(t, [][]string{
		{"1.0.0", "1.1.0"},
		{"1.2.0", "1.3.0"},
		{"2.0.0"},
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTPClient: srv.Client()}
	tags, err := c.Tags(context.Background(), "org/chart")
	if err != nil {
		t.Fatalf("Tags: %v", err)
	}
	if got := strings.Join(tags, ","); got != "1.0.0,1.1.0,1.2.0,1.3.0,2.0.0" {
		t.Fatalf("tags = %q", got)
	}
}

func TestTags_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>not json</html>"))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTPClient: srv.Client()}
	_, err := c.Tags(context.Background(), "org/chart")
	if err == nil {
		t.Fatal("want an error on a non-JSON body")
	}
	if !strings.Contains(err.Error(), "parse tag list") {
		t.Fatalf("error = %v", err)
	}
}

func TestTags_EmptyRepoPath(t *testing.T) {
	c := &Client{BaseURL: "https://registry.test"}
	if _, err := c.Tags(context.Background(), "  "); err == nil {
		t.Fatal("want an error for an empty repository path")
	}
}

func TestParseChallenge(t *testing.T) {
	scheme, params := parseChallenge(`Bearer realm="https://ghcr.io/token",service="ghcr.io",scope="repository:org/chart:pull,push"`)
	if scheme != "Bearer" {
		t.Fatalf("scheme = %q", scheme)
	}
	if params["realm"] != "https://ghcr.io/token" {
		t.Fatalf("realm = %q", params["realm"])
	}
	if params["service"] != "ghcr.io" {
		t.Fatalf("service = %q", params["service"])
	}
	// The comma inside the quoted scope must not split the parameter.
	if params["scope"] != "repository:org/chart:pull,push" {
		t.Fatalf("scope = %q", params["scope"])
	}
}

func TestNormalizeHost(t *testing.T) {
	for in, want := range map[string]string{
		"GHCR.io":                     "ghcr.io",
		"docker.io":                   "registry-1.docker.io",
		"index.docker.io":             "registry-1.docker.io",
		"registry.gitlab.example.org": "registry.gitlab.example.org",
	} {
		if got := NormalizeHost(in); got != want {
			t.Fatalf("NormalizeHost(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNewClient_FakeRegistryOverride(t *testing.T) {
	t.Setenv(FakeRegistryEnv, "http://127.0.0.1:9999/")
	c := NewClient("ghcr.io")
	if c.BaseURL != "http://127.0.0.1:9999" {
		t.Fatalf("BaseURL = %q", c.BaseURL)
	}
}

func TestNewClient_DefaultsToHTTPS(t *testing.T) {
	t.Setenv(FakeRegistryEnv, "")
	if got := NewClient("ghcr.io").BaseURL; got != "https://ghcr.io" {
		t.Fatalf("BaseURL = %q", got)
	}
}

// A registry names its own token realm, and that realm legitimately lives on
// another host (Docker Hub answers registry-1.docker.io challenges with
// auth.docker.io). A scheme downgrade is never legitimate though: it would put
// the credential on the wire in the clear.
func TestTags_RefusesNonHTTPSTokenRealm(t *testing.T) {
	var leaked string
	mux := http.NewServeMux()
	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "stolen"})
	}))
	defer attacker.Close()
	srv := httptest.NewServer(mux)
	defer srv.Close()
	mux.HandleFunc("/v2/org/chart/tags/list", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token"`, attacker.URL))
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})

	// BaseURL is https to model production; the challenge names an http realm.
	c := &Client{BaseURL: "https://registry.test", HTTPClient: srv.Client(), Username: "alice", Secret: "s3cret"}
	c.BaseURL = strings.Replace(srv.URL, "http://", "https://", 1)
	// Point the request at the real (http) test server while BaseURL claims https.
	_, err := c.fetchToken(context.Background(), map[string]string{"realm": attacker.URL + "/token"}, "org/chart")
	if err == nil {
		t.Fatal("want a refusal when an https registry names an http token realm")
	}
	if leaked != "" {
		t.Fatalf("credential sent to a plaintext realm: %q", leaked)
	}
	if !strings.Contains(err.Error(), "non-https token realm") {
		t.Fatalf("error = %v", err)
	}
}

// A cross-host realm must still work: that is how Docker Hub and GitLab behave.
func TestTags_AllowsCrossHostHTTPSRealm(t *testing.T) {
	if _, err := url.Parse("https://auth.docker.io/token"); err != nil {
		t.Fatal(err)
	}
	c := &Client{BaseURL: "https://registry-1.docker.io"}
	// Only the scheme check should reject; a cross-host https realm must pass it.
	_, err := c.fetchToken(context.Background(), map[string]string{"realm": "https://auth.example.invalid/token"}, "org/chart")
	if err != nil && strings.Contains(err.Error(), "non-https token realm") {
		t.Fatalf("cross-host https realm must be allowed, got %v", err)
	}
}

// The paginated request carries our bearer token, so a registry naming a
// foreign host in its Link header must not be able to redirect it there.
func TestTags_LinkHeaderCannotRetargetAnotherHost(t *testing.T) {
	var leaked string
	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{"tags": []string{"9.9.9"}})
	}))
	defer attacker.Close()

	page := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.Header().Set("WWW-Authenticate", `Basic realm="registry"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if page == 0 {
			page++
			w.Header().Set("Link", fmt.Sprintf(`<%s/v2/org/chart/tags/list?last=1.0.0>; rel="next"`, attacker.URL))
			_ = json.NewEncoder(w).Encode(map[string]any{"tags": []string{"1.0.0"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"tags": []string{"2.0.0"}})
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTPClient: srv.Client(), Username: "alice", Secret: "s3cret"}
	tags, err := c.Tags(context.Background(), "org/chart")
	if err != nil {
		t.Fatalf("Tags: %v", err)
	}
	if leaked != "" {
		t.Fatalf("credential sent to the host named by the Link header: %q", leaked)
	}
	// Pagination still followed, but against our own registry.
	if strings.Join(tags, ",") != "1.0.0,2.0.0" {
		t.Fatalf("tags = %v", tags)
	}
}

// One slow registry must not be able to hold a caller for maxPages x the
// per-request timeout; the walk carries its own budget.
func TestTags_TotalBudgetIsBounded(t *testing.T) {
	if totalBudget > 60*time.Second {
		t.Fatalf("totalBudget = %v: too generous to protect a deadline-less caller", totalBudget)
	}
}

// helm pull uses the chart reference verbatim, so the tag listing must query
// the same repository path — normalising here would list one repository and
// install from another.
func TestTags_PreservesRepoPathCase(t *testing.T) {
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{"tags": []string{"1.0.0"}})
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTPClient: srv.Client()}
	if _, err := c.Tags(context.Background(), "OrgName/MyChart"); err != nil {
		t.Fatalf("Tags: %v", err)
	}
	if seen != "/v2/OrgName/MyChart/tags/list" {
		t.Fatalf("registry saw %q, want the path helm pull would use", seen)
	}
}

// Checking the realm URL's own scheme is not enough: Go forwards the
// Authorization header across a same-host redirect, so a registry could answer
// with an https realm that redirects to http and collect the credential in the
// clear. The redirect itself must be refused.
func TestTags_RefusesPlaintextRedirectFromHTTPSOrigin(t *testing.T) {
	var leaked string
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked = r.Header.Get("Authorization")
	}))
	defer plain.Close()

	secure := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL+"/v2/org/chart/tags/list", http.StatusFound)
	}))
	defer secure.Close()

	client := secure.Client()
	client.CheckRedirect = refusePlaintextRedirect
	c := &Client{BaseURL: secure.URL, HTTPClient: client, Username: "alice", Secret: "s3cret"}
	c.useBasic = true

	if _, err := c.Tags(context.Background(), "org/chart"); err == nil {
		t.Fatal("want a refusal when an https origin redirects to plaintext")
	}
	if leaked != "" {
		t.Fatalf("credential forwarded to a plaintext hop: %q", leaked)
	}
}

// The policy has to be wired into the client production actually uses.
func TestNewClient_RefusesPlaintextRedirects(t *testing.T) {
	t.Setenv(FakeRegistryEnv, "")
	if NewClient("ghcr.io").HTTPClient.CheckRedirect == nil {
		t.Fatal("NewClient must install a redirect policy")
	}
	req, _ := http.NewRequest(http.MethodGet, "http://ghcr.io/token", nil)
	via, _ := http.NewRequest(http.MethodGet, "https://ghcr.io/v2/x/tags/list", nil)
	if err := refusePlaintextRedirect(req, []*http.Request{via}); err == nil {
		t.Fatal("an https origin redirecting to http must be refused")
	}
	// An https-to-https redirect stays legitimate.
	req2, _ := http.NewRequest(http.MethodGet, "https://auth.docker.io/token", nil)
	if err := refusePlaintextRedirect(req2, []*http.Request{via}); err != nil {
		t.Fatalf("https redirect must be allowed: %v", err)
	}
}

// A Link header with no path is not a next page; following it would re-request
// the registry root until the page cap trips.
func TestTags_EmptyLinkPathStopsPagination(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Link", `<>; rel="next"`)
		_ = json.NewEncoder(w).Encode(map[string]any{"tags": []string{"1.0.0"}})
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTPClient: srv.Client()}
	tags, err := c.Tags(context.Background(), "org/chart")
	if err != nil {
		t.Fatalf("Tags: %v", err)
	}
	if hits != 1 {
		t.Fatalf("walked %d pages for an empty Link target", hits)
	}
	if strings.Join(tags, ",") != "1.0.0" {
		t.Fatalf("tags = %v", tags)
	}
}

// Installing a CheckRedirect replaces Go's built-in hop limit, so the limit
// has to be restated or a redirect loop runs until the timeout.
func TestRefusePlaintextRedirect_CapsHops(t *testing.T) {
	origin, _ := http.NewRequest(http.MethodGet, "https://ghcr.io/v2/x/tags/list", nil)
	next, _ := http.NewRequest(http.MethodGet, "https://ghcr.io/hop", nil)

	via := make([]*http.Request, maxRedirects)
	for i := range via {
		via[i] = origin
	}
	if err := refusePlaintextRedirect(next, via); err == nil {
		t.Fatalf("a chain of %d hops must be stopped", maxRedirects)
	}
	if err := refusePlaintextRedirect(next, via[:maxRedirects-1]); err != nil {
		t.Fatalf("a chain below the cap must be allowed: %v", err)
	}
}

// The next page is the same endpoint with a new cursor, so only the query is
// taken from the Link header. That keeps a registry mounted under a path prefix
// working whether or not its Link repeats the prefix, and leaves no way to
// steer the request — and the bearer token with it — off the endpoint.
func TestNextPageURL_TakesOnlyTheCursor(t *testing.T) {
	const current = "https://reg.example.test/prefix/v2/foo/tags/list?n=100"
	cases := map[string]string{
		"link repeats the mount prefix":  `</prefix/v2/foo/tags/list?n=100&last=v1>; rel="next"`,
		"link omits the mount prefix":    `</v2/foo/tags/list?n=100&last=v1>; rel="next"`,
		"link is relative":               `<v2/foo/tags/list?n=100&last=v1>; rel="next"`,
		"link names another host":        `<https://evil.test/v2/foo/tags/list?n=100&last=v1>; rel="next"`,
		"link escapes to another tenant": `</tenant-b/v2/secret/tags/list?n=100&last=v1>; rel="next"`,
		"link traverses upwards":         `<../../../other/tags/list?n=100&last=v1>; rel="next"`,
	}
	want := "https://reg.example.test/prefix/v2/foo/tags/list?n=100&last=v1"
	for name, link := range cases {
		t.Run(name, func(t *testing.T) {
			resp := &http.Response{Header: http.Header{}}
			resp.Header.Set("Link", link)
			if got := nextPageURL(current, resp); got != want {
				t.Fatalf("nextPageURL = %q, want %q", got, want)
			}
		})
	}
}

// A cursor that does not advance would spin until the page cap.
func TestNextPageURL_StopsOnANonAdvancingCursor(t *testing.T) {
	const current = "https://reg.example.test/v2/foo/tags/list?n=100"
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("Link", `</v2/foo/tags/list?n=100>; rel="next"`)
	if got := nextPageURL(current, resp); got != "" {
		t.Fatalf("nextPageURL = %q, want pagination to stop", got)
	}
}

// A cursor may carry a comma. Splitting the header on every comma would bisect
// the URI reference and silently end pagination one page early.
func TestNextPageURL_CommaInsideTheCursor(t *testing.T) {
	const current = "https://reg.test/v2/foo/tags/list?n=100"
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("Link", `</v2/foo/tags/list?n=100&last=v1,build.1>; rel="next"`)

	// The cursor is passed through verbatim; re-encoding could corrupt it.
	want := "https://reg.test/v2/foo/tags/list?n=100&last=v1,build.1"
	if got := nextPageURL(current, resp); got != want {
		t.Fatalf("nextPageURL = %q, want %q", got, want)
	}
}

// A registry that reorders query parameters between pages must not defeat the
// non-advancing-cursor guard and burn the whole page budget.
func TestNextPageURL_StopsOnAReorderedButIdenticalCursor(t *testing.T) {
	const current = "https://reg.test/v2/foo/tags/list?n=100&last=v1"
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("Link", `</v2/foo/tags/list?last=v1&n=100>; rel="next"`)

	if got := nextPageURL(current, resp); got != "" {
		t.Fatalf("nextPageURL = %q, want pagination to stop", got)
	}
}

// The auth-scope splitter keeps its own comma case working.
func TestSplitTopLevelCommas_KeepsQuotedAndBracketedCommas(t *testing.T) {
	got := splitTopLevelCommas(`realm="https://a/token",scope="repository:a/b:pull,push"`)
	if len(got) != 2 {
		t.Fatalf("split into %d parts: %q", len(got), got)
	}
	if !strings.Contains(got[1], "pull,push") {
		t.Fatalf("quoted comma was split: %q", got)
	}
}
