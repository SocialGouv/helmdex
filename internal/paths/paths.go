// Package paths centralizes where helmdex keeps its per-repo state
// (helm envs, caches, catalog snapshots, depmeta).
package paths

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateSegment rejects a value that would not stay a single path component
// once joined under StateDir.
//
// Every user-supplied string that becomes a directory or file name in helmdex
// state goes through here — instance names, dependency ids, source names.
// These strings reach the filesystem from URL segments, request bodies and
// config files, so an unvalidated one is an arbitrary-path primitive: it
// reads, writes or deletes anywhere the process can.
//
// kind names the value in the error message ("instance name", "dependency
// id", ...).
func ValidateSegment(kind, value string) error {
	v := strings.TrimSpace(value)
	if v == "" {
		return fmt.Errorf("%s is required", kind)
	}
	if v == "." || v == ".." || strings.ContainsAny(v, `/\`) || strings.Contains(v, "..") {
		return fmt.Errorf("invalid %s %q", kind, value)
	}
	return nil
}

// StateDir returns the root directory for helmdex state of a given repo.
//
// A repo that opts into helmdex (helmdex.yaml at its root) keeps state
// in-repo under `.helmdex/`. Any other repo stays fully helmdex-agnostic:
// state lives under the user cache dir, keyed by the repo's absolute path,
// so helmdex never adds files to the repo itself.
func StateDir(repoRoot string) string {
	if OptedIn(repoRoot) {
		return filepath.Join(repoRoot, ".helmdex")
	}
	abs, err := filepath.Abs(repoRoot)
	if err != nil {
		abs = repoRoot
	}
	abs = filepath.Clean(abs)
	h := sha256.Sum256([]byte(abs))
	// Basename prefix keeps cache dirs recognizable; the hash disambiguates.
	name := filepath.Base(abs) + "-" + hex.EncodeToString(h[:8])
	return filepath.Join(cacheHome(), "repos", name)
}

// OptedIn reports whether the repo opted into helmdex (helmdex.yaml at its
// root). Non-opted repos must stay byte-identical: no generated files, no
// in-repo state.
func OptedIn(repoRoot string) bool {
	_, err := os.Stat(filepath.Join(repoRoot, "helmdex.yaml"))
	return err == nil
}

// State joins parts under StateDir(repoRoot).
func State(repoRoot string, parts ...string) string {
	return filepath.Join(append([]string{StateDir(repoRoot)}, parts...)...)
}

func cacheHome() string {
	if v := os.Getenv("HELMDEX_CACHE_DIR"); v != "" {
		return v
	}
	base, err := os.UserCacheDir()
	if err != nil {
		// No resolvable home: degrade to a world-visible but functional
		// location rather than failing every path construction.
		base = os.TempDir()
	}
	return filepath.Join(base, "helmdex")
}
