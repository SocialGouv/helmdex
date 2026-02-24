package yamlchart

import (
	"fmt"
	"strings"
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

	// Upsert key is the stable dep id (alias if set, else name).
	//
	// This allows depending on the same chart multiple times by using distinct
	// aliases (Helm supports this), while keeping values keying stable.
	id := DependencyID(dep)
	for i := range c.Dependencies {
		if DependencyID(c.Dependencies[i]) == id {
			c.Dependencies[i] = dep
			return nil
		}
	}
	// Add (ensure no duplicate id exists).
	for _, existing := range c.Dependencies {
		if DependencyID(existing) == id {
			return fmt.Errorf("dependency id %q already used", id)
		}
	}
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

// ReplaceDependencyByID replaces an existing dependency identified by oldID with dep.
//
// This is needed when changing a dependency's id (alias), because UpsertDependency
// keys by the *new* id and would otherwise add a second entry.
func (c *Chart) ReplaceDependencyByID(oldID DepID, dep Dependency) error {
	if strings.TrimSpace(string(oldID)) == "" {
		return fmt.Errorf("old dependency id is required")
	}
	if dep.Name == "" {
		return fmt.Errorf("dependency name is required")
	}
	if dep.Version == "" {
		return fmt.Errorf("dependency version is required")
	}
	if dep.Repository == "" {
		return fmt.Errorf("dependency repository is required")
	}
	newID := DependencyID(dep)
	// Ensure old exists; record index.
	idx := -1
	for i := range c.Dependencies {
		if DependencyID(c.Dependencies[i]) == oldID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("dependency %q not found", oldID)
	}
	// If id changes, ensure no other dep already uses the new id.
	if newID != oldID {
		for i := range c.Dependencies {
			if i == idx {
				continue
			}
			if DependencyID(c.Dependencies[i]) == newID {
				return fmt.Errorf("dependency id %q already used", newID)
			}
		}
	}
	// Replace.
	c.Dependencies[idx] = dep
	return nil
}
