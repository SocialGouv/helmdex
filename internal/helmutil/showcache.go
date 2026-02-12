package helmutil

import (
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
)

type ShowKind string

const (
	ShowKindReadme ShowKind = "readme"
	ShowKindValues ShowKind = "values"
)

func showCacheDir(repoRoot string) string {
	return filepath.Join(repoRoot, ".helmdex", "cache", "helmshow")
}

func showKey(repoURL, chart, version string) string {
	h := sha1.Sum([]byte(repoURL + "\n" + chart + "\n" + version))
	return hex.EncodeToString(h[:])
}

func ShowCachePath(repoRoot, repoURL, chart, version string, kind ShowKind) string {
	key := showKey(repoURL, chart, version)
	base := filepath.Join(showCacheDir(repoRoot), key)
	switch kind {
	case ShowKindReadme:
		return filepath.Join(base, "readme.txt")
	case ShowKindValues:
		return filepath.Join(base, "values.yaml")
	default:
		return filepath.Join(base, string(kind))
	}
}

func ReadShowCache(repoRoot, repoURL, chart, version string, kind ShowKind) (string, bool, error) {
	p := ShowCachePath(repoRoot, repoURL, chart, version, kind)
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	return string(b), true, nil
}

func WriteShowCache(repoRoot, repoURL, chart, version string, kind ShowKind, content string) error {
	p := ShowCachePath(repoRoot, repoURL, chart, version, kind)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(content), 0o644)
}

func ClearShowCache(repoRoot string) error {
	return os.RemoveAll(showCacheDir(repoRoot))
}

