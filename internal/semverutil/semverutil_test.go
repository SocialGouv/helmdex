package semverutil

import "testing"

func TestBestStable_PicksHighestStable(t *testing.T) {
	best, ok := BestStable([]string{"1.2.3", "v1.10.0", "1.9.9", "2.0.0-rc.1"})
	if !ok || best != "v1.10.0" {
		t.Fatalf("best = %q ok=%v", best, ok)
	}
}

// CI-pinned pseudo-versions (e.g. "0.0.0-sha.d68be124" on OCI charts in
// gitops repos) are prereleases: they must never be offered as upgrade
// targets, and must not break version scanning.
func TestBestStable_IgnoresPinnedPseudoVersions(t *testing.T) {
	best, ok := BestStable([]string{"0.0.0-sha.d68be124", "0.0.0-sha.aaaa111"})
	if ok {
		t.Fatalf("expected no stable, got %q", best)
	}

	best, ok = BestStable([]string{"0.0.0-sha.d68be124", "1.0.0"})
	if !ok || best != "1.0.0" {
		t.Fatalf("best = %q ok=%v", best, ok)
	}
}

func TestBestStable_ToleratesGarbage(t *testing.T) {
	best, ok := BestStable([]string{"", "not-a-version", "1.2", "1.2.3.4", "3.4.5"})
	if !ok || best != "3.4.5" {
		t.Fatalf("best = %q ok=%v", best, ok)
	}
}
