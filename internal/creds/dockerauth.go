package creds

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"sort"
)

// SyncRegistryAuths reconciles helmdex's stored OCI credentials into a
// docker-style config.json (helmdex's isolated HELM_REGISTRY_CONFIG file):
// it upserts an auths entry for every stored OCI host and removes entries for
// hosts helmdex previously materialized but no longer holds — so a logout
// actually stops helm from authenticating.
//
// Hosts written by other tools (e.g. `helm registry login`) are never touched:
// helmdex tracks the hosts it manages in a sidecar file and only prunes those.
//
// It is cheap when nothing changed: the write is skipped while the config is
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

	storeMu.Lock()
	defer storeMu.Unlock()

	st, err := Load()
	if err != nil {
		return err
	}
	var ociCreds []Credential
	current := map[string]struct{}{}
	for _, c := range st.Credentials {
		if c.Kind == KindOCI && c.Secret != "" {
			ociCreds = append(ociCreds, c)
			current[c.Host] = struct{}{}
		}
	}

	sidecar := registryConfigPath + ".helmdex-hosts"
	previous := readManagedHosts(sidecar)
	var removeHosts []string
	for _, h := range previous {
		if _, ok := current[h]; !ok {
			removeHosts = append(removeHosts, h)
		}
	}
	if len(ociCreds) == 0 && len(removeHosts) == 0 {
		return nil
	}
	if err := mergeDockerAuths(registryConfigPath, ociCreds, removeHosts); err != nil {
		return err
	}
	return writeManagedHosts(sidecar, current)
}

// RemoveRegistryAuth deletes host entries from a docker-style config.json and
// drops the host from the managed-hosts sidecar.
func RemoveRegistryAuth(registryConfigPath, host string) error {
	storeMu.Lock()
	defer storeMu.Unlock()
	if err := mergeDockerAuths(registryConfigPath, nil, []string{host}); err != nil {
		return err
	}
	sidecar := registryConfigPath + ".helmdex-hosts"
	managed := map[string]struct{}{}
	for _, h := range readManagedHosts(sidecar) {
		if h != host {
			managed[h] = struct{}{}
		}
	}
	return writeManagedHosts(sidecar, managed)
}

func readManagedHosts(sidecar string) []string {
	b, err := os.ReadFile(sidecar)
	if err != nil {
		return nil
	}
	var hosts []string
	if err := json.Unmarshal(b, &hosts); err != nil {
		return nil
	}
	return hosts
}

func writeManagedHosts(sidecar string, hosts map[string]struct{}) error {
	list := make([]string, 0, len(hosts))
	for h := range hosts {
		list = append(list, h)
	}
	sort.Strings(list)
	b, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return writeFileAtomic0600(sidecar, b, 0o755)
}

// mergeDockerAuths reads (or initializes) a docker-style config file, upserts
// the `auth` field of an entry per credential (preserving any other fields on
// that host, such as identitytoken), removes the listed hosts, and writes the
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
		// Preserve any sibling fields (identitytoken, email, …) another tool
		// wrote for this host; only (re)set the base64 basic-auth blob.
		entry := map[string]json.RawMessage{}
		if raw, ok := auths[c.Host]; ok {
			_ = json.Unmarshal(raw, &entry)
		}
		authVal, err := json.Marshal(base64.StdEncoding.EncodeToString([]byte(c.Username + ":" + c.Secret)))
		if err != nil {
			return err
		}
		entry["auth"] = authVal
		merged, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		auths[c.Host] = merged
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
	return writeFileAtomic0600(path, out, 0o755)
}
