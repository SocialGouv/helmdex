package presets

import (
	"os"
	"path/filepath"
	"strings"

	semver "github.com/Masterminds/semver/v3"
)

// PresetCoverage represents which chart versions are covered by presets in a
// source cache for a given chart.
//
// Coverage is inferred from subdirectory names under:
//
//	<chartRoot>/<dir>
//
// where <dir> can be:
//   - an exact version directory (e.g. "15.0.0")
//   - a semver constraint directory (e.g. ">=1.0.0 <2.0.0")
//
// Other directory names are ignored.
type PresetCoverage struct {
	exactCanonical map[string]struct{}
	constraints    []*semver.Constraints
}

// ReadPresetCoverage reads coverage information from the preset cache chart root.
//
// ok=false indicates chartRoot does not exist (no coverage info available).
func ReadPresetCoverage(chartRoot string) (cov PresetCoverage, ok bool, err error) {
	chartRoot = filepath.Clean(strings.TrimSpace(chartRoot))
	if chartRoot == "" {
		return PresetCoverage{}, false, nil
	}
	entries, err := os.ReadDir(chartRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return PresetCoverage{}, false, nil
		}
		return PresetCoverage{}, false, err
	}

	cov = PresetCoverage{exactCanonical: map[string]struct{}{}, constraints: nil}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := strings.TrimSpace(e.Name())
		if name == "" {
			continue
		}
		if v, err := semver.NewVersion(name); err == nil {
			cov.exactCanonical[v.String()] = struct{}{}
			continue
		}
		if c, err := semver.NewConstraint(name); err == nil {
			cov.constraints = append(cov.constraints, c)
			continue
		}
	}
	return cov, true, nil
}

func (c PresetCoverage) Empty() bool {
	return len(c.exactCanonical) == 0 && len(c.constraints) == 0
}

// Supports reports whether the given raw version is covered by this preset coverage.
// Invalid semver strings are treated as unsupported.
func (c PresetCoverage) Supports(rawVersion string) bool {
	rawVersion = strings.TrimSpace(rawVersion)
	if rawVersion == "" {
		return false
	}
	v, err := semver.NewVersion(rawVersion)
	if err != nil {
		return false
	}
	if _, ok := c.exactCanonical[v.String()]; ok {
		return true
	}
	for _, con := range c.constraints {
		if con == nil {
			continue
		}
		if con.Check(v) {
			return true
		}
	}
	return false
}

// Filter returns the subset of candidate versions that are supported.
// Order is preserved.
func (c PresetCoverage) Filter(candidates []string) []string {
	if len(candidates) == 0 {
		return nil
	}
	out := make([]string, 0, len(candidates))
	for _, v := range candidates {
		if c.Supports(v) {
			out = append(out, v)
		}
	}
	return out
}
