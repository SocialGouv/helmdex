// Package testutil builds the hermetic fixtures shared by helmdex's CLI,
// server and TUI end-to-end tests: a stand-in `helm` binary, an isolated
// process environment, and throwaway repos wired to a local git "remote".
//
// It is test-only infrastructure and must never be imported by production
// code.
package testutil

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// FakeHelmDir builds the fakehelm binary if needed and returns the directory
// holding it under the name `helm`, ready to be prepended to PATH.
//
// The binary lands in <repoRoot>/bin/fakehelm/ so the same artifact serves the
// Go tests and the JS harnesses. It is rebuilt whenever its sources are newer,
// and written through a temp file + rename so concurrent `go test ./...`
// packages never observe a half-written binary.
func FakeHelmDir(t *testing.T) string {
	t.Helper()
	buildFakeHelmOnce.Do(func() {
		fakeHelmDir, buildFakeHelmErr = buildFakeHelm(RepoRoot(t))
	})
	if buildFakeHelmErr != nil {
		t.Fatalf("build fakehelm: %v", buildFakeHelmErr)
	}
	return fakeHelmDir
}

var (
	buildFakeHelmOnce sync.Once
	fakeHelmDir       string
	buildFakeHelmErr  error
)

func buildFakeHelm(repoRoot string) (string, error) {
	outDir := filepath.Join(repoRoot, "bin", "fakehelm")
	binPath := filepath.Join(outDir, "helm")
	srcDir := filepath.Join(repoRoot, "internal", "testutil", "fakehelm")

	fresh, err := isFresherThan(binPath, srcDir)
	if err != nil {
		return "", err
	}
	if fresh {
		return outDir, nil
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(outDir, "helm-*.tmp")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		return "", err
	}
	defer os.Remove(tmpPath) //nolint:errcheck // removed by the rename on success

	cmd := exec.Command("go", "build", "-mod=vendor", "-o", tmpPath, "./internal/testutil/fakehelm")
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("go build fakehelm: %v\n%s", err, out)
	}
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(tmpPath, binPath); err != nil {
		return "", err
	}
	return outDir, nil
}

// isFresherThan reports whether bin exists and is newer than every file under
// srcDir.
func isFresherThan(bin, srcDir string) (bool, error) {
	st, err := os.Stat(bin)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	binTime := st.ModTime()
	fresh := true
	err = filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.ModTime().After(binTime) {
			fresh = false
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return fresh, nil
}

// Hermetic isolates the current test process so helmdex operations touch
// neither the network, the user's home, nor any Helm installation:
//
//   - fakehelm is first on PATH and HELMDEX_E2E_STUB_HELM makes helmdex resolve
//     Helm through PATH instead of downloading a bundled release
//   - HOME, XDG_* and HELMDEX_CACHE_DIR point at throwaway directories
//   - HELMDEX_USER_CONFIG points at a non-existent file, so no user-level
//     helmdex config leaks into the test
//
// It returns the path to the fakehelm invocation log, which stays empty until
// a Helm command actually runs (see FakeHelmCalls).
func Hermetic(t *testing.T) string {
	t.Helper()

	helmDir := FakeHelmDir(t)
	t.Setenv("PATH", helmDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HELMDEX_E2E_STUB_HELM", "1")

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("HELMDEX_CACHE_DIR", filepath.Join(home, "helmdex-cache"))
	t.Setenv("HELMDEX_USER_CONFIG", filepath.Join(home, "absent-user-config.yaml"))

	logPath := filepath.Join(t.TempDir(), "fakehelm.log")
	t.Setenv("HELMDEX_FAKE_HELM_LOG", logPath)
	return logPath
}

// FakeHelmCalls returns the helm invocations recorded so far, each as the
// joined argument list (e.g. "dependency update"). Missing log file means no
// Helm command ran.
func FakeHelmCalls(t *testing.T, logPath string) []string {
	t.Helper()
	b, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read fakehelm log %s: %v", logPath, err)
	}
	out := []string{}
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var rec struct {
			Args []string `json:"args"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("parse fakehelm log line %q: %v", line, err)
		}
		out = append(out, strings.Join(rec.Args, " "))
	}
	return out
}

// RepoRoot locates the helmdex checkout containing this file.
func RepoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(thisFile)
	for i := 0; i < 10; i++ {
		cand := filepath.Clean(filepath.Join(dir, strings.Repeat("../", i)))
		if _, err := os.Stat(filepath.Join(cand, "go.mod")); err == nil {
			return cand
		}
	}
	wd, _ := os.Getwd()
	t.Fatalf("could not locate repo root (go.mod) from %s (wd=%s)", thisFile, wd)
	return ""
}

// FixturePath resolves a path under the checked-in fixtures/ directory.
func FixturePath(t *testing.T, parts ...string) string {
	t.Helper()
	return filepath.Join(append([]string{RepoRoot(t), "fixtures"}, parts...)...)
}
