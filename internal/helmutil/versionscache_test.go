package helmutil

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestVersionsCacheReadWriteRoundTrip(t *testing.T) {
	repoRoot := t.TempDir()
	repoURL := "https://example.com/charts"
	chart := "nginx"

	vs := []string{"1.2.3", "1.2.2"}
	if _, err := WriteVersionsCache(repoRoot, repoURL, chart, vs); err != nil {
		t.Fatalf("WriteVersionsCache: %v", err)
	}

	got, fetchedAt, ok, err := ReadVersionsCache(repoRoot, repoURL, chart)
	if err != nil {
		t.Fatalf("ReadVersionsCache: %v", err)
	}
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if fetchedAt.IsZero() {
		t.Fatalf("expected fetchedAt to be set")
	}
	if len(got) != len(vs) {
		t.Fatalf("expected %d versions, got %d", len(vs), len(got))
	}
	for i := range vs {
		if got[i] != vs[i] {
			t.Fatalf("versions[%d]: expected %q, got %q", i, vs[i], got[i])
		}
	}

	// Ensure file exists at the expected path.
	p := VersionsCachePath(repoRoot, repoURL, chart)
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("expected cache file to exist at %s: %v", p, err)
	}
	if filepath.Base(p) != "versions.json" {
		t.Fatalf("unexpected cache filename: %s", p)
	}
}

func TestVersionsCacheStale(t *testing.T) {
	now := time.Date(2026, 2, 23, 10, 0, 0, 0, time.UTC)
	ttl := 15 * time.Minute

	if !VersionsCacheStale(time.Time{}, ttl, now) {
		t.Fatalf("zero fetchedAt should be stale")
	}
	if VersionsCacheStale(now.Add(-10*time.Minute), ttl, now) {
		t.Fatalf("10m old should not be stale")
	}
	if !VersionsCacheStale(now.Add(-16*time.Minute), ttl, now) {
		t.Fatalf("16m old should be stale")
	}
}

