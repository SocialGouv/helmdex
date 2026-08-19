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

// RelatedHosts returns plausible sibling hosts that commonly share the same
// account as host — e.g. a GitLab container registry vs its GitLab instance:
//
//	registry.gitlab.example.org  <-> gitlab.example.org
//	pic-registry.example.org     <-> pic.example.org
//
// The result never contains host itself and is best-effort (used only to
// widen credential detection and token-page guessing).
func RelatedHosts(host string) []string {
	host = strings.ToLower(strings.TrimSpace(host))
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return nil
	}
	first, rest := labels[0], strings.Join(labels[1:], ".")

	var out []string
	add := func(h string) {
		if h == "" || h == host {
			return
		}
		for _, e := range out {
			if e == h {
				return
			}
		}
		out = append(out, h)
	}

	switch {
	case first == "registry":
		// registry.example.org -> example.org
		add(rest)
	case strings.HasSuffix(first, "-registry"):
		// pic-registry.example.org -> pic.example.org
		add(strings.TrimSuffix(first, "-registry") + "." + rest)
	default:
		// example.org -> registry.example.org (git host -> registry host)
		add("registry." + host)
	}
	return out
}
