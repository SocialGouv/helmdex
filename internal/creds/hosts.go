package creds

import (
	"net/url"
	"strings"
)

// HostOf extracts the lowercase hostname (without port) from a chart or git
// URL. It understands oci://, http(s)://, ssh:// and scp-like git syntax
// (git@host:path). It returns "" for local paths and unparseable input.
func HostOf(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme == "file" {
			return ""
		}
		return strings.ToLower(u.Hostname())
	}
	// scp-like syntax: [user@]host:path — but not a Windows drive (C:\...)
	// and not a plain local path.
	if i := strings.Index(raw, ":"); i > 0 && !strings.Contains(raw[:i], "/") && !strings.Contains(raw[:i], "\\") {
		hostPart := raw[:i]
		if j := strings.Index(hostPart, "@"); j >= 0 {
			hostPart = hostPart[j+1:]
		}
		if strings.Contains(hostPart, ".") || hostPart == "localhost" {
			return strings.ToLower(hostPart)
		}
	}
	return ""
}

// IsSSHURL reports whether a git URL uses SSH transport (ssh:// or scp-like).
func IsSSHURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "ssh://") {
		return true
	}
	if strings.Contains(raw, "://") {
		return false
	}
	// scp-like git@host:path
	i := strings.Index(raw, ":")
	return i > 0 && strings.Contains(raw[:i], "@")
}

// registryStripped classifies a registry-shaped host and returns the host
// that owns the account, following the common GitLab conventions:
//
//	registry.gitlab.example.org -> gitlab.example.org
//	pic-registry.example.org    -> pic.example.org
//
// ok is false when host is not registry-shaped.
func registryStripped(host string) (string, bool) {
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return "", false
	}
	first, rest := labels[0], strings.Join(labels[1:], ".")
	switch {
	case first == "registry":
		return rest, true
	case strings.HasSuffix(first, "-registry"):
		return strings.TrimSuffix(first, "-registry") + "." + rest, true
	}
	return "", false
}

// AccountHostFor maps a registry host to the host that likely owns the
// account (identity when host is not registry-shaped).
func AccountHostFor(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if stripped, ok := registryStripped(host); ok {
		return stripped
	}
	return host
}

// RelatedHosts returns plausible sibling hosts that commonly share the same
// account as host — e.g. a GitLab container registry vs its GitLab instance.
// The result never contains host itself and is best-effort (used only to
// widen credential detection and token-page guessing).
func RelatedHosts(host string) []string {
	host = strings.ToLower(strings.TrimSpace(host))
	if strings.Count(host, ".") < 1 {
		return nil
	}
	if stripped, ok := registryStripped(host); ok && stripped != host {
		return []string{stripped}
	}
	// git host -> registry host
	return []string{"registry." + host}
}

// IsGitHubHost reports whether host is GitHub-operated: github.com and its
// registry ghcr.io, plus GHES/GHEC tenants on *.ghe.com and *.github.com.
// It matches on exact host or dotted suffix — never a bare substring, so
// "notgithub.com" or "mygithub.attacker.example" do not qualify.
func IsGitHubHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	return host == "github.com" ||
		host == "ghcr.io" ||
		strings.HasSuffix(host, ".github.com") ||
		strings.HasSuffix(host, ".ghe.com")
}
