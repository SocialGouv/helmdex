package creds

import (
	"net/url"
	"strings"
)

// TokenPage describes a provider page where the user can create a PAT with
// the right scopes, used for browser-assisted sign-in.
type TokenPage struct {
	// Provider is "github", "gitlab" or "" when unknown.
	Provider string `json:"provider"`
	// URL is the token-creation page, pre-filled with a name and scopes
	// where the provider supports it.
	URL string `json:"url"`
	// Host is the account host the token page belongs to (for a registry
	// host this is usually the related git host).
	Host string `json:"host"`
}

// TokenPageFor guesses the token-creation page for a host. For registry
// hosts it prefers the related git host (a GitLab/GitHub PAT covers both the
// registry and git). Best-effort: unknown self-hosted providers get the
// GitLab-style URL, which covers the common case of a private GitLab.
func TokenPageFor(host string, kind Kind) TokenPage {
	host = strings.ToLower(strings.TrimSpace(host))
	accountHost := host
	if kind == KindOCI {
		// pic-registry.example.org -> pic.example.org, registry.gitlab.com -> gitlab.com
		for _, rel := range RelatedHosts(host) {
			if !strings.HasPrefix(rel, "registry.") && !strings.Contains(strings.Split(rel, ".")[0], "-registry") {
				accountHost = rel
				break
			}
		}
	}

	if accountHost == "github.com" || strings.Contains(accountHost, "github") || host == "ghcr.io" {
		if host == "ghcr.io" {
			accountHost = "github.com"
		}
		q := url.Values{}
		q.Set("description", "helmdex")
		q.Set("scopes", "repo,read:packages")
		return TokenPage{
			Provider: "github",
			Host:     accountHost,
			URL:      "https://" + accountHost + "/settings/tokens/new?" + q.Encode(),
		}
	}

	// GitLab (gitlab.com and self-hosted) supports pre-filling the token
	// name and scopes via query parameters.
	q := url.Values{}
	q.Set("name", "helmdex")
	q.Set("scopes", "read_repository,read_registry,read_api")
	return TokenPage{
		Provider: "gitlab",
		Host:     accountHost,
		URL:      "https://" + accountHost + "/-/user_settings/personal_access_tokens?" + q.Encode(),
	}
}
