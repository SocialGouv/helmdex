package helmutil

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Charts cache stores the result of `helm search repo <repo> -o json` normalized
// to a list of chart names.
//
// Goal: keep the TUI responsive by showing cached chart names immediately, while
// a background refresh updates the cache.

type chartsCacheFile struct {
	FetchedAt time.Time `json:"fetchedAt"`
	Charts    []string  `json:"charts"`
}

func chartsCacheDir(repoRoot string) string {
	return filepath.Join(repoRoot, ".helmdex", "cache", "helmcharts")
}

func ChartsCacheKey(repoURL string) string {
	h := sha1.Sum([]byte(repoURL))
	return hex.EncodeToString(h[:])
}

func ChartsCachePath(repoRoot, repoURL string) string {
	key := ChartsCacheKey(repoURL)
	return filepath.Join(chartsCacheDir(repoRoot), key, "charts.json")
}

func ReadChartsCache(repoRoot, repoURL string) (charts []string, fetchedAt time.Time, ok bool, err error) {
	p := ChartsCachePath(repoRoot, repoURL)
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, time.Time{}, false, nil
		}
		return nil, time.Time{}, false, err
	}
	var f chartsCacheFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, time.Time{}, false, err
	}
	return f.Charts, f.FetchedAt, true, nil
}

func WriteChartsCache(repoRoot, repoURL string, charts []string) (fetchedAt time.Time, err error) {
	fetchedAt = time.Now().UTC()
	f := chartsCacheFile{FetchedAt: fetchedAt, Charts: charts}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return time.Time{}, err
	}
	p := ChartsCachePath(repoRoot, repoURL)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return time.Time{}, err
	}
	if err := os.WriteFile(p, b, 0o644); err != nil {
		return time.Time{}, err
	}
	return fetchedAt, nil
}

func ChartsCacheStale(fetchedAt time.Time, ttl time.Duration, now time.Time) bool {
	if fetchedAt.IsZero() {
		return true
	}
	return now.Sub(fetchedAt) > ttl
}

func ClearChartsCache(repoRoot string) error {
	return os.RemoveAll(chartsCacheDir(repoRoot))
}

