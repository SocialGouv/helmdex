package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"helmdex/internal/catalog"
	"helmdex/internal/config"
	"helmdex/internal/yamlchart"

	"gopkg.in/yaml.v3"
)

func TestE2E_RemoteCatalogAndSets_Hermetic(t *testing.T) {
	// This test is intentionally hermetic:
	// - "remote" catalog + preset repo is a local git repository
	// - no network access required
	// - no Helm binary required (we pre-write a matching Chart.lock so instance apply skips relock)
	repoRoot := t.TempDir()

	// 1) Create a local git repo that acts as the remote source.
	// We build it from checked-in fixtures so it also serves as documentation.
	remoteRepo := t.TempDir()
	fixtureRoot := filepath.Join(findRepoRoot(t), "fixtures", "remote-source")
	copyDir(t, fixtureRoot, remoteRepo)
	gitInitRepo(t, remoteRepo)
	gitCommitAll(t, remoteRepo, "e2e fixtures")

	const (
		sourceName = "s1"
		platform   = "eks"

		entryPG    = "bitnami-postgresql-15.5.0"
		entryNGINX = "bitnami-nginx-15.0.0"
		setDev     = "dev"
		setHA      = "ha-production"
	)

	// 2) Create a helmdex repo and write helmdex.yaml pointing at the local git repo.
	mustRunCLI(t, repoRoot, nil, "init")

	cfgPath := filepath.Join(repoRoot, "helmdex.yaml")
	cfg := config.Config{
		APIVersion: config.APIVersion,
		Kind:       config.Kind,
		Repo:       config.RepoConfig{AppsDir: "apps"},
		Platform:   config.PlatformConfig{Name: platform},
		Sources: []config.Source{
			{
				Name: sourceName,
				Git:  config.GitRef{URL: remoteRepo},
				Presets: config.PresetsConfig{
					Enabled:    true,
					ChartsPath: "charts",
				},
				Catalog: config.CatalogConfig{
					Enabled: true,
					Path:    "catalog.yaml",
				},
			},
		},
	}
	if err := config.WriteFile(cfgPath, cfg); err != nil {
		t.Fatalf("write helmdex.yaml: %v", err)
	}

	// 3) Sync remote catalog into .helmdex and verify entries are visible locally.
	mustRunCLI(t, repoRoot, &cfgPath, "catalog", "sync")
	out := mustRunCLI(t, repoRoot, &cfgPath, "catalog", "list", "--format", "json")
	{
		// The CLI JSON output currently uses Go struct field names (e.g. "ID")
		// because catalog entries do not have json tags.
		// Parse into the actual type to be resilient.
		var got []catalog.Entry
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatalf("parse catalog list json: %v\noutput: %s", err, out)
		}
		foundPG := false
		foundNG := false
		for _, e := range got {
			if e.ID == entryPG {
				foundPG = true
			}
			if e.ID == entryNGINX {
				foundNG = true
			}
		}
		if !foundPG || !foundNG {
			t.Fatalf("expected %q and %q in catalog list output, got: %s", entryPG, entryNGINX, out)
		}
	}
	// Also verify `catalog get` works (example/documentation).
	_ = mustRunCLI(t, repoRoot, &cfgPath, "catalog", "get", entryPG, "--format", "json")
	_ = mustRunCLI(t, repoRoot, &cfgPath, "catalog", "get", entryNGINX, "--format", "json")

	// 4) Create 2 instances, one per catalog entry.
	// This avoids multi-doc layer concatenation (multiple deps) while still demonstrating
	// that the catalog can target multiple charts.
	runOneInstance(t, repoRoot, cfgPath, "pg", entryPG, setDev, setHA)
	runOneInstance(t, repoRoot, cfgPath, "nginx", entryNGINX, setDev, setHA)
}

func runOneInstance(t *testing.T, repoRoot string, cfgPath string, instance string, catalogID string, defaultSet string, selectedSet string) {
	t.Helper()

	mustRunCLI(t, repoRoot, &cfgPath, "instance", "create", instance)

	// Add dependency from catalog (materializes defaultSets selection).
	mustRunCLI(t, repoRoot, &cfgPath, "instance", "dep", "add-from-catalog", instance, "--id", catalogID)

	instDir := filepath.Join(repoRoot, "apps", instance)
	depID := onlyDepID(t, filepath.Join(instDir, "Chart.yaml"))
	// Per-dependency marker files (selection-by-presence).
	defaultPath := filepath.Join(instDir, fmt.Sprintf("values.dep-set.%s--%s.yaml", depID, defaultSet))
	if _, err := os.Stat(defaultPath); err != nil {
		t.Fatalf("expected default set file to be materialized: %v", err)
	}

	// Demonstrate set selection:
	// - defaultSet is created by add-from-catalog
	// - we deselect it by deleting the file
	// - we select another set by creating its file (presence-based)
	if err := os.Remove(defaultPath); err != nil {
		t.Fatalf("deselect default set %q: %v", defaultSet, err)
	}
	if err := os.WriteFile(filepath.Join(instDir, fmt.Sprintf("values.dep-set.%s--%s.yaml", depID, selectedSet)), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("select set %q: %v", selectedSet, err)
	}

	// Keep apply hermetic by pre-writing a matching Chart.lock (so deps are "in sync" and relock is skipped).
	// The lock must contain tuples matching Chart.yaml (order-insensitive).
	lock := lockFromChartYAML(t, filepath.Join(instDir, "Chart.yaml"))
	lockBytes, err := yaml.Marshal(lock)
	if err != nil {
		t.Fatalf("marshal Chart.lock: %v", err)
	}
	if err := os.WriteFile(filepath.Join(instDir, "Chart.lock"), lockBytes, 0o644); err != nil {
		t.Fatalf("write Chart.lock: %v", err)
	}

	// Add per-dependency overrides to prove final precedence.
	mustRunCLI(t, repoRoot, &cfgPath, "instance", "dep", "values", "set", instance, depID, "--path", "$.global.tier", "--value-yaml", "instance")
	mustRunCLI(t, repoRoot, &cfgPath, "instance", "dep", "values", "set", instance, depID, "--path", "$.app.replicas", "--value-yaml", "3")

	// Apply: imports default/platform/selected sets from remote cache + generates merged values.yaml.
	mustRunCLI(t, repoRoot, &cfgPath, "instance", "apply", instance)

	mergedBytes, err := os.ReadFile(filepath.Join(instDir, "values.yaml"))
	if err != nil {
		t.Fatalf("read values.yaml: %v", err)
	}
	var merged map[string]any
	if err := yaml.Unmarshal(mergedBytes, &merged); err != nil {
		t.Fatalf("parse values.yaml: %v\nvalues.yaml:\n%s", err, string(mergedBytes))
	}

	depRootAny, ok := merged[depID]
	if !ok {
		t.Fatalf("expected %q root in values.yaml, got keys: %v", depID, keys(merged))
	}
	depRoot, _ := depRootAny.(map[string]any)
	if depRoot == nil {
		t.Fatalf("expected %q to be a map in values.yaml, got: %#v", depID, depRootAny)
	}

	// app.replicas comes from per-dependency override.
	app, _ := depRoot["app"].(map[string]any)
	if app == nil {
		t.Fatalf("expected %s.app map in values.yaml, got: %#v", depID, depRoot["app"])
	}
	if got, want := intFromAny(app["replicas"]), 3; got != want {
		t.Fatalf("%s.app.replicas: got %d want %d", depID, got, want)
	}

	// global.tier comes from per-dependency override.
	global, _ := depRoot["global"].(map[string]any)
	if global == nil {
		t.Fatalf("expected %s.global map in values.yaml, got: %#v", depID, depRoot["global"])
	}
	if got, want := strFromAny(global["tier"]), "instance"; got != want {
		t.Fatalf("%s.global.tier: got %q want %q", depID, got, want)
	}

	// pfOnly comes from platform layer.
	if got, want := boolFromAny(depRoot["pfOnly"]), true; got != want {
		t.Fatalf("%s.pfOnly: got %v want %v", depID, got, want)
	}

	// Selected set content should appear; default set content should not.
	if got, want := boolFromAny(depRoot["setHA"]), true; got != want {
		t.Fatalf("%s.setHA: got %v want %v", depID, got, want)
	}
	if got := boolFromAny(depRoot["setDev"]); got {
		t.Fatalf("%s.setDev: expected false (default set was deselected), got true", depID)
	}
}

func onlyDepID(t *testing.T, chartPath string) string {
	t.Helper()
	c, err := yamlchart.ReadChart(chartPath)
	if err != nil {
		t.Fatalf("read Chart.yaml: %v", err)
	}
	if len(c.Dependencies) != 1 {
		t.Fatalf("expected exactly 1 dependency in %s, got %d", chartPath, len(c.Dependencies))
	}
	return string(yamlchart.DependencyID(c.Dependencies[0]))
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func lockFromChartYAML(t *testing.T, chartPath string) yamlchart.Lock {
	t.Helper()
	c, err := yamlchart.ReadChart(chartPath)
	if err != nil {
		t.Fatalf("read Chart.yaml: %v", err)
	}
	l := yamlchart.Lock{APIVersion: "v2"}
	for _, d := range c.Dependencies {
		l.Dependencies = append(l.Dependencies, yamlchart.LockDependency{Name: d.Name, Version: d.Version, Repository: d.Repository})
	}
	return l
}

func mustRunCLI(t *testing.T, repoRoot string, cfgPath *string, args ...string) string {
	t.Helper()

	cmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	full := []string{"--repo", repoRoot}
	if cfgPath != nil {
		full = append(full, "--config", *cfgPath)
	}
	full = append(full, args...)
	cmd.SetArgs(full)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("helmdex %v failed: %v\nstdout:\n%s\nstderr:\n%s", args, err, stdout.String(), stderr.String())
	}
	if stderr.Len() > 0 {
		// Cobra commands occasionally write non-fatal info to stderr. Keep it visible in failure output.
	}
	return stdout.String()
}

func copyDir(t *testing.T, src string, dst string) {
	t.Helper()
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatalf("mkdir dst %s: %v", dst, err)
	}

	walkFn := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		// Do not copy any VCS directories if present.
		if strings.HasPrefix(rel, ".git") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		defer out.Close()
		if _, err := io.Copy(out, in); err != nil {
			return err
		}
		return nil
	}

	if err := filepath.WalkDir(src, walkFn); err != nil {
		t.Fatalf("copy dir %s -> %s: %v", src, dst, err)
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	dir := filepath.Dir(thisFile)
	for i := 0; i < 10; i++ {
		cand := filepath.Clean(filepath.Join(dir, strings.Repeat("../", i)))
		if _, err := os.Stat(filepath.Join(cand, "go.mod")); err == nil {
			return cand
		}
	}
	// Fall back to current working directory; helpful error for debugging.
	wd, _ := os.Getwd()
	t.Fatalf("could not locate repo root (go.mod) from %s (wd=%s)", thisFile, wd)
	return ""
}

func gitInitRepo(t *testing.T, dir string) {
	t.Helper()
	git(t, dir, "init")
	git(t, dir, "config", "user.email", "e2e@example.invalid")
	git(t, dir, "config", "user.name", "helmdex-e2e")
}

func gitCommitAll(t *testing.T, dir string, msg string) {
	t.Helper()
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-m", msg)
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(b))
	}
}

func strFromAny(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func intFromAny(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case float32:
		return int(x)
	default:
		return 0
	}
}

func boolFromAny(v any) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}
