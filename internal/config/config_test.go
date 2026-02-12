package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFile_AppliesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "helmdex.yaml")
	data := "" +
		"apiVersion: helmdex.io/v1alpha1\n" +
		"kind: HelmdexConfig\n" +
		"repo: {}\n" +
		"platform: {name: \"\"}\n" +
		"sources: []\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if cfg.Repo.AppsDir != "apps" {
		t.Fatalf("appsDir default not applied: %q", cfg.Repo.AppsDir)
	}
	if !cfg.ArtifactHubEnabled() {
		t.Fatalf("artifactHub default not applied")
	}
}

func TestLoadFile_RequiresPlatformWhenPresetsEnabled(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "helmdex.yaml")
	data := "" +
		"apiVersion: helmdex.io/v1alpha1\n" +
		"kind: HelmdexConfig\n" +
		"repo: {appsDir: apps}\n" +
		"platform: {name: \"\"}\n" +
		"sources:\n" +
		"  - name: s1\n" +
		"    git: {url: https://example.invalid/repo.git}\n" +
		"    presets: {enabled: true}\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := LoadFile(path); err == nil {
		t.Fatalf("expected error")
	}
}

