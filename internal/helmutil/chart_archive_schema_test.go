package helmutil

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestReadChartArchiveFilesWithSchema_PrefersTopLevel(t *testing.T) {
	tmp := t.TempDir()
	tgz := filepath.Join(tmp, "chart.tgz")

	f, err := os.Create(tgz)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close() //nolint:errcheck // test file close

	gz := gzip.NewWriter(f)
	defer gz.Close() //nolint:errcheck // test gzip close
	tr := tar.NewWriter(gz)
	defer tr.Close() //nolint:errcheck // test tar close

	write := func(name, content string) {
		b := []byte(content)
		if err := tr.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(b))}); err != nil {
			t.Fatalf("header %s: %v", name, err)
		}
		if _, err := tr.Write(b); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	// Fallbacks (subchart paths) should not override.
	write("mychart/charts/sub/values.yaml", "sub: true\n")
	write("mychart/charts/sub/values.schema.json", "{\"title\":\"sub\"}")
	write("mychart/charts/sub/README.md", "sub readme")

	// Preferred top-level.
	write("mychart/values.yaml", "top: true\n")
	write("mychart/values.schema.json", "{\"title\":\"top\"}")
	write("mychart/README.md", "top readme")

	if err := tr.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gz close: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("file close: %v", err)
	}

	readme, values, schema, err := ReadChartArchiveFilesWithSchema(tgz)
	if err != nil {
		t.Fatalf("ReadChartArchiveFilesWithSchema: %v", err)
	}
	if readme != "top readme" {
		t.Fatalf("unexpected readme: %q", readme)
	}
	if values != "top: true\n" {
		t.Fatalf("unexpected values: %q", values)
	}
	if schema != "{\"title\":\"top\"}" {
		t.Fatalf("unexpected schema: %q", schema)
	}
}
