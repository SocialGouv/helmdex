package catalog

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

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
		dest := filepath.Join(cacheRoot, src.Name)
		res, err := gitutil.CloneOrUpdate(ctx, gitutil.CloneOrUpdateParams{
			URL:     src.Git.URL,
			Ref:     src.Git.Ref,
			Commit:  src.Git.Commit,
			DestDir: dest,
		})
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

