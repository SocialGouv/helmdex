package yamlchart

import (
	"fmt"
)

type DepID string

// DependencyID returns the stable identifier used for values keying.
// Contract: alias if set, else name.
func DependencyID(d Dependency) DepID {
	if d.Alias != "" {
		return DepID(d.Alias)
	}
	return DepID(d.Name)
}

func (c *Chart) UpsertDependency(dep Dependency) error {
	if dep.Name == "" {
		return fmt.Errorf("dependency name is required")
	}
	if dep.Version == "" {
		return fmt.Errorf("dependency version is required")
	}
	if dep.Repository == "" {
		return fmt.Errorf("dependency repository is required")
	}

	// Uniqueness of id.
	id := DependencyID(dep)
	for _, existing := range c.Dependencies {
		if DependencyID(existing) == id && existing.Name != dep.Name {
			return fmt.Errorf("dependency id %q already used", id)
		}
	}

	for i := range c.Dependencies {
		if c.Dependencies[i].Name == dep.Name {
			c.Dependencies[i] = dep
			return nil
		}
	}
	// Add.
	c.Dependencies = append(c.Dependencies, dep)
	return nil
}

func (c *Chart) RemoveDependencyByID(id DepID) bool {
	for i := range c.Dependencies {
		if DependencyID(c.Dependencies[i]) == id {
			c.Dependencies = append(c.Dependencies[:i], c.Dependencies[i+1:]...)
			return true
		}
	}
	return false
}
