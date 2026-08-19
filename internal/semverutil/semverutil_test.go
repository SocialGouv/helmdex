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

func TestNewer(t *testing.T) {
	cases := []struct {
		candidate, current string
		want               bool
	}{
		{"v0.6.0", "v0.5.0", true},
		{"v0.5.0", "v0.5.0", false},
		{"v0.4.0", "v0.5.0", false}, // never offer a downgrade
		{"0.6.0", "v0.5.0", true},   // mixed v-prefix
		{"v9.9.9", "dev", false},    // unparseable current → no claim
		{"nonsense", "v0.5.0", false},

		// SemVer §11 pre-release precedence: numeric identifiers compare
		// numerically (a lexical compare would invert these two).
		{"v1.0.0-rc10", "v1.0.0-rc2", false}, // rc10/rc2 are alphanumeric: lexical, rc10 < rc2
		{"v1.0.0-rc.10", "v1.0.0-rc.2", true},
		{"v1.0.0-rc.2", "v1.0.0-rc.10", false},
		{"v1.0.0", "v1.0.0-rc.1", true},          // stable outranks pre-release
		{"v1.0.0-rc.1", "v1.0.0", false},         // pre-release never beats its stable
		{"v1.0.0-alpha.1", "v1.0.0-alpha", true}, // longer equal prefix ranks higher
		{"v1.0.0-1", "v1.0.0-alpha", false},      // numeric below alphanumeric
		{"v1.0.0-alpha", "v1.0.0-1", true},

		// Build metadata is ignored, even when it contains dashes.
		{"v1.0.1+meta-x", "v1.0.0", true},
		{"v1.0.0+meta-x", "v1.0.0", false},
		{"v1.0.0-", "v0.9.0", false}, // dangling dash is invalid
	}
	for _, c := range cases {
		if got := Newer(c.candidate, c.current); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.candidate, c.current, got, c.want)
		}
	}
}

func TestBestStableStillIgnoresPreReleases(t *testing.T) {
	best, ok := BestStable([]string{"v1.0.0-rc.10", "v0.9.0", "v1.0.0-rc.2"})
	if !ok || best != "v0.9.0" {
		t.Fatalf("BestStable = (%q, %v), want (v0.9.0, true)", best, ok)
	}
}
