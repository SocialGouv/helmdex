package creds

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
)

// SyncRegistryAuths materializes all stored OCI credentials into a
// docker-style config.json (helmdex's isolated HELM_REGISTRY_CONFIG file),
// preserving entries written there by `helm registry login`.
//
// It is cheap when nothing changed: the write is skipped while the target is
// newer than the credential store.
func SyncRegistryAuths(registryConfigPath string) error {
	storePath, err := StorePath()
	if err != nil {
		// No resolvable user config dir means no credential store can
		// exist; there is nothing to materialize.
		return nil
	}
	storeInfo, err := os.Stat(storePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if cfgInfo, err := os.Stat(registryConfigPath); err == nil {
		if !cfgInfo.ModTime().Before(storeInfo.ModTime()) {
			return nil
		}
	}

	st, err := Load()
	if err != nil {
		return err
	}
	var ociCreds []Credential
	for _, c := range st.Credentials {
		if c.Kind == KindOCI && c.Secret != "" {
			ociCreds = append(ociCreds, c)
		}
	}
	if len(ociCreds) == 0 {
		return nil
	}
	return mergeDockerAuths(registryConfigPath, ociCreds, nil)
}

// RemoveRegistryAuth deletes host entries from a docker-style config.json.
func RemoveRegistryAuth(registryConfigPath, host string) error {
	return mergeDockerAuths(registryConfigPath, nil, []string{host})
}

// mergeDockerAuths reads (or initializes) a docker-style config file, upserts
// an auths entry per credential, removes the listed hosts, and writes the
// result atomically with 0600 permissions.
func mergeDockerAuths(path string, upserts []Credential, removeHosts []string) error {
	// Work on raw maps to preserve unknown fields helm may have written.
	root := map[string]json.RawMessage{}
	if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
		if err := json.Unmarshal(b, &root); err != nil {
			// A corrupt registry config would break every helm OCI call
			// anyway; rewrite it from scratch.
			root = map[string]json.RawMessage{}
		}
	}
	auths := map[string]json.RawMessage{}
	if raw, ok := root["auths"]; ok {
		_ = json.Unmarshal(raw, &auths)
	}

	for _, c := range upserts {
		entry, err := json.Marshal(dockerAuthEntry{
			Auth: base64.StdEncoding.EncodeToString([]byte(c.Username + ":" + c.Secret)),
		})
		if err != nil {
			return err
		}
		auths[c.Host] = entry
	}
	for _, h := range removeHosts {
		for _, k := range dockerAuthKeys(h) {
			delete(auths, k)
		}
	}

	rawAuths, err := json.Marshal(auths)
	if err != nil {
		return err
	}
	root["auths"] = rawAuths
	out, err := json.MarshalIndent(root, "", "\t")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".registry-config-*.json")
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
