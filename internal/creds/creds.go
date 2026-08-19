// Package creds manages per-host credentials for private chart sources
// (OCI registries, classic Helm repositories, git remotes).
//
// Credentials are stored user-globally (a host credential is not repo-specific)
// in ~/.config/helmdex/credentials.yaml with 0600 permissions, and are
// materialized into helmdex's isolated Helm environments on demand.
package creds

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Kind classifies what a credential authenticates against.
type Kind string

const (
	// KindOCI is an OCI registry (oci:// chart dependencies).
	KindOCI Kind = "oci"
	// KindGit is a git remote (catalog/preset sources).
	KindGit Kind = "git"
	// KindHelmRepo is a classic HTTP(S) Helm repository.
	KindHelmRepo Kind = "helm-repo"
)

// ValidKind reports whether k is one of the supported kinds.
func ValidKind(k Kind) bool {
	switch k {
	case KindOCI, KindGit, KindHelmRepo:
		return true
	}
	return false
}

// Credential is one stored credential for a host.
type Credential struct {
	Host     string `yaml:"host" json:"host"`
	Kind     Kind   `yaml:"kind" json:"kind"`
	Username string `yaml:"username,omitempty" json:"username,omitempty"`
	// Secret is a password, PAT or identity token. Never returned over the
	// HTTP API and never logged.
	Secret string `yaml:"secret,omitempty" json:"-"`
	// SSHKeyPath points at a private key used for git-over-SSH. Only the
	// path is stored; the key content is never read by helmdex.
	SSHKeyPath string `yaml:"sshKeyPath,omitempty" json:"sshKeyPath,omitempty"`
	// Source records where the credential came from (manual, docker-config,
	// gh-cli, ...) for display purposes.
	Source string `yaml:"source,omitempty" json:"source,omitempty"`
}

// Store is the on-disk credential collection.
type Store struct {
	APIVersion  string       `yaml:"apiVersion"`
	Kind        string       `yaml:"kind"`
	Credentials []Credential `yaml:"credentials"`
}

const (
	storeAPIVersion = "helmdex.io/v1alpha1"
	storeKind       = "HelmdexCredentials"
)

// StorePath returns the credentials file location.
// Overridable with HELMDEX_CREDENTIALS (used by tests).
func StorePath() (string, error) {
	if v := strings.TrimSpace(os.Getenv("HELMDEX_CREDENTIALS")); v != "" {
		return v, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(base, "helmdex", "credentials.yaml"), nil
}

// Load reads the credential store. A missing file yields an empty store.
func Load() (Store, error) {
	path, err := StorePath()
	if err != nil {
		return Store{}, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Store{APIVersion: storeAPIVersion, Kind: storeKind}, nil
		}
		return Store{}, err
	}
	var st Store
	if err := yaml.Unmarshal(b, &st); err != nil {
		return Store{}, fmt.Errorf("parse credentials %s: %w", path, err)
	}
	return st, nil
}

// Save writes the credential store with 0600 permissions.
func Save(st Store) error {
	path, err := StorePath()
	if err != nil {
		return err
	}
	st.APIVersion = storeAPIVersion
	st.Kind = storeKind
	sort.SliceStable(st.Credentials, func(i, j int) bool {
		if st.Credentials[i].Host != st.Credentials[j].Host {
			return st.Credentials[i].Host < st.Credentials[j].Host
		}
		return st.Credentials[i].Kind < st.Credentials[j].Kind
	})
	out, err := yaml.Marshal(st)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".credentials-*.yaml")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := tmp.Write(out); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// Upsert adds or replaces the credential for (host, kind).
func Upsert(c Credential) error {
	c.Host = strings.ToLower(strings.TrimSpace(c.Host))
	if c.Host == "" {
		return fmt.Errorf("credential host is required")
	}
	if !ValidKind(c.Kind) {
		return fmt.Errorf("invalid credential kind %q", c.Kind)
	}
	st, err := Load()
	if err != nil {
		return err
	}
	replaced := false
	for i := range st.Credentials {
		if st.Credentials[i].Host == c.Host && st.Credentials[i].Kind == c.Kind {
			st.Credentials[i] = c
			replaced = true
			break
		}
	}
	if !replaced {
		st.Credentials = append(st.Credentials, c)
	}
	return Save(st)
}

// Remove deletes the credential for (host, kind). With an empty kind, all
// kinds for the host are removed. It reports whether anything was removed.
func Remove(host string, kind Kind) (bool, error) {
	host = strings.ToLower(strings.TrimSpace(host))
	st, err := Load()
	if err != nil {
		return false, err
	}
	kept := st.Credentials[:0]
	removed := false
	for _, c := range st.Credentials {
		if c.Host == host && (kind == "" || c.Kind == kind) {
			removed = true
			continue
		}
		kept = append(kept, c)
	}
	if !removed {
		return false, nil
	}
	st.Credentials = kept
	return true, Save(st)
}

// ForHost returns the stored credential for (host, kind).
func ForHost(host string, kind Kind) (Credential, bool) {
	host = strings.ToLower(strings.TrimSpace(host))
	st, err := Load()
	if err != nil {
		return Credential{}, false
	}
	for _, c := range st.Credentials {
		if c.Host == host && c.Kind == kind {
			return c, true
		}
	}
	return Credential{}, false
}

// ForURL returns the stored credential for the host of url, for the given kind.
func ForURL(rawURL string, kind Kind) (Credential, bool) {
	h := HostOf(rawURL)
	if h == "" {
		return Credential{}, false
	}
	return ForHost(h, kind)
}

// List returns all stored credentials.
func List() ([]Credential, error) {
	st, err := Load()
	if err != nil {
		return nil, err
	}
	return st.Credentials, nil
}
