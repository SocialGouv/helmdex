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

// Newer reports whether candidate is a strictly higher SemVer than current.
// Unparseable versions (e.g. a "dev" build) never signal an update: claiming
// one without a comparable baseline would be a guess.
func Newer(candidate, current string) bool {
	c, okC := parseSemver(candidate)
	cur, okCur := parseSemver(current)
	return okC && okCur && cur.Less(c)
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
	return comparePre(a.Pre, b.Pre) < 0
}

// comparePre orders pre-release strings per SemVer §11: stable ("") outranks
// any pre-release; identifiers compare dot by dot, numerically when both are
// numeric (rc10 > rc2, which a lexical compare gets backwards), numeric below
// alphanumeric, and a shorter identifier list below a longer equal prefix.
func comparePre(a, b string) int {
	if a == b {
		return 0
	}
	if a == "" {
		return 1
	}
	if b == "" {
		return -1
	}
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		x, y := as[i], bs[i]
		if x == y {
			continue
		}
		xn, xErr := strconv.Atoi(x)
		yn, yErr := strconv.Atoi(y)
		switch {
		case xErr == nil && yErr == nil:
			if xn != yn {
				if xn < yn {
					return -1
				}
				return 1
			}
		case xErr == nil:
			return -1
		case yErr == nil:
			return 1
		default:
			if x < y {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(as) < len(bs):
		return -1
	case len(as) > len(bs):
		return 1
	}
	return 0
}

func parseSemver(raw string) (semver, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return semver{}, false
	}
	s = strings.TrimPrefix(s, "v")
	// Trim build metadata.
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	pre := ""
	hadDash := false
	if i := strings.IndexByte(s, '-'); i >= 0 {
		hadDash = true
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
	// A dash in the version core must introduce a non-empty pre-release.
	// Checked on the post-metadata string: a dash inside build metadata
	// ("v1.0.0+meta-x") is legal and already trimmed away.
	if hadDash && pre == "" {
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
