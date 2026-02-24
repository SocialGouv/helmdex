package catalog

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"helmdex/internal/config"
)

func TestSyncSourceToCache_FilesystemSourceCopiesDir(t *testing.T) {
	tmp := t.TempDir()
	srcDir := filepath.Join(tmp, "src")
	destDir := filepath.Join(tmp, "dest")

	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("mkdir srcDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "catalog.yaml"), []byte("hello: world\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	src := config.Source{Name: "s1", Git: config.GitRef{URL: srcDir}}
	res, err := syncSourceToCache(context.Background(), src, destDir)
	if err != nil {
		t.Fatalf("syncSourceToCache: %v", err)
	}
	if res.ResolvedCommit == "" {
		t.Fatalf("expected resolved commit to be set")
	}
	if _, err := os.Stat(filepath.Join(destDir, "catalog.yaml")); err != nil {
		t.Fatalf("expected file to be copied: %v", err)
	}
}

