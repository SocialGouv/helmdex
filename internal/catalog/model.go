package catalog

// v0.2 minimal catalog model.

type Catalog struct {
	APIVersion string  `yaml:"apiVersion"`
	Kind       string  `yaml:"kind"`
	Entries    []Entry `yaml:"entries"`
}

type Entry struct {
	ID          string   `yaml:"id"`
	Description string   `yaml:"description,omitempty"`
	Chart       Chart    `yaml:"chart"`
	Version     string   `yaml:"version"`
	Digest      string   `yaml:"digest,omitempty"`
	DefaultSets []string `yaml:"defaultSets,omitempty"`
}

type Chart struct {
	Repo string `yaml:"repo"`
	Name string `yaml:"name"`
}
