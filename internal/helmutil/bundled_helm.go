package helmutil

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	bundledHelmVersion = "v4.1.1"
	getHelmBaseURL     = "https://get.helm.sh"
)

var (
	helmPathMu     sync.Mutex
	helmPathCached string

	bundledHelmEventMu   sync.Mutex
	bundledHelmEventSink chan<- BundledHelmEvent
)

// BundledHelmEvent is emitted when helmdex needs to download the pinned Helm binary.
// It is intended for UX (e.g. TUI spinners) and must be non-blocking.
type BundledHelmEvent struct {
	Kind    BundledHelmEventKind
	Version string
	Err     string
}

type BundledHelmEventKind string

const (
	BundledHelmDownloadStart BundledHelmEventKind = "download_start"
	BundledHelmDownloadDone  BundledHelmEventKind = "download_done"
)

// SetBundledHelmEventSink installs a non-blocking sink for bundled-helm download events.
// Passing nil disables event emission.
func SetBundledHelmEventSink(sink chan<- BundledHelmEvent) {
	bundledHelmEventMu.Lock()
	defer bundledHelmEventMu.Unlock()
	bundledHelmEventSink = sink
}

func emitBundledHelmEvent(ev BundledHelmEvent) {
	bundledHelmEventMu.Lock()
	sink := bundledHelmEventSink
	bundledHelmEventMu.Unlock()
	if sink == nil {
		return
	}
	select {
	case sink <- ev:
	default:
		// Never block background operations for UX.
	}
}

// helmCommandPath resolves the Helm binary to execute.
//
// Default (recommended): use a bundled/pinned Helm binary at ~/.helmdex/bin/helm
// and auto-download it if missing or wrong version.
//
// Escape hatches (opt-in):
// - HELMDEX_NO_BUNDLED_HELM=1: do not download; require helm on PATH
// - HELMDEX_E2E_STUB_HELM=1: do not download; require helm on PATH (tests)
func helmCommandPath(ctx context.Context) (string, error) {
	helmPathMu.Lock()
	cached := helmPathCached
	helmPathMu.Unlock()
	if cached != "" {
		return cached, nil
	}

	p, err := computeHelmCommandPath(ctx)
	if err != nil {
		return "", err
	}

	helmPathMu.Lock()
	if helmPathCached == "" {
		helmPathCached = p
	}
	helmPathMu.Unlock()

	return p, nil
}

func computeHelmCommandPath(ctx context.Context) (string, error) {
	if strings.TrimSpace(os.Getenv("HELMDEX_E2E_STUB_HELM")) == "1" {
		return lookPathHelm()
	}
	if strings.TrimSpace(os.Getenv("HELMDEX_NO_BUNDLED_HELM")) == "1" {
		return lookPathHelm()
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home dir: %w", err)
	}
	dest := bundledHelmPathForHome(home, runtime.GOOS)
	if err := ensureBundledHelm(ctx, dest, runtime.GOOS, runtime.GOARCH); err != nil {
		return "", err
	}
	return dest, nil
}

func bundledHelmPathForHome(home, goos string) string {
	name := "helm"
	if goos == "windows" {
		name = "helm.exe"
	}
	return filepath.Join(home, ".helmdex", "bin", name)
}

func lookPathHelm() (string, error) {
	name := "helm"
	if runtime.GOOS == "windows" {
		name = "helm.exe"
	}
	p, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("helm not found on PATH (set %s=0 or install helm): %w", "HELMDEX_NO_BUNDLED_HELM", err)
	}
	return p, nil
}

func ensureBundledHelm(ctx context.Context, dest, goos, goarch string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}

	lockDir := filepath.Join(filepath.Dir(dest), ".helm-download.lock")
	releaseLock, err := acquireDirLock(ctx, lockDir, 5*time.Minute)
	if err != nil {
		return err
	}
	defer releaseLock()

	// Check again after acquiring lock.
	if ok, _ := helmBinaryIsVersion(ctx, dest, bundledHelmVersion); ok {
		return nil
	}

	// We'll download/install. Emit UI events best-effort.
	emitBundledHelmEvent(BundledHelmEvent{Kind: BundledHelmDownloadStart, Version: bundledHelmVersion})
	var retErr error
	defer func() {
		errMsg := ""
		if retErr != nil {
			errMsg = retErr.Error()
		}
		emitBundledHelmEvent(BundledHelmEvent{Kind: BundledHelmDownloadDone, Version: bundledHelmVersion, Err: errMsg})
	}()

	archiveName, err := helmArchiveName(bundledHelmVersion, goos, goarch)
	if err != nil {
		retErr = err
		return err
	}
	archiveURL := strings.TrimRight(getHelmBaseURL, "/") + "/" + archiveName

	expected, err := fetchExpectedArchiveSHA256(ctx, archiveURL, archiveName)
	if err != nil {
		retErr = err
		return err
	}
	archiveBytes, err := downloadAndVerify(ctx, archiveURL, expected)
	if err != nil {
		retErr = err
		return err
	}

	binary, err := extractHelmBinary(archiveBytes, goos)
	if err != nil {
		retErr = err
		return err
	}

	if err := atomicWriteFile(dest, binary, goos != "windows"); err != nil {
		retErr = err
		return err
	}

	if ok, verr := helmBinaryIsVersion(ctx, dest, bundledHelmVersion); !ok {
		if verr == nil {
			verr = errors.New("version mismatch")
		}
		err := fmt.Errorf("installed bundled helm but version check failed: %w", verr)
		retErr = err
		return err
	}
	return nil
}

func helmBinaryIsVersion(ctx context.Context, helmPath, want string) (bool, error) {
	st, err := os.Stat(helmPath)
	if err != nil {
		return false, err
	}
	if st.IsDir() {
		return false, fmt.Errorf("%s is a directory", helmPath)
	}
	ctx2, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx2, helmPath, "version", "--short")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("run helm version: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	s := strings.TrimSpace(string(out))
	return strings.Contains(s, want), nil
}

func helmArchiveName(version, goos, goarch string) (string, error) {
	// Helm release artifacts are named using GOOS/GOARCH values (e.g. linux-amd64).
	// Windows uses zip, others use tar.gz.
	ext := "tar.gz"
	if goos == "windows" {
		ext = "zip"
	}
	if strings.TrimSpace(goos) == "" || strings.TrimSpace(goarch) == "" {
		return "", fmt.Errorf("invalid platform: %q/%q", goos, goarch)
	}
	return fmt.Sprintf("helm-%s-%s-%s.%s", version, goos, goarch, ext), nil
}

func fetchExpectedArchiveSHA256(ctx context.Context, archiveURL, archiveName string) (string, error) {
	// Helm releases often provide per-archive checksum files:
	// - <archive>.sha256sum (format: "<sha>  <filename>")
	// - <archive>.sha256    (format varies: "<sha>" or "<sha>  <filename>")
	//
	// Some versions historically shipped a combined checksums file, but it's not
	// consistently present.
	for _, u := range []string{archiveURL + ".sha256sum", archiveURL + ".sha256"} {
		b, err := downloadBytes(ctx, u)
		if err != nil {
			continue
		}
		sum, err := parseSHA256File(string(b), archiveName)
		if err != nil {
			continue
		}
		if sum != "" {
			return sum, nil
		}
	}

	// Fallback: try combined checksums file if it exists.
	checksumsName := fmt.Sprintf("helm-%s-checksums.txt", bundledHelmVersion)
	checksumsURL := strings.TrimRight(getHelmBaseURL, "/") + "/" + checksumsName
	if b, err := downloadBytes(ctx, checksumsURL); err == nil {
		m := parseChecksumsTxt(string(b))
		sum := strings.TrimSpace(m[archiveName])
		if sum != "" {
			return sum, nil
		}
	}

	return "", fmt.Errorf("could not find sha256 for %s (tried .sha256sum/.sha256 and %s)", archiveName, checksumsURL)
}

func parseSHA256File(s, wantFilename string) (string, error) {
	// Accept:
	// - "<sha>" (single token)
	// - "<sha>  <filename>" (sha256sum format)
	line := strings.TrimSpace(strings.SplitN(s, "\n", 2)[0])
	if line == "" {
		return "", errors.New("empty sha file")
	}
	fs := strings.Fields(line)
	if len(fs) == 1 {
		return strings.TrimSpace(fs[0]), nil
	}
	if len(fs) >= 2 {
		sha := strings.TrimSpace(fs[0])
		name := strings.TrimSpace(fs[len(fs)-1])
		if wantFilename != "" && name != wantFilename {
			return "", fmt.Errorf("sha file refers to %q, want %q", name, wantFilename)
		}
		return sha, nil
	}
	return "", errors.New("unrecognized sha file format")
}

func parseChecksumsTxt(s string) map[string]string {
	// Format: "<hex>  <filename>" (whitespace-separated)
	out := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fs := strings.Fields(line)
		if len(fs) < 2 {
			continue
		}
		sha := strings.TrimSpace(fs[0])
		name := strings.TrimSpace(fs[len(fs)-1])
		if sha == "" || name == "" {
			continue
		}
		out[name] = sha
	}
	return out
}

func downloadAndVerify(ctx context.Context, url, expectedSHA256Hex string) ([]byte, error) {
	b, err := downloadBytes(ctx, url)
	if err != nil {
		return nil, err
	}
	h := sha256.Sum256(b)
	got := hex.EncodeToString(h[:])
	exp := strings.ToLower(strings.TrimSpace(expectedSHA256Hex))
	if exp == "" {
		return nil, errors.New("empty expected sha256")
	}
	if got != exp {
		return nil, fmt.Errorf("sha256 mismatch for %s: expected %s, got %s", url, exp, got)
	}
	return b, nil
}

func downloadBytes(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck // response body close
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		return nil, fmt.Errorf("download %s: http %s (%s)", url, resp.Status, strings.TrimSpace(string(b)))
	}
	return io.ReadAll(resp.Body)
}

func extractHelmBinary(archive []byte, goos string) ([]byte, error) {
	if goos == "windows" {
		return extractHelmFromZip(archive)
	}
	return extractHelmFromTarGz(archive)
}

func extractHelmFromTarGz(archive []byte) ([]byte, error) {
	r := bytes.NewReader(archive)
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	defer gz.Close() //nolint:errcheck // gzip reader close

	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		name := strings.TrimSpace(h.Name)
		if h.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(name) != "helm" {
			continue
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			return nil, err
		}
		if len(b) == 0 {
			return nil, errors.New("extracted helm binary is empty")
		}
		return b, nil
	}
	return nil, errors.New("helm binary not found in tar.gz")
}

func extractHelmFromZip(archive []byte) ([]byte, error) {
	r := bytes.NewReader(archive)
	zr, err := zip.NewReader(r, int64(len(archive)))
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		name := strings.TrimSpace(f.Name)
		if filepath.Base(name) != "helm.exe" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return nil, err
		}
		if len(b) == 0 {
			return nil, errors.New("extracted helm.exe is empty")
		}
		return b, nil
	}
	return nil, errors.New("helm.exe not found in zip")
}

func atomicWriteFile(dest string, content []byte, chmodExec bool) error {
	dir := filepath.Dir(dest)
	tmp, err := os.CreateTemp(dir, ".tmp-helm-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()

	if _, err := tmp.Write(content); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if chmodExec {
		_ = os.Chmod(tmpName, 0o755)
	}

	// Windows: os.Rename does not reliably replace existing files.
	_ = os.Remove(dest)
	if err := os.Rename(tmpName, dest); err != nil {
		return err
	}
	return nil
}

func acquireDirLock(ctx context.Context, lockDir string, staleAfter time.Duration) (func(), error) {
	// Cross-platform lock: os.Mkdir is atomic.
	deadline := time.Now().Add(30 * time.Second)
	for {
		err := os.Mkdir(lockDir, 0o755)
		if err == nil {
			// write metadata for staleness checks (best-effort)
			_ = os.WriteFile(filepath.Join(lockDir, "meta"), []byte(time.Now().UTC().Format(time.RFC3339)), 0o644)
			return func() { _ = os.RemoveAll(lockDir) }, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}
		// Lock exists: clear if stale.
		if staleAfter > 0 {
			if st, statErr := os.Stat(lockDir); statErr == nil {
				if time.Since(st.ModTime()) > staleAfter {
					_ = os.RemoveAll(lockDir)
					continue
				}
			}
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timeout waiting for lock %s", lockDir)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(120 * time.Millisecond):
		}
	}
}
