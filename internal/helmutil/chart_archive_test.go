package helmutil

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestReadChartArchiveFiles(t *testing.T) {
	tmp := t.TempDir()
	tgzPath := filepath.Join(tmp, "chart-1.0.0.tgz")

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tr := tar.NewWriter(gz)

	// Typical helm chart archive layout: <chartname>/...
	write := func(name, content string) {
		b := []byte(content)
		if err := tr.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(b))}); err != nil {
			t.Fatalf("header: %v", err)
		}
		if _, err := tr.Write(b); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	write("mychart/README.md", "# Hello\n")
	write("mychart/values.yaml", "replicaCount: 1\n")

	if err := tr.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	if err := os.WriteFile(tgzPath, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write tgz: %v", err)
	}

	r, v, err := ReadChartArchiveFiles(tgzPath)
	if err != nil {
		t.Fatalf("ReadChartArchiveFiles: %v", err)
	}
	if r == "" || v == "" {
		t.Fatalf("expected readme and values, got readme=%q values=%q", r, v)
	}
}
