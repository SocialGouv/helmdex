package instances

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"helmdex/internal/helmutil"
	"helmdex/internal/yamlchart"
)

type Instance struct {
	Name string
	Path string
}

func instanceDir(repoRoot, appsDir, name string) string {
	if appsDir == "" {
		appsDir = "apps"
	}
	return filepath.Join(repoRoot, appsDir, name)
}

func Create(repoRoot, appsDir, name string) (Instance, error) {
	if name == "" {
		return Instance{}, fmt.Errorf("instance name is required")
	}
	if strings.Contains(name, string(os.PathSeparator)) {
		return Instance{}, fmt.Errorf("invalid instance name %q", name)
	}

	dir := instanceDir(repoRoot, appsDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Instance{}, err
	}

	chartPath := filepath.Join(dir, "Chart.yaml")
	if _, err := os.Stat(chartPath); err == nil {
		return Instance{}, fmt.Errorf("instance already exists at %s", dir)
	}

	chart := yamlchart.NewUmbrellaChart(name)
	if err := yamlchart.WriteChart(chartPath, chart); err != nil {
		return Instance{}, err
	}

	// User-owned file; helmdex should not overwrite if it already exists.
	instanceValues := filepath.Join(dir, "values.instance.yaml")
	if _, err := os.Stat(instanceValues); err != nil {
		_ = os.WriteFile(instanceValues, []byte("# User overrides for this instance\n{}\n"), 0o644)
	}

	return Instance{Name: name, Path: dir}, nil
}

func Get(repoRoot, appsDir, name string) (Instance, error) {
	dir := instanceDir(repoRoot, appsDir, name)
	if _, err := os.Stat(filepath.Join(dir, "Chart.yaml")); err != nil {
		return Instance{}, fmt.Errorf("instance %q not found at %s", name, dir)
	}
	return Instance{Name: name, Path: dir}, nil
}

func List(repoRoot, appsDir string) ([]Instance, error) {
	root := filepath.Join(repoRoot, appsDir)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []Instance{}, nil
		}
		return nil, err
	}
	var out []Instance
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(root, e.Name())
		if _, err := os.Stat(filepath.Join(p, "Chart.yaml")); err == nil {
			out = append(out, Instance{Name: e.Name(), Path: p})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func Remove(repoRoot, appsDir, name string) error {
	dir := instanceDir(repoRoot, appsDir, name)
	return os.RemoveAll(dir)
}

func RelockDependencies(ctx context.Context, repoRoot, instancePath string) error {
	env := helmutil.EnvForRepo(repoRoot)
	// v0.1: prefer `helm dependency build` (uses lockfile) when lock exists, else update.
	lockPath := filepath.Join(instancePath, "Chart.lock")
	if _, err := os.Stat(lockPath); err == nil {
		// If the lock exists but is out-of-sync with Chart.yaml, Helm requires an
		// update to re-resolve and regenerate Chart.lock.
		if err := helmutil.DependencyBuild(ctx, env, instancePath); err != nil {
			s := err.Error()
			if strings.Contains(s, "Chart.lock") && strings.Contains(s, "out of sync") {
				return helmutil.DependencyUpdate(ctx, env, instancePath)
			}
			return err
		}
		return nil
	}
	return helmutil.DependencyUpdate(ctx, env, instancePath)
}

// RelockIfDepsChanged re-locks dependencies only when the instance's declared
// dependencies are out of sync with Chart.lock.
//
// If Chart.lock does not exist, this will relock only when Chart.yaml declares
// any dependencies.
func RelockIfDepsChanged(ctx context.Context, repoRoot, instancePath string) (bool, error) {
	changed, err := DepsChanged(instancePath)
	if err != nil {
		return false, err
	}
	if !changed {
		return false, nil
	}
	return true, RelockDependencies(ctx, repoRoot, instancePath)
}
