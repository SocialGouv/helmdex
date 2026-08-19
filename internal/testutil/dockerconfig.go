package testutil

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// WriteDockerConfig points DOCKER_CONFIG at a temp dir holding an auths
// entry for host, and returns the config.json path. Shared fixture for
// credential-detection tests.
func WriteDockerConfig(t *testing.T, host, user, secret string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", dir)
	auth := base64.StdEncoding.EncodeToString([]byte(user + ":" + secret))
	cfg := map[string]any{"auths": map[string]any{host: map[string]string{"auth": auth}}}
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
