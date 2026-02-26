package semverutil

import (
	"fmt"
	"strconv"
	"strings"
)

// BestStable picks the highest SemVer from the list, ignoring pre-releases.
// It is intentionally small and tolerant (it also accepts leading "v").
// If no stable SemVer exists, ok=false.
func BestStable(versions []string) (best string, ok bool) {
	var bestV semver
	for _, raw := range versions {
		v, valid := parseSemver(raw)
		if !valid {
			continue
		}
		if v.Pre != "" {
			continue
		}
		if !ok || bestV.Less(v) {
			bestV = v
			best = raw
			ok = true
		}
	}
	return best, ok
}

type semver struct {
	Major int
	Minor int
	Patch int
	Pre   string
}

func (a semver) Less(b semver) bool {
	if a.Major != b.Major {
		return a.Major < b.Major
	}
	if a.Minor != b.Minor {
		return a.Minor < b.Minor
	}
	if a.Patch != b.Patch {
		return a.Patch < b.Patch
	}
	// Stable is considered greater than pre-release.
	if a.Pre == "" && b.Pre != "" {
		return false
	}
	if a.Pre != "" && b.Pre == "" {
		return true
	}
	// Both pre-release: lexical fallback.
	return a.Pre < b.Pre
}

func parseSemver(raw string) (semver, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return semver{}, false
	}
	if strings.HasPrefix(s, "v") {
		s = strings.TrimPrefix(s, "v")
	}
	// Trim build metadata.
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	pre := ""
	if i := strings.IndexByte(s, '-'); i >= 0 {
		pre = s[i+1:]
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return semver{}, false
	}
	maj, err := strconv.Atoi(parts[0])
	if err != nil {
		return semver{}, false
	}
	min, err := strconv.Atoi(parts[1])
	if err != nil {
		return semver{}, false
	}
	pat, err := strconv.Atoi(parts[2])
	if err != nil {
		return semver{}, false
	}
	if maj < 0 || min < 0 || pat < 0 {
		return semver{}, false
	}
	// Basic validation: pre-release must be non-empty if dash present.
	if strings.Contains(raw, "-") && pre == "" {
		return semver{}, false
	}
	return semver{Major: maj, Minor: min, Patch: pat, Pre: pre}, true
}

func (v semver) String() string {
	if v.Pre == "" {
		return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	}
	return fmt.Sprintf("%d.%d.%d-%s", v.Major, v.Minor, v.Patch, v.Pre)
}
