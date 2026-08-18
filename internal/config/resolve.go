package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ConfigSource identifies where a resolved config came from.
type ConfigSource string

const (
	SourceFlag    ConfigSource = "flag"
	SourceRepo    ConfigSource = "repo"
	SourceUser    ConfigSource = "user"
	SourceDefault ConfigSource = "default"
)

// Resolved is the outcome of the config resolution chain.
type Resolved struct {
	Config Config
	// Path is the config file actually loaded ("" when built-in defaults).
	Path   string
	Source ConfigSource
}

// UserConfig is the schema of the user-level config file
// (~/.config/helmdex/config.yaml). It embeds the regular Config (global
// sources, platform, artifactHub…) plus per-repo overrides keyed by the
// repo's absolute path. This lets helmdex work on repos that stay fully
// helmdex-agnostic (no helmdex.yaml committed).
type UserConfig struct {
	Config `yaml:",inline"`

	// Repos maps an absolute repo path to repo-section overrides.
	Repos map[string]RepoConfig `yaml:"repos,omitempty"`
}

// UserConfigPath returns the user-level config file location.
// Overridable with HELMDEX_USER_CONFIG (used by tests).
func UserConfigPath() (string, error) {
	if v := os.Getenv("HELMDEX_USER_CONFIG"); v != "" {
		return v, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(base, "helmdex", "config.yaml"), nil
}

// Resolve implements the config resolution chain:
//  1. explicitPath (--config): must load, hard error otherwise
//  2. <repoRoot>/helmdex.yaml when present
//  3. user config (~/.config/helmdex/config.yaml) when present,
//     with per-repo overrides applied for repoRoot
//  4. built-in defaults
func Resolve(repoRoot, explicitPath string) (Resolved, error) {
	if explicitPath != "" {
		cfg, err := LoadFile(explicitPath)
		if err != nil {
			return Resolved{}, err
		}
		return Resolved{Config: cfg, Path: explicitPath, Source: SourceFlag}, nil
	}

	repoCfgPath := filepath.Join(repoRoot, "helmdex.yaml")
	if _, err := os.Stat(repoCfgPath); err == nil {
		cfg, err := LoadFile(repoCfgPath)
		if err != nil {
			return Resolved{}, err
		}
		return Resolved{Config: cfg, Path: repoCfgPath, Source: SourceRepo}, nil
	}

	userPath, err := UserConfigPath()
	if err == nil {
		if _, statErr := os.Stat(userPath); statErr == nil {
			cfg, err := loadUserFile(userPath, repoRoot)
			if err != nil {
				return Resolved{}, err
			}
			return Resolved{Config: cfg, Path: userPath, Source: SourceUser}, nil
		}
	}

	return Resolved{Config: DefaultConfig(), Source: SourceDefault}, nil
}

// Save persists cfg back to the location a Resolve produced. Repo/flag
// configs are written as-is; user/default resolutions are merged into the
// user config file, preserving its per-repo overrides. It returns the path
// actually written.
func Save(res Resolved, cfg Config) (string, error) {
	switch res.Source {
	case SourceFlag, SourceRepo:
		return res.Path, WriteFile(res.Path, cfg)
	case SourceUser, SourceDefault, "":
		path := res.Path
		if path == "" {
			p, err := UserConfigPath()
			if err != nil {
				return "", err
			}
			path = p
		}
		var uc UserConfig
		b, err := os.ReadFile(path)
		switch {
		case err == nil:
			if err := yaml.Unmarshal(b, &uc); err != nil {
				return "", fmt.Errorf("parse user config %s: %w", path, err)
			}
		case os.IsNotExist(err):
			// Fresh user config.
		default:
			return "", err
		}
		// Update only the global fields callers legitimately edit. The repo
		// section and per-repo overrides are preserved from disk: cfg's
		// Repo may carry a per-repo override applied during Resolve, and
		// writing it back would silently promote it to the global default
		// for every other repo.
		uc.APIVersion = cfg.APIVersion
		uc.Kind = cfg.Kind
		uc.Platform = cfg.Platform
		uc.Sources = cfg.Sources
		uc.ArtifactHub = cfg.ArtifactHub
		if err := uc.ValidateForWrite(); err != nil {
			return "", err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", err
		}
		out, err := yaml.Marshal(uc)
		if err != nil {
			return "", err
		}
		return path, os.WriteFile(path, out, 0o644)
	default:
		return "", fmt.Errorf("unknown config source %q", res.Source)
	}
}

func loadUserFile(path, repoRoot string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var uc UserConfig
	if err := yaml.Unmarshal(b, &uc); err != nil {
		return Config{}, fmt.Errorf("parse user config %s: %w", path, err)
	}
	cfg := uc.Config

	// Per-repo overrides, keyed by absolute (cleaned) repo path.
	abs, err := filepath.Abs(repoRoot)
	if err != nil {
		return Config{}, err
	}
	abs = filepath.Clean(abs)
	for key, over := range uc.Repos {
		if filepath.Clean(key) != abs {
			continue
		}
		if over.AppsDir != "" {
			cfg.Repo.AppsDir = over.AppsDir
		}
		if over.TemplatesDir != "" {
			cfg.Repo.TemplatesDir = over.TemplatesDir
		}
	}

	// The user config header is optional; default it before validation.
	if cfg.APIVersion == "" {
		cfg.APIVersion = APIVersion
	}
	if cfg.Kind == "" {
		cfg.Kind = Kind
	}
	applyDefaults(&cfg)
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("invalid user config %s: %w", path, err)
	}
	return cfg, nil
}
