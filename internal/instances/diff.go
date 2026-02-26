package instances

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"helmdex/internal/yamlchart"
)

// DepsChanged determines whether the dependencies declared in Chart.yaml differ
// from the dependencies recorded in Chart.lock.
//
// Semantics:
// - If Chart.lock does not exist, deps are considered changed when Chart.yaml has any dependencies.
// - If Chart.lock exists, compare (name, version, repository) tuples (order-insensitive).
func DepsChanged(instancePath string) (bool, error) {
	chartPath := filepath.Join(instancePath, "Chart.yaml")
	c, err := yamlchart.ReadChart(chartPath)
	if err != nil {
		return false, err
	}

	lockPath := filepath.Join(instancePath, "Chart.lock")
	if _, err := os.Stat(lockPath); err != nil {
		if os.IsNotExist(err) {
			return len(c.Dependencies) > 0, nil
		}
		return false, err
	}

	l, err := yamlchart.ReadLock(lockPath)
	if err != nil {
		return false, fmt.Errorf("read Chart.lock: %w", err)
	}

	fromChart := make([]depKey, 0, len(c.Dependencies))
	for _, d := range c.Dependencies {
		fromChart = append(fromChart, depKey{Name: d.Name, Version: d.Version, Repo: d.Repository})
	}

	fromLock := make([]depKey, 0, len(l.Dependencies))
	for _, d := range l.Dependencies {
		fromLock = append(fromLock, depKey{Name: d.Name, Version: d.Version, Repo: d.Repository})
	}

	sort.Slice(fromChart, func(i, j int) bool { return fromChart[i].Less(fromChart[j]) })
	sort.Slice(fromLock, func(i, j int) bool { return fromLock[i].Less(fromLock[j]) })

	if len(fromChart) != len(fromLock) {
		return true, nil
	}
	for i := range fromChart {
		if fromChart[i] != fromLock[i] {
			return true, nil
		}
	}
	return false, nil
}

type depKey struct {
	Name    string
	Version string
	Repo    string
}

func (a depKey) Less(b depKey) bool {
	if a.Name != b.Name {
		return a.Name < b.Name
	}
	if a.Repo != b.Repo {
		return a.Repo < b.Repo
	}
	return a.Version < b.Version
}
