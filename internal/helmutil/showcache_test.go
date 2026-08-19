package helmutil

import (
	"os"
	"testing"
)

// An absent artifact must never be cached as blank content, and a pre-existing
// empty cache file (from an older build) must read as a miss — otherwise the
// UI shows an empty tab that looks broken instead of "no README/schema".
func TestShowCacheEmptyIsMiss(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HELMDEX_CACHE_DIR", t.TempDir())
	const url, chart, ver = "oci://reg.test/org/c", "c", "1.0.0"

	// Writing empty content is a no-op: nothing is cached.
	if err := WriteShowCache(root, url, chart, ver, ShowKindReadme, "   \n"); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := ReadShowCache(root, url, chart, ver, ShowKindReadme); err != nil || ok {
		t.Fatalf("empty content must not be cached: ok=%v err=%v", ok, err)
	}

	// A stale empty file on disk reads as a miss.
	p := ShowCachePath(root, url, chart, ver, ShowKindSchema)
	if err := os.MkdirAll(dirOf(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := ReadShowCache(root, url, chart, ver, ShowKindSchema); err != nil || ok {
		t.Fatalf("empty cache file must read as miss: ok=%v err=%v", ok, err)
	}

	// Real content still round-trips.
	if err := WriteShowCache(root, url, chart, ver, ShowKindValues, "replicaCount: 1\n"); err != nil {
		t.Fatal(err)
	}
	if s, ok, err := ReadShowCache(root, url, chart, ver, ShowKindValues); err != nil || !ok || s == "" {
		t.Fatalf("non-empty content must round-trip: %q ok=%v err=%v", s, ok, err)
	}
}

func dirOf(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[:i]
		}
	}
	return "."
}
