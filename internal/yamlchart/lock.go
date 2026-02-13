package yamlchart

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Lock represents Helm's Chart.lock file.
// We only model the subset needed to decide whether dependencies are in-sync.
//
// Helm's schema (v2 charts) typically includes fields like:
// apiVersion, generated, digest, and dependencies[].
type Lock struct {
	APIVersion    string           `yaml:"apiVersion"`
	Generated     string           `yaml:"generated,omitempty"`
	Digest        string           `yaml:"digest,omitempty"`
	Dependencies  []LockDependency  `yaml:"dependencies,omitempty"`
}

type LockDependency struct {
	Name       string `yaml:"name"`
	Version    string `yaml:"version"`
	Repository string `yaml:"repository"`
	Digest     string `yaml:"digest,omitempty"`
}

func ReadLock(path string) (Lock, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Lock{}, err
	}
	var l Lock
	if err := yaml.Unmarshal(b, &l); err != nil {
		return Lock{}, err
	}
	return l, nil
}

