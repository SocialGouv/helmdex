package creds

import (
	"regexp"
	"strings"
)

// authErrRe matches error text produced by helm, ORAS/registry clients and
// git when a remote requires (or refuses) authentication. Patterns are kept
// specific enough to not match local errors like "permission denied" on files.
var authErrRe = regexp.MustCompile(`(?i)` + strings.Join([]string{
	// HTTP status codes, delimited by whitespace/colon so a chart version
	// like "v1.403.0" or "401-foo" does not classify as an auth failure.
	`(?:^|\s)(?:401|403)(?:\s|:|$)`,
	`unauthorized`,
	`unauthenticated`,
	`authentication required`,
	`authentication failed`,
	`authorization failed`,
	`failed to authorize`,
	`could not read username`,
	`could not read password`,
	`terminal prompts disabled`,
	`invalid username or password`,
	`invalid_grant`,
	`http basic: access denied`,
	`access denied`,
	`pull access denied`,
	`denied: requested access`,
	`permission denied \(publickey`,
	`fatal: could not read`,
	`basic credential not found`,
}, "|"))

// IsAuthError reports whether an error message looks like a remote
// authentication/authorization failure.
func IsAuthError(msg string) bool {
	return authErrRe.MatchString(msg)
}

// AuthCandidate identifies a remote a failed operation may need credentials
// for. It is embedded in API error payloads so the UI can offer sign-in.
type AuthCandidate struct {
	Host string `json:"host"`
	Kind Kind   `json:"kind"`
	URL  string `json:"url,omitempty"`
}

// CandidateForRepo builds an AuthCandidate from a chart dependency
// repository URL (oci:// or classic HTTP repo). Returns false for local
// (file://) or unparseable URLs.
func CandidateForRepo(repoURL string) (AuthCandidate, bool) {
	repoURL = strings.TrimSpace(repoURL)
	kind := KindHelmRepo
	if strings.HasPrefix(repoURL, "oci://") {
		kind = KindOCI
	}
	// Keyed by authority: signing in for a registry on a non-default port must
	// store a credential for that endpoint, not for its hostname at large.
	h := HostKeyFor(repoURL, kind)
	if h == "" {
		return AuthCandidate{}, false
	}
	return AuthCandidate{Host: h, Kind: kind, URL: repoURL}, true
}

// CandidateForGit builds an AuthCandidate from a git URL.
func CandidateForGit(gitURL string) (AuthCandidate, bool) {
	h := HostOf(gitURL)
	if h == "" {
		return AuthCandidate{}, false
	}
	return AuthCandidate{Host: h, Kind: KindGit, URL: gitURL}, true
}

// MatchCandidates narrows candidates to those whose host appears in the
// error text; when none match, all candidates are returned (the error text
// often elides the host, e.g. plain "401 Unauthorized").
func MatchCandidates(errText string, cands []AuthCandidate) []AuthCandidate {
	if len(cands) <= 1 {
		return cands
	}
	lower := strings.ToLower(errText)
	var matched []AuthCandidate
	for _, c := range cands {
		if strings.Contains(lower, strings.ToLower(c.Host)) {
			matched = append(matched, c)
		}
	}
	if len(matched) > 0 {
		return matched
	}
	return cands
}
