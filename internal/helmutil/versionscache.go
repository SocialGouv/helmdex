package helmutil

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Versions cache stores the result of `helm search repo <repo>/<chart> --versions`.
//
// Goal: keep the TUI responsive by showing cached versions immediately, while a
// background refresh updates the cache.

type versionsCacheFile struct {
	FetchedAt time.Time `json:"fetchedAt"`
	Versions  []string  `json:"versions"`
}

func versionsCacheDir(repoRoot string) string {
	return filepath.Join(repoRoot, ".helmdex", "cache", "helmversions")
}

func VersionsCacheKey(repoURL, chart string) string {
	h := sha1.Sum([]byte(repoURL + "\n" + chart))
	return hex.EncodeToString(h[:])
}

func VersionsCachePath(repoRoot, repoURL, chart string) string {
	key := VersionsCacheKey(repoURL, chart)
	return filepath.Join(versionsCacheDir(repoRoot), key, "versions.json")
}

func ReadVersionsCache(repoRoot, repoURL, chart string) (versions []string, fetchedAt time.Time, ok bool, err error) {
	p := VersionsCachePath(repoRoot, repoURL, chart)
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, time.Time{}, false, nil
		}
		return nil, time.Time{}, false, err
	}
	var f versionsCacheFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, time.Time{}, false, err
	}
	return f.Versions, f.FetchedAt, true, nil
}

func WriteVersionsCache(repoRoot, repoURL, chart string, versions []string) (fetchedAt time.Time, err error) {
	fetchedAt = time.Now().UTC()
	f := versionsCacheFile{FetchedAt: fetchedAt, Versions: versions}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return time.Time{}, err
	}
	p := VersionsCachePath(repoRoot, repoURL, chart)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return time.Time{}, err
	}
	if err := os.WriteFile(p, b, 0o644); err != nil {
		return time.Time{}, err
	}
	return fetchedAt, nil
}

func VersionsCacheStale(fetchedAt time.Time, ttl time.Duration, now time.Time) bool {
	if fetchedAt.IsZero() {
		return true
	}
	return now.Sub(fetchedAt) > ttl
}

func ClearVersionsCache(repoRoot string) error {
	return os.RemoveAll(versionsCacheDir(repoRoot))
}
