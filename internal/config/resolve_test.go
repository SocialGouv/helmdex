package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolve_RepoConfigWins(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("HELMDEX_USER_CONFIG", filepath.Join(t.TempDir(), "config.yaml"))
	if err := os.WriteFile(filepath.Join(repo, "helmdex.yaml"),
		[]byte("apiVersion: helmdex.io/v1alpha1\nkind: HelmdexConfig\nrepo:\n  appsDir: envs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Resolve(repo, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != SourceRepo || res.Config.Repo.AppsDir != "envs" {
		t.Fatalf("res = %+v", res)
	}
}

func TestResolve_UserConfigWithPerRepoOverride(t *testing.T) {
	repo := t.TempDir()
	userCfg := filepath.Join(t.TempDir(), "config.yaml")
	t.Setenv("HELMDEX_USER_CONFIG", userCfg)

	content := "platform:\n  name: atlas\nrepos:\n  " + repo + ":\n    appsDir: envs\n    templatesDir: blueprints\n"
	if err := os.WriteFile(userCfg, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Resolve(repo, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != SourceUser {
		t.Fatalf("source = %q", res.Source)
	}
	if res.Config.Platform.Name != "atlas" {
		t.Fatalf("platform = %q", res.Config.Platform.Name)
	}
	if res.Config.Repo.AppsDir != "envs" || res.Config.Repo.TemplatesDir != "blueprints" {
		t.Fatalf("repo overrides not applied: %+v", res.Config.Repo)
	}

	// Another repo gets the user defaults, not the override.
	other := t.TempDir()
	res2, err := Resolve(other, "")
	if err != nil {
		t.Fatal(err)
	}
	if res2.Config.Repo.AppsDir != "apps" || res2.Config.Repo.TemplatesDir != "" {
		t.Fatalf("unexpected override leak: %+v", res2.Config.Repo)
	}
}

func TestResolve_DefaultsWhenNothingExists(t *testing.T) {
	t.Setenv("HELMDEX_USER_CONFIG", filepath.Join(t.TempDir(), "config.yaml"))
	res, err := Resolve(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != SourceDefault || res.Path != "" {
		t.Fatalf("res = %+v", res)
	}
	if res.Config.Repo.AppsDir != "apps" {
		t.Fatalf("defaults not applied: %+v", res.Config.Repo)
	}
}

func TestSave_UserConfigPreservesRepoOverrides(t *testing.T) {
	userCfg := filepath.Join(t.TempDir(), "config.yaml")
	t.Setenv("HELMDEX_USER_CONFIG", userCfg)
	if err := os.WriteFile(userCfg, []byte("repos:\n  /some/repo:\n    appsDir: envs\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig()
	cfg.Platform.Name = "atlas"
	path, err := Save(Resolved{Path: userCfg, Source: SourceUser}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if path != userCfg {
		t.Fatalf("saved to %q", path)
	}

	res, err := Resolve("/some/repo", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Config.Platform.Name != "atlas" {
		t.Fatalf("platform lost: %+v", res.Config)
	}
	if res.Config.Repo.AppsDir != "envs" {
		t.Fatalf("per-repo override lost: %+v", res.Config.Repo)
	}
}

func TestSave_DefaultSourceCreatesUserConfig(t *testing.T) {
	userCfg := filepath.Join(t.TempDir(), "nested", "config.yaml")
	t.Setenv("HELMDEX_USER_CONFIG", userCfg)

	cfg := DefaultConfig()
	cfg.Platform.Name = "atlas"
	path, err := Save(Resolved{Source: SourceDefault}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if path != userCfg {
		t.Fatalf("saved to %q, want %q", path, userCfg)
	}
	res, err := Resolve(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != SourceUser || res.Config.Platform.Name != "atlas" {
		t.Fatalf("res = %+v", res)
	}
}
