package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectTarget(t *testing.T) {
	cases := []struct {
		goos, goarch, appimage, exe string
		wantPath, wantAsset         string
		wantErr                     string
	}{
		{"linux", "amd64", "/apps/helmdex.AppImage", "/tmp/mount/exe",
			"/apps/helmdex.AppImage", "helmdex-desktop-linux-amd64.AppImage", ""},
		{"linux", "arm64", "/a.AppImage", "/e",
			"/a.AppImage", "helmdex-desktop-linux-arm64.AppImage", ""},
		{"linux", "amd64", "", "/usr/local/bin/helmdex-desktop", "", "", "AppImage"},
		{"windows", "amd64", "", `C:\apps\helmdex-desktop.exe`,
			`C:\apps\helmdex-desktop.exe`, "helmdex-desktop-windows-amd64.exe", ""},
		{"darwin", "arm64", "", "/Applications/Helmdex.app/Contents/MacOS/x", "", "", "macOS"},
	}
	for _, c := range cases {
		target, err := detectTarget(c.goos, c.goarch, c.appimage, c.exe)
		if c.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("detectTarget(%s): err = %v, want mention of %q", c.goos, err, c.wantErr)
			}
			continue
		}
		if err != nil {
			t.Errorf("detectTarget(%s): %v", c.goos, err)
			continue
		}
		if target.Path != c.wantPath || target.AssetName != c.wantAsset {
			t.Errorf("detectTarget(%s) = %+v, want (%s, %s)", c.goos, target, c.wantPath, c.wantAsset)
		}
	}
}

// releaseStub serves an asset and its sha256 like a GitHub release download.
func releaseStub(t *testing.T, tag, asset string, content []byte, breakSum bool) *httptest.Server {
	t.Helper()
	sum := sha256.Sum256(content)
	hexSum := hex.EncodeToString(sum[:])
	if breakSum {
		hexSum = strings.Repeat("0", 64)
	}
	mux := http.NewServeMux()
	mux.HandleFunc(fmt.Sprintf("/%s/%s", tag, asset), func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(content)
	})
	mux.HandleFunc(fmt.Sprintf("/%s/%s.sha256", tag, asset), func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, "%s  %s\n", hexSum, asset)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func testTarget(t *testing.T) Target {
	t.Helper()
	path := filepath.Join(t.TempDir(), "helmdex.AppImage")
	if err := os.WriteFile(path, []byte("OLD BINARY"), 0o755); err != nil {
		t.Fatal(err)
	}
	return Target{Path: path, AssetName: "helmdex-desktop-linux-amd64.AppImage"}
}

func TestApplyReplacesTheTarget(t *testing.T) {
	target := testTarget(t)
	newBinary := []byte("NEW BINARY v9")
	stub := releaseStub(t, "v9.9.9", target.AssetName, newBinary, false)

	if err := Apply(context.Background(), target, "v9.9.9", stub.URL); err != nil {
		t.Fatalf("apply: %v", err)
	}
	got, err := os.ReadFile(target.Path)
	if err != nil || string(got) != string(newBinary) {
		t.Fatalf("target content = %q, %v", got, err)
	}
	info, _ := os.Stat(target.Path)
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("target lost its executable bit: %v", info.Mode())
	}
	for _, leftover := range []string{target.Path + ".old", target.Path + ".download"} {
		if _, err := os.Stat(leftover); !os.IsNotExist(err) {
			t.Fatalf("leftover %s still present", leftover)
		}
	}
}

// A symlinked install (launcher symlink → real AppImage) must update the
// REAL file: renaming over the link path would turn it into a regular file
// and leave the linked binary stale.
func TestDetectTargetResolvesSymlinks(t *testing.T) {
	real := filepath.Join(t.TempDir(), "helmdex.AppImage")
	if err := os.WriteFile(real, []byte("OLD"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "helmdex-desktop")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	target, err := detectTarget("linux", "amd64", link, "")
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if target.Path != real {
		t.Fatalf("target.Path = %q, want resolved %q", target.Path, real)
	}

	// End to end: the real file gets the update, the link stays a link.
	stub := releaseStub(t, "v9.9.9", target.AssetName, []byte("NEW"), false)
	if err := Apply(context.Background(), target, "v9.9.9", stub.URL); err != nil {
		t.Fatalf("apply: %v", err)
	}
	got, _ := os.ReadFile(real)
	if string(got) != "NEW" {
		t.Fatalf("real binary = %q, want NEW", got)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("launcher symlink was destroyed: %v %v", fi, err)
	}
}

// Concurrent Applies must never leave the install without a valid binary
// (unserialized, the two-rename dance reproducibly bricked ~2.6% of runs).
func TestApplyConcurrentNeverBricksTheTarget(t *testing.T) {
	target := testTarget(t)
	newBinary := []byte("NEW BINARY v9")
	stub := releaseStub(t, "v9.9.9", target.AssetName, newBinary, false)

	for i := 0; i < 40; i++ {
		errs := make(chan error, 2)
		for j := 0; j < 2; j++ {
			go func() {
				errs <- Apply(context.Background(), target, "v9.9.9", stub.URL)
			}()
		}
		for j := 0; j < 2; j++ {
			if err := <-errs; err != nil {
				t.Fatalf("iteration %d: apply failed: %v", i, err)
			}
		}
		got, err := os.ReadFile(target.Path)
		if err != nil {
			t.Fatalf("iteration %d: target is GONE: %v", i, err)
		}
		if string(got) != string(newBinary) {
			t.Fatalf("iteration %d: target corrupted: %q", i, got)
		}
	}
}

// The .sha256 parser must pick the row matching the asset, not blindly the
// first row of a multi-entry manifest.
func TestApplyChecksumManifestMatchesAssetRow(t *testing.T) {
	target := testTarget(t)
	newBinary := []byte("NEW BINARY v9")
	sum := sha256.Sum256(newBinary)

	mux := http.NewServeMux()
	mux.HandleFunc("/v9.9.9/"+target.AssetName, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(newBinary)
	})
	mux.HandleFunc("/v9.9.9/"+target.AssetName+".sha256", func(w http.ResponseWriter, r *http.Request) {
		// Wrong row first; the right row is flagged binary-mode ("*").
		_, _ = fmt.Fprintf(w, "%s  some-other-asset.zip\n%s  *%s\n",
			strings.Repeat("d", 64), hex.EncodeToString(sum[:]), target.AssetName)
	})
	stub := httptest.NewServer(mux)
	t.Cleanup(stub.Close)

	if err := Apply(context.Background(), target, "v9.9.9", stub.URL); err != nil {
		t.Fatalf("apply with manifest: %v", err)
	}
	got, _ := os.ReadFile(target.Path)
	if string(got) != string(newBinary) {
		t.Fatalf("target = %q", got)
	}
}

func TestApplyChecksumMismatchKeepsTarget(t *testing.T) {
	target := testTarget(t)
	stub := releaseStub(t, "v9.9.9", target.AssetName, []byte("EVIL"), true)

	err := Apply(context.Background(), target, "v9.9.9", stub.URL)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum error, got %v", err)
	}
	got, _ := os.ReadFile(target.Path)
	if string(got) != "OLD BINARY" {
		t.Fatalf("target was modified despite bad checksum: %q", got)
	}
	if _, err := os.Stat(target.Path + ".download"); !os.IsNotExist(err) {
		t.Fatal("failed download not cleaned up")
	}
}

func TestApplyMissingAssetKeepsTarget(t *testing.T) {
	target := testTarget(t)
	stub := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(stub.Close)

	err := Apply(context.Background(), target, "v9.9.9", stub.URL)
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected 404 error, got %v", err)
	}
	got, _ := os.ReadFile(target.Path)
	if string(got) != "OLD BINARY" {
		t.Fatalf("target was modified: %q", got)
	}
}

func TestApplyRejectsInvalidTags(t *testing.T) {
	target := testTarget(t)
	for _, tag := range []string{"", "latest", "../../etc", "v1.0.0/..", "v1;rm"} {
		if err := Apply(context.Background(), target, tag, "http://unused.invalid"); err == nil {
			t.Errorf("tag %q accepted", tag)
		}
	}
}

func TestApplyRejectsMalformedChecksumFile(t *testing.T) {
	target := testTarget(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("   \n"))
	})
	stub := httptest.NewServer(mux)
	t.Cleanup(stub.Close)

	err := Apply(context.Background(), target, "v9.9.9", stub.URL)
	if err == nil || !strings.Contains(err.Error(), "malformed checksum") {
		t.Fatalf("expected malformed checksum error, got %v", err)
	}
}

func TestCleanupLeftovers(t *testing.T) {
	target := testTarget(t)
	for _, suffix := range []string{".old", ".download"} {
		if err := os.WriteFile(target.Path+suffix, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	CleanupLeftovers(target.Path)
	for _, suffix := range []string{".old", ".download"} {
		if _, err := os.Stat(target.Path + suffix); !os.IsNotExist(err) {
			t.Fatalf("%s not removed", suffix)
		}
	}
}
