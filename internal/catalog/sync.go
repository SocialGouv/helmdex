package catalog

import (
	"crypto/sha256"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"helmdex/internal/config"
	"helmdex/internal/gitutil"

	"gopkg.in/yaml.v3"
)

type Syncer struct {
	repoRoot string
}

type SyncResult struct {
	SourceName     string
	ResolvedCommit string
}

func NewSyncer(repoRoot string) *Syncer {
	return &Syncer{repoRoot: repoRoot}
}

func (s *Syncer) Sync(ctx context.Context, cfg config.Config) ([]SyncResult, error) {
	return s.SyncFiltered(ctx, cfg, func(config.Source) bool { return true })
}

// SyncFiltered syncs a subset of sources into `.helmdex/cache/<source>` and
// refreshes `.helmdex/catalog/<source>.yaml` for catalog-enabled sources.
//
// This is used by the TUI to implement per-dependency preset sync without
// necessarily syncing unrelated sources.
func (s *Syncer) SyncFiltered(ctx context.Context, cfg config.Config, include func(config.Source) bool) ([]SyncResult, error) {
	var out []SyncResult

	cacheRoot := filepath.Join(s.repoRoot, ".helmdex", "cache")
	if err := os.MkdirAll(cacheRoot, 0o755); err != nil {
		return nil, err
	}
	catRoot := filepath.Join(s.repoRoot, ".helmdex", "catalog")
	if err := os.MkdirAll(catRoot, 0o755); err != nil {
		return nil, err
	}

	for _, src := range cfg.Sources {
		if include != nil && !include(src) {
			continue
		}
		dest := filepath.Join(cacheRoot, src.Name)

		// Support local filesystem sources (non-git directories): copy into cache.
		// This keeps local testing friction low and avoids requiring `git init`.
		res, err := syncSourceToCache(ctx, src, dest)
		if err != nil {
			return nil, fmt.Errorf("sync source %q: %w", src.Name, err)
		}

		// Write meta with resolved commit.
		metaPath := filepath.Join(dest, ".helmdex-meta.yaml")
		meta := map[string]any{
			"resolvedCommit": res.ResolvedCommit,
			"url":            src.Git.URL,
			"ref":            src.Git.Ref,
			"commit":         src.Git.Commit,
		}
		b, _ := yaml.Marshal(meta)
		_ = os.WriteFile(metaPath, b, 0o644)

		if src.Catalog.Enabled {
			catPath := src.Catalog.Path
			if catPath == "" {
				catPath = "catalog.yaml"
			}
			srcCatalogPath := filepath.Join(dest, catPath)
			catalogBytes, err := os.ReadFile(srcCatalogPath)
			if err != nil {
				return nil, fmt.Errorf("read catalog for source %q: %w", src.Name, err)
			}
			target := filepath.Join(catRoot, src.Name+".yaml")
			if err := os.WriteFile(target, catalogBytes, 0o644); err != nil {
				return nil, err
			}
		}

		out = append(out, SyncResult{SourceName: src.Name, ResolvedCommit: res.ResolvedCommit})
	}

	return out, nil
}

func syncSourceToCache(ctx context.Context, src config.Source, dest string) (gitutil.CloneOrUpdateResult, error) {
	url := src.Git.URL
	if url == "" {
		return gitutil.CloneOrUpdateResult{}, fmt.Errorf("git url is required")
	}

	// Detect local directory without .git => filesystem source.
	if st, err := os.Stat(url); err == nil && st.IsDir() {
		if _, err := os.Stat(filepath.Join(url, ".git")); err != nil {
			resolved, err := copyDirToCache(url, dest)
			if err != nil {
				return gitutil.CloneOrUpdateResult{}, err
			}
			return gitutil.CloneOrUpdateResult{ResolvedCommit: resolved}, nil
		}
	}

	// Default: git clone/fetch/checkout.
	return gitutil.CloneOrUpdate(ctx, gitutil.CloneOrUpdateParams{
		URL:     url,
		Ref:     src.Git.Ref,
		Commit:  src.Git.Commit,
		DestDir: dest,
	})
}

func copyDirToCache(srcDir, destDir string) (resolved string, err error) {
	// Replace destDir so removed files disappear too.
	_ = os.RemoveAll(destDir)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", err
	}

	h := sha256.New()
	paths := []string{}
	err = filepath.WalkDir(srcDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		// Skip any nested .git dirs if the user happens to point at a git repo but
		// git detection failed for some reason.
		if d.IsDir() && rel == ".git" {
			return filepath.SkipDir
		}
		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(paths)

	for _, rel := range paths {
		srcPath := filepath.Join(srcDir, rel)
		dstPath := filepath.Join(destDir, rel)
		st, err := os.Stat(srcPath)
		if err != nil {
			return "", err
		}
		if st.IsDir() {
			if err := os.MkdirAll(dstPath, 0o755); err != nil {
				return "", err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			return "", err
		}
		srcF, err := os.Open(srcPath)
		if err != nil {
			return "", err
		}
		dstF, err := os.Create(dstPath)
		if err != nil {
			_ = srcF.Close()
			return "", err
		}
		if _, err := io.Copy(io.MultiWriter(dstF, h), srcF); err != nil {
			_ = dstF.Close()
			_ = srcF.Close()
			return "", err
		}
		// Close files promptly to avoid accumulating defers in large dirs.
		if err := dstF.Close(); err != nil {
			_ = srcF.Close()
			return "", err
		}
		if err := srcF.Close(); err != nil {
			return "", err
		}
		_, _ = h.Write([]byte("\n" + rel + "\n"))
	}

	return "fs-" + hex.EncodeToString(h.Sum(nil))[:12], nil
}
