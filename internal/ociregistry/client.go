// Package ociregistry queries the OCI Distribution API (the `/v2/` endpoints)
// of a container registry.
//
// Helm charts stored in a registry expose their versions as tags, and the Helm
// CLI has no command to list them (`helm search` covers only the Artifact Hub
// and classic index.yaml repositories). Listing tags here is what lets helmdex
// offer a version picker for oci:// dependencies.
package ociregistry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	// pageSize is the number of tags requested per page; a registry may
	// return fewer, and some ignore the parameter entirely.
	pageSize = 100
	// maxPages bounds Link-header pagination. Registries do not guarantee any
	// tag ordering, so a truncated list could omit the newest versions — the
	// cap raises an error rather than returning a plausible-looking subset.
	maxPages = 20
	// maxBody caps how much of a response is read, for both parsing and error
	// snippets.
	maxBody = 4 << 20
	// snippetMax caps how much of a failing body is echoed into an error.
	snippetMax = 512
	// totalBudget caps a whole paginated tag walk, independent of the caller's
	// deadline and of the per-request timeout.
	totalBudget = 45 * time.Second
	// maxRedirects restates Go's default hop limit, which a custom
	// CheckRedirect would otherwise disable.
	maxRedirects = 10
)

// FakeRegistryEnv names the variable that redirects every registry host at a
// single base URL, so the hermetic test suites can exercise tag listing
// without network access. Test-only; see internal/testutil.FakeRegistry.
const FakeRegistryEnv = "HELMDEX_FAKE_OCI_REGISTRY"

// Client talks to one registry. A Client caches the bearer token obtained from
// an auth challenge, so paginating a tag list re-authenticates only once.
type Client struct {
	// BaseURL is the registry root, scheme included (e.g. "https://ghcr.io").
	BaseURL    string
	HTTPClient *http.Client
	// Username and Secret authenticate against private registries. Both empty
	// means anonymous access, which is enough for public charts. Callers pass
	// an already-resolved pair — this package holds no credential policy.
	Username string
	Secret   string

	token    string
	useBasic bool
}

// NewClient builds a client for a registry host (e.g. "ghcr.io").
func NewClient(host string) *Client {
	c := &Client{
		BaseURL:    "https://" + NormalizeHost(host),
		HTTPClient: newHTTPClient(),
	}
	if base := strings.TrimSpace(os.Getenv(FakeRegistryEnv)); base != "" {
		c.BaseURL = strings.TrimRight(base, "/")
	}
	return c
}

// NormalizeHost lowercases a registry host and resolves the Docker Hub
// aliases, whose API lives on a different hostname than the reference prefix.
func NormalizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	switch host {
	case "docker.io", "index.docker.io":
		return "registry-1.docker.io"
	}
	return host
}

// Tags lists every tag of repoPath (e.g. "bitnamicharts/nginx"), following the
// registry's Link pagination. Tags are returned in registry order, which is
// unspecified — callers that need an ordering must sort.
func (c *Client) Tags(ctx context.Context, repoPath string) ([]string, error) {
	// Case is preserved: `helm pull` uses the reference verbatim, so
	// normalising here would list one repository and install from another.
	repoPath = strings.Trim(strings.TrimSpace(repoPath), "/")
	if repoPath == "" {
		return nil, fmt.Errorf("repository path is required")
	}

	// Per-request timeouts multiply by maxPages, so a slow registry could hold
	// a caller far longer than any of them expects — the HTTP handler passes a
	// deadline-less request context. Bound the whole walk.
	ctx, cancel := context.WithTimeout(ctx, totalBudget)
	defer cancel()

	next := fmt.Sprintf("%s/v2/%s/tags/list?n=%d", c.BaseURL, repoPath, pageSize)
	var all []string
	for page := 0; next != ""; page++ {
		if page >= maxPages {
			return nil, fmt.Errorf(
				"tag list for %s exceeds %d pages of %d tags; registries do not order tags, so a partial list could hide the newest versions",
				repoPath, maxPages, pageSize)
		}
		current := next
		tags, link, err := c.fetchPage(ctx, current, repoPath)
		if err != nil {
			return nil, err
		}
		all = append(all, tags...)
		next = link
	}
	return all, nil
}

func (c *Client) fetchPage(ctx context.Context, rawURL, repoPath string) (tags []string, next string, err error) {
	resp, err := c.doAuthenticated(ctx, rawURL, repoPath)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close() //nolint:errcheck // read-only response

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, "", fmt.Errorf("read tag list from %s: %w", rawURL, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", statusError("list tags", rawURL, resp.Status, resp.StatusCode, body)
	}

	var payload struct {
		Name string   `json:"name"`
		Tags []string `json:"tags"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, "", fmt.Errorf("parse tag list from %s: %w (body: %s)", rawURL, err, snippet(body))
	}
	return payload.Tags, nextPageURL(rawURL, resp), nil
}

// doAuthenticated issues the request and, when the registry answers with an
// auth challenge, satisfies it and retries once.
func (c *Client) doAuthenticated(ctx context.Context, rawURL, repoPath string) (*http.Response, error) {
	resp, err := c.do(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}

	challenge := resp.Header.Get("WWW-Authenticate")
	drain(resp)
	scheme, params := parseChallenge(challenge)
	switch strings.ToLower(scheme) {
	case "bearer":
		token, err := c.fetchToken(ctx, params, repoPath)
		if err != nil {
			return nil, err
		}
		c.token = token
	case "basic":
		if c.Secret == "" {
			return nil, &StatusError{
				Action: "list tags", Target: rawURL, Status: "401 Unauthorized", Code: http.StatusUnauthorized,
				Body: "registry requires authentication and no credential is available for it",
			}
		}
		c.useBasic = true
	default:
		return nil, &StatusError{
			Action: "list tags", Target: rawURL, Status: "401 Unauthorized", Code: http.StatusUnauthorized,
			Body: fmt.Sprintf("unsupported authentication challenge %q", challenge),
		}
	}
	return c.do(ctx, rawURL)
}

func (c *Client) do(ctx context.Context, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	switch {
	case c.token != "":
		req.Header.Set("Authorization", "Bearer "+c.token)
	case c.useBasic && c.Secret != "":
		req.SetBasicAuth(c.Username, c.Secret)
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", rawURL, err)
	}
	return resp, nil
}

// fetchToken exchanges the challenge for a bearer token at the realm endpoint,
// presenting the stored credential when there is one. Registries grant an
// anonymous token for public repositories, so this also works unauthenticated.
func (c *Client) fetchToken(ctx context.Context, params map[string]string, repoPath string) (string, error) {
	realm := strings.TrimSpace(params["realm"])
	if realm == "" {
		return "", fmt.Errorf("registry authentication challenge for %s has no realm", repoPath)
	}
	u, err := url.Parse(realm)
	if err != nil {
		return "", fmt.Errorf("parse registry token realm %q: %w", realm, err)
	}
	// The realm routinely lives on another host — Docker Hub answers
	// registry-1.docker.io challenges with auth.docker.io, GitLab's registry
	// with gitlab.com — so the host cannot be pinned to the registry's. What is
	// never legitimate is a downgrade: an https registry naming an http realm
	// would put the credential on the wire in the clear for anyone on path.
	if base, berr := url.Parse(c.BaseURL); berr == nil && base.Scheme == "https" && u.Scheme != "https" {
		return "", fmt.Errorf("refusing to authenticate against non-https token realm %q for https registry %s", realm, base.Host)
	}
	q := u.Query()
	if service := strings.TrimSpace(params["service"]); service != "" {
		q.Set("service", service)
	}
	scope := strings.TrimSpace(params["scope"])
	if scope == "" {
		scope = "repository:" + repoPath + ":pull"
	}
	q.Set("scope", scope)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	if c.Secret != "" {
		req.SetBasicAuth(c.Username, c.Secret)
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("GET %s: %w", u.Redacted(), err)
	}
	defer resp.Body.Close() //nolint:errcheck // read-only response

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return "", fmt.Errorf("read registry token from %s: %w", u.Redacted(), err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", statusError("obtain registry token", u.Redacted(), resp.Status, resp.StatusCode, body)
	}

	var tr struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("parse registry token from %s: %w", u.Redacted(), err)
	}
	token := tr.Token
	if token == "" {
		token = tr.AccessToken
	}
	if token == "" {
		return "", fmt.Errorf("registry token endpoint %s returned no token", u.Redacted())
	}
	return token, nil
}

// nextPageURL reads the RFC 5988 Link header the Distribution spec uses for
// tag pagination and returns the next request URL.
//
// Only the query is taken from the header, and it is applied to the URL we
// just requested. The next page is by definition the same tags endpoint with a
// new cursor, so reusing our own URL removes every way a registry could steer
// the request — and with it the bearer token — somewhere else: another host,
// another tenant's path, above a mount prefix, or through `..`.
func nextPageURL(currentURL string, resp *http.Response) string {
	current, err := url.Parse(currentURL)
	if err != nil {
		return ""
	}
	for _, header := range resp.Header.Values("Link") {
		for _, part := range splitTopLevelCommas(header) {
			target, attrs, found := strings.Cut(part, ";")
			if !found || !hasRelNext(attrs) {
				continue
			}
			ref, err := url.Parse(strings.Trim(strings.TrimSpace(target), "<>"))
			if err != nil || ref.RawQuery == "" {
				return ""
			}
			next := *current
			next.RawQuery = ref.RawQuery
			// A cursor that does not advance would spin until the page cap.
			// Compare by value: a registry that reorders query parameters
			// between pages would defeat a byte-exact comparison.
			if next.Query().Encode() == current.Query().Encode() {
				return ""
			}
			return next.String()
		}
	}
	return ""
}

func hasRelNext(attrs string) bool {
	for _, attr := range strings.Split(attrs, ";") {
		if strings.EqualFold(strings.TrimSpace(strings.ReplaceAll(attr, `"`, "")), "rel=next") {
			return true
		}
	}
	return false
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return newHTTPClient()
}

func newHTTPClient() *http.Client {
	return &http.Client{Timeout: 15 * time.Second, CheckRedirect: refusePlaintextRedirect}
}

// refusePlaintextRedirect stops a redirect chain that began on https from
// continuing in the clear. Go forwards the Authorization header across a
// redirect whenever the hostname matches — it compares hosts without the port
// — so a registry could otherwise answer with an https realm, redirect it to
// http on its own host, and collect the credential in plaintext. That defeats
// checking the realm URL's own scheme.
func refusePlaintextRedirect(req *http.Request, via []*http.Request) error {
	// Installing any CheckRedirect replaces Go's built-in 10-hop cap, so it has
	// to be restated here or a redirect loop runs until the timeout.
	if len(via) >= maxRedirects {
		return fmt.Errorf("stopped after %d redirects", maxRedirects)
	}
	if len(via) > 0 && via[0].URL.Scheme == "https" && req.URL.Scheme != "https" {
		return fmt.Errorf("refusing plaintext redirect to %s from an https origin", req.URL.Redacted())
	}
	return nil
}

// parseChallenge splits a WWW-Authenticate value into its scheme and
// key="value" parameters, e.g.
//
//	Bearer realm="https://ghcr.io/token",service="ghcr.io",scope="repository:x:pull"
func parseChallenge(header string) (scheme string, params map[string]string) {
	header = strings.TrimSpace(header)
	params = map[string]string{}
	if header == "" {
		return "", params
	}
	scheme, rest, found := strings.Cut(header, " ")
	if !found {
		return scheme, params
	}
	for _, part := range splitTopLevelCommas(rest) {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.Trim(strings.TrimSpace(value), `"`)
		if key != "" {
			params[key] = value
		}
	}
	return scheme, params
}

// splitTopLevelCommas splits on commas that are not inside a quoted value or a
// <URI> reference. Both header shapes here legitimately contain one: an auth
// scope like "repository:a/b:pull,push", and a Link cursor carrying a comma.
func splitTopLevelCommas(s string) []string {
	var out []string
	var current strings.Builder
	quoted, bracketed := false, false
	for _, r := range s {
		switch {
		case r == '"':
			quoted = !quoted
			current.WriteRune(r)
		case r == '<' && !quoted:
			bracketed = true
			current.WriteRune(r)
		case r == '>' && !quoted:
			bracketed = false
			current.WriteRune(r)
		case r == ',' && !quoted && !bracketed:
			out = append(out, current.String())
			current.Reset()
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		out = append(out, current.String())
	}
	return out
}

// statusError renders a failed registry response. The HTTP status stays in the
// message so creds.IsAuthError can recognise 401/403 and the UI can offer to
// sign in.
// StatusError is a non-200 answer from the registry. It is distinguishable
// from a transport failure so callers can tell "the registry rejected this
// path" from "the registry could not be reached".
type StatusError struct {
	Action string
	Target string
	Status string
	Code   int
	Body   string
}

func (e *StatusError) Error() string {
	if e.Body != "" {
		return fmt.Sprintf("%s: %s returned %s: %s", e.Action, e.Target, e.Status, e.Body)
	}
	return fmt.Sprintf("%s: %s returned %s", e.Action, e.Target, e.Status)
}

func statusError(action, target, status string, code int, body []byte) error {
	return &StatusError{Action: action, Target: target, Status: status, Code: code, Body: snippet(body)}
}

func snippet(body []byte) string {
	s := strings.TrimSpace(string(body))
	if len(s) > snippetMax {
		s = s[:snippetMax] + "…"
	}
	return strings.Join(strings.Fields(s), " ")
}

// drain consumes and closes a response body so the connection can be reused
// for the retry.
func drain(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBody))
	_ = resp.Body.Close()
}
