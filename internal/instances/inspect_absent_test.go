package instances

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"helmdex/internal/helmutil"
	"helmdex/internal/yamlchart"
)

// writeChartArchive builds a minimal chart .tgz at dest containing only the
// given top-level files under <chartName>/.
func writeChartArchive(t *testing.T, dest, chartName string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		hdr := &tar.Header{Name: chartName + "/" + name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A chart archive that ships values.yaml but no README/schema must yield the
// values content and a definitive "absent" error (not a fetch failure) for the
// missing artifacts — the tab is genuinely empty, not broken.
func TestLoadDepInspectContent_ArtifactAbsent(t *testing.T) {
	repoRoot := t.TempDir() // agnostic repo (no helmdex.yaml)
	t.Setenv("HELMDEX_CACHE_DIR", filepath.Join(t.TempDir(), "cache"))

	const name, version = "da-manager", "0.0.0-sha.abc"
	repoURL := "oci://registry.example.test/org/" + name

	// Seed the isolated per-URL env cache so FindCachedChartArchive hits it
	// and the network path is never taken.
	env := helmutil.EnvForRepoURL(repoRoot, repoURL)
	tgz := filepath.Join(env.CacheHome, "repository", name+"-"+version+".tgz")
	writeChartArchive(t, tgz, name, map[string]string{
		"Chart.yaml":  "apiVersion: v2\nname: " + name + "\nversion: " + version + "\n",
		"values.yaml": "replicaCount: 1\n",
	})

	dep := yamlchart.Dependency{Name: name, Version: version, Repository: repoURL}

	values, err := LoadDepInspectContent(context.Background(), repoRoot, t.TempDir(), dep, InspectValues)
	if err != nil || values == "" {
		t.Fatalf("values must load from the archive: %q err=%v", values, err)
	}

	for _, kind := range []InspectKind{InspectReadme, InspectSchema} {
		_, err := LoadDepInspectContent(context.Background(), repoRoot, t.TempDir(), dep, kind)
		if !errors.Is(err, ErrArtifactAbsent) {
			t.Fatalf("inspect %s: want ErrArtifactAbsent, got %v", kind, err)
		}
	}
}
