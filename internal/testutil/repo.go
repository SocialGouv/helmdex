package testutil

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// SourceMode selects how a test repo is wired to the fixture chart source.
type SourceMode int

const (
	// SourceGit publishes fixtures/remote-source as a local git repository —
	// the production shape, exercising gitutil clone/fetch.
	SourceGit SourceMode = iota
	// SourceDir points the source straight at a directory, exercising the
	// filesystem-source path in catalog.Sync.
	SourceDir
	// SourceNone configures no source at all.
	SourceNone
)

// RepoOpts configures NewRepo.
type RepoOpts struct {
	// Agnostic seeds the repo from fixtures/agnostic-gitops and writes no
	// helmdex.yaml, so helmdex must adapt to a repo that never opted in.
	// Sources are not configured in this mode.
	Agnostic bool

	// SourceMode selects how the fixture chart source is exposed. Ignored when
	// Agnostic is set.
	SourceMode SourceMode

	// Platform names the presets layer to import (values.platform.<name>.yaml).
	// Defaults to "eks", which the fixtures provide.
	Platform string

	// ArtifactHub enables the Artifact Hub source. Off by default: it would
	// reach the network.
	ArtifactHub bool

	// Seed runs once the repo is on disk, before anything reads it. Use it for
	// fixtures that must exist before layout discovery, which helmdex performs
	// at startup (a templates/ dir created later stays invisible).
	Seed func(t *testing.T, r Repo)
}

// Repo is a throwaway helmdex workspace.
type Repo struct {
	// Root is the repo directory passed to helmdex as --repo.
	Root string
	// ConfigPath is the helmdex.yaml path, empty for agnostic repos.
	ConfigPath string
	// Source is the git remote directory or fixture directory backing the
	// configured source, empty when SourceNone or Agnostic.
	Source string
	// AppsDir is the instances directory, relative to Root.
	AppsDir string
}

// NewRepo creates a hermetic helmdex workspace under t.TempDir().
//
// It does not call Hermetic: tests that run Helm-dependent flows must call
// Hermetic first so the fake Helm and isolated home are in place.
func NewRepo(t *testing.T, opts RepoOpts) Repo {
	t.Helper()

	root := t.TempDir()

	if opts.Agnostic {
		CopyDir(t, FixturePath(t, "agnostic-gitops"), root)
		repo := Repo{Root: root, AppsDir: "apps"}
		if opts.Seed != nil {
			opts.Seed(t, repo)
		}
		return repo
	}

	if err := os.MkdirAll(filepath.Join(root, "apps"), 0o755); err != nil {
		t.Fatalf("create apps dir: %v", err)
	}

	platform := opts.Platform
	if platform == "" {
		platform = "eks"
	}

	repo := Repo{Root: root, ConfigPath: filepath.Join(root, "helmdex.yaml"), AppsDir: "apps"}

	switch opts.SourceMode {
	case SourceNone:
	case SourceDir:
		repo.Source = FixturePath(t, "remote-source")
	case SourceGit:
		remote := t.TempDir()
		CopyDir(t, FixturePath(t, "remote-source"), remote)
		GitInit(t, remote)
		GitCommitAll(t, remote, "fixture source")
		repo.Source = remote
	default:
		t.Fatalf("unknown SourceMode %d", opts.SourceMode)
	}

	writeConfig(t, repo, platform, opts.ArtifactHub)
	if opts.Seed != nil {
		opts.Seed(t, repo)
	}
	return repo
}

func writeConfig(t *testing.T, repo Repo, platform string, artifactHub bool) {
	t.Helper()

	lines := []string{
		"apiVersion: helmdex.io/v1alpha1",
		"kind: HelmdexConfig",
		"repo:",
		"  appsDir: " + repo.AppsDir,
		"platform:",
		"  name: " + platform,
	}
	if repo.Source == "" {
		lines = append(lines, "sources: []")
	} else {
		lines = append(lines,
			"sources:",
			"  - name: Example",
			"    git:",
			"      url: "+repo.Source,
			"    presets:",
			"      enabled: true",
			"      chartsPath: charts",
			"    catalog:",
			"      enabled: true",
			"      path: catalog.yaml",
		)
	}
	lines = append(lines, "artifactHub:", fmt.Sprintf("  enabled: %t", artifactHub), "")

	if err := os.WriteFile(repo.ConfigPath, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatalf("write %s: %v", repo.ConfigPath, err)
	}
}

// Path resolves a path inside the repo.
func (r Repo) Path(parts ...string) string {
	return filepath.Join(append([]string{r.Root}, parts...)...)
}

// InstancePath resolves an instance directory inside the repo.
func (r Repo) InstancePath(name string) string {
	return filepath.Join(r.Root, r.AppsDir, name)
}

// Read returns the contents of a repo-relative file, failing the test when it
// is missing.
func (r Repo) Read(t *testing.T, parts ...string) string {
	t.Helper()
	p := r.Path(parts...)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}

// Exists reports whether a repo-relative path exists.
func (r Repo) Exists(t *testing.T, parts ...string) bool {
	t.Helper()
	p := r.Path(parts...)
	_, err := os.Stat(p)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	t.Fatalf("stat %s: %v", p, err)
	return false
}

// Write creates or replaces a repo-relative file, creating parent directories.
func (r Repo) Write(t *testing.T, content string, parts ...string) {
	t.Helper()
	p := r.Path(parts...)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(p), err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
}

// CopyDir recursively copies src into dst, preserving the executable bit.
func CopyDir(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, b, info.Mode().Perm())
	})
	if err != nil {
		t.Fatalf("copy %s -> %s: %v", src, dst, err)
	}
}

// GitInit initialises a git repository with a committer identity, so commits
// work under the isolated HOME set by Hermetic.
func GitInit(t *testing.T, dir string) {
	t.Helper()
	Git(t, dir, "init", "-b", "main")
	Git(t, dir, "config", "user.email", "e2e@example.invalid")
	Git(t, dir, "config", "user.name", "helmdex-e2e")
}

// GitCommitAll stages everything in dir and commits it.
func GitCommitAll(t *testing.T, dir, msg string) {
	t.Helper()
	Git(t, dir, "add", "-A")
	Git(t, dir, "commit", "-m", msg)
}

// Git runs a git command in dir and fails the test on error.
func Git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s failed: %v\n%s", args, dir, err, b)
	}
}
