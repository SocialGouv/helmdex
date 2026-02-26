package yamlchart

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Chart struct {
	APIVersion   string       `yaml:"apiVersion"`
	Name         string       `yaml:"name"`
	Version      string       `yaml:"version"`
	Dependencies []Dependency `yaml:"dependencies,omitempty"`
}

type Dependency struct {
	Name       string `yaml:"name"`
	Version    string `yaml:"version"`
	Repository string `yaml:"repository"`
	Alias      string `yaml:"alias,omitempty"`
}

func NewUmbrellaChart(instanceName string) Chart {
	return Chart{
		APIVersion:   "v2",
		Name:         instanceName,
		Version:      "0.1.0",
		Dependencies: []Dependency{},
	}
}

func WriteChart(path string, c Chart) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	buf, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, buf, 0o644)
}
