// Package selfupdate replaces the running desktop binary with a published
// release asset: download next to the target, verify the asset's sha256,
// atomically swap, keep a rollback copy until the swap succeeded.
package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
)

// Target describes what a self-update would replace.
type Target struct {
	// Path is the file the update overwrites: the AppImage file on Linux,
	// the portable exe on Windows.
	Path string
	// AssetName is the release asset matching this installation.
	AssetName string
}

// DetectTarget resolves the updatable artifact for this process, or an
// explicit error saying why automatic updates are unsupported here (the UI
// then falls back to the release page link).
func DetectTarget() (Target, error) {
	exe, err := os.Executable()
	if err != nil {
		return Target{}, fmt.Errorf("resolve current executable: %w", err)
	}
	return detectTarget(runtime.GOOS, runtime.GOARCH, os.Getenv("APPIMAGE"), exe)
}

func detectTarget(goos, goarch, appimage, exe string) (Target, error) {
	switch goos {
	case "linux":
		// The AppImage runtime executes an extracted copy; the file to
		// replace is the AppImage itself, exposed via $APPIMAGE.
		if appimage == "" {
			return Target{}, fmt.Errorf("automatic update needs the AppImage build (this binary was not launched from an AppImage); use the release page instead")
		}
		return Target{Path: resolveLinks(appimage), AssetName: "helmdex-desktop-linux-" + goarch + ".AppImage"}, nil
	case "windows":
		return Target{Path: resolveLinks(exe), AssetName: "helmdex-desktop-windows-" + goarch + ".exe"}, nil
	case "darwin":
		return Target{}, fmt.Errorf("automatic update is not supported on macOS yet; use the release page instead")
	default:
		return Target{}, fmt.Errorf("automatic update is not supported on %s", goos)
	}
}

// resolveLinks follows symlinks so the update replaces the real file. A
// rename over a symlink path would turn the link into a regular file and
// leave the linked binary stale for every other launcher referencing it.
func resolveLinks(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return path
}

var (
	tagPattern    = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+[0-9A-Za-z.+-]*$`)
	sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// applyMu serializes updates: two concurrent Applies share the .download
// path and would interleave writes and race the swap (a reproduced ~2.6%
// chance of leaving NO binary at all with the two-rename sequence).
var applyMu sync.Mutex

// Apply downloads downloadBase/<tag>/<asset>, verifies it against the
// published .sha256, and swaps it in place of target.Path. On POSIX the swap
// is a rename-over (the target path never stops existing); Windows locks the
// running image, so it falls back to a two-step swap whose .old leftover is
// cleaned at next launch.
func Apply(ctx context.Context, target Target, tag, downloadBase string) error {
	applyMu.Lock()
	defer applyMu.Unlock()

	if !tagPattern.MatchString(tag) {
		return fmt.Errorf("invalid release tag %q", tag)
	}
	info, err := os.Stat(target.Path)
	if err != nil {
		return fmt.Errorf("update target: %w", err)
	}

	assetURL := fmt.Sprintf("%s/%s/%s", downloadBase, tag, target.AssetName)
	wantSum, err := fetchChecksum(ctx, assetURL+".sha256", target.AssetName)
	if err != nil {
		return err
	}

	// Download beside the target so the final rename stays on one filesystem.
	tmp := target.Path + ".download"
	gotSum, err := downloadTo(ctx, assetURL, tmp)
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if gotSum != wantSum {
		_ = os.Remove(tmp)
		return fmt.Errorf("checksum mismatch for %s: got %s, want %s", target.AssetName, gotSum, wantSum)
	}
	if err := os.Chmod(tmp, info.Mode().Perm()|0o755); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("mark update executable: %w", err)
	}
	return swap(target.Path, tmp)
}

// swap installs tmp at path. POSIX: hardlink the current binary as rollback,
// then rename-over — path exists at every instant, so a crash can never
// leave the install without a binary. Windows (or filesystems without
// hardlinks): two-step swap with rollback.
func swap(path, tmp string) error {
	old := path + ".old"
	_ = os.Remove(old)

	if runtime.GOOS != "windows" {
		if err := os.Link(path, old); err == nil {
			if err := os.Rename(tmp, path); err != nil {
				_ = os.Remove(old)
				_ = os.Remove(tmp)
				return fmt.Errorf("install update: %w", err)
			}
			_ = os.Remove(old)
			return nil
		}
		// Hardlinks unavailable here: fall through to the two-step swap.
	}

	if err := os.Rename(path, old); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("stash current binary: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		// Roll back: better the old version than none.
		if rbErr := os.Rename(old, path); rbErr != nil {
			return fmt.Errorf("install update: %v (ROLLBACK FAILED: %v — previous binary is at %s)", err, rbErr, old)
		}
		return fmt.Errorf("install update: %w", err)
	}
	_ = os.Remove(old)
	return nil
}

// CleanupLeftovers removes files a previous update could not delete (the
// .old image stays locked on Windows while the app runs).
func CleanupLeftovers(targetPath string) {
	_ = os.Remove(targetPath + ".old")
	_ = os.Remove(targetPath + ".download")
}

// fetchChecksum reads a sha256sum-format file ("<hex>  <filename>" lines)
// and returns the hash for assetName. Single-hash files without a filename
// column are accepted too; a multi-entry manifest must name the asset.
func fetchChecksum(ctx context.Context, url, assetName string) (string, error) {
	body, err := get(ctx, url)
	if err != nil {
		return "", err
	}
	defer body.Close() //nolint:errcheck // response body close
	raw, err := io.ReadAll(io.LimitReader(body, 8192))
	if err != nil {
		return "", fmt.Errorf("read checksum from %s: %w", url, err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 0 || !sha256Pattern.MatchString(strings.ToLower(fields[0])) {
			continue
		}
		if len(fields) == 1 {
			return strings.ToLower(fields[0]), nil
		}
		// sha256sum may prefix binary-mode filenames with "*".
		if filepath.Base(strings.TrimPrefix(fields[1], "*")) == assetName {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("malformed checksum file at %s (no entry for %s)", url, assetName)
}

func downloadTo(ctx context.Context, url, path string) (sha string, err error) {
	body, err := get(ctx, url)
	if err != nil {
		return "", err
	}
	defer body.Close() //nolint:errcheck // response body close

	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o700)
	if err != nil {
		return "", fmt.Errorf("create %s: %w", path, err)
	}
	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, hasher), body); err != nil {
		_ = f.Close()
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("finish writing %s: %w", path, err)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// get issues a GET without a client timeout: assets are large and the
// caller's context bounds the whole operation.
func get(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request for %s: %w", url, err)
	}
	req.Header.Set("User-Agent", "helmdex")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", url, err)
	}
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 256))
		_ = res.Body.Close()
		return nil, fmt.Errorf("fetch %s: status %d: %s", url, res.StatusCode, strings.TrimSpace(string(body)))
	}
	return res.Body, nil
}
