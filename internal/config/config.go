package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	APIVersion = "helmdex.io/v1alpha1"
	Kind       = "HelmdexConfig"
)

type Config struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`

	Repo     RepoConfig     `yaml:"repo"`
	Platform PlatformConfig `yaml:"platform"`
	Sources  []Source       `yaml:"sources"`

	ArtifactHub ArtifactHubConfig `yaml:"artifactHub"`
}

type RepoConfig struct {
	AppsDir string `yaml:"appsDir"`
}

type PlatformConfig struct {
	Name string `yaml:"name"`
}

type Source struct {
	Name string `yaml:"name"`
	Git  GitRef `yaml:"git"`

	Presets PresetsConfig `yaml:"presets"`
	Catalog CatalogConfig `yaml:"catalog"`
}

type GitRef struct {
	URL    string `yaml:"url"`
	Ref    string `yaml:"ref,omitempty"`
	Commit string `yaml:"commit,omitempty"`
}

type PresetsConfig struct {
	Enabled    bool   `yaml:"enabled"`
	ChartsPath string `yaml:"chartsPath,omitempty"`
}

type CatalogConfig struct {
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path,omitempty"`
}

type ArtifactHubConfig struct {
	// Enabled defaults to true when omitted.
	Enabled *bool `yaml:"enabled,omitempty"`
}

func DefaultConfig() Config {
	return Config{
		APIVersion: APIVersion,
		Kind:       Kind,
		Repo: RepoConfig{
			AppsDir: "apps",
		},
		Platform: PlatformConfig{
			Name: "",
		},
		Sources: []Source{},
		ArtifactHub: ArtifactHubConfig{
			Enabled: boolPtr(true),
		},
	}
}

func LoadFile(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return Config{}, err
	}

	applyDefaults(&cfg)

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func WriteFile(path string, cfg Config) error {
	if err := cfg.ValidateForWrite(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	out, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

func (c Config) Validate() error {
	if c.APIVersion != APIVersion {
		return fmt.Errorf("unsupported apiVersion %q", c.APIVersion)
	}
	if c.Kind != Kind {
		return fmt.Errorf("unsupported kind %q", c.Kind)
	}
	seen := map[string]struct{}{}
	for _, s := range c.Sources {
		if s.Name == "" {
			return fmt.Errorf("sources[].name is required")
		}
		if _, ok := seen[s.Name]; ok {
			return fmt.Errorf("duplicate sources[].name %q", s.Name)
		}
		seen[s.Name] = struct{}{}
		if s.Git.URL == "" {
			return fmt.Errorf("sources[%s].git.url is required", s.Name)
		}
		if s.Presets.Enabled && c.Platform.Name == "" {
			return fmt.Errorf("platform.name is required when source %q presets are enabled", s.Name)
		}
	}
	return nil
}

func (c Config) ValidateForWrite() error {
	if c.APIVersion == "" {
		c.APIVersion = APIVersion
	}
	if c.Kind == "" {
		c.Kind = Kind
	}
	if c.Repo.AppsDir == "" {
		c.Repo.AppsDir = "apps"
	}
	// For writing a default config, we don't enforce platform.name.
	if c.APIVersion != APIVersion || c.Kind != Kind {
		return fmt.Errorf("invalid config header")
	}
	return nil
}

func (c Config) ArtifactHubEnabled() bool {
	if c.ArtifactHub.Enabled == nil {
		return true
	}
	return *c.ArtifactHub.Enabled
}

func applyDefaults(cfg *Config) {
	if cfg.Repo.AppsDir == "" {
		cfg.Repo.AppsDir = "apps"
	}
	if cfg.ArtifactHub.Enabled == nil {
		cfg.ArtifactHub.Enabled = boolPtr(true)
	}
	for i := range cfg.Sources {
		s := &cfg.Sources[i]
		if s.Presets.Enabled && s.Presets.ChartsPath == "" {
			s.Presets.ChartsPath = "charts"
		}
		if s.Catalog.Enabled && s.Catalog.Path == "" {
			s.Catalog.Path = "catalog.yaml"
		}
	}
}

func boolPtr(v bool) *bool { return &v }
