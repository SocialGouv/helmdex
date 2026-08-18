package testutil_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"helmdex/internal/helmutil"
	"helmdex/internal/instances"
	"helmdex/internal/testutil"
	"helmdex/internal/yamlchart"
)

// These tests pin the contract the rest of the e2e suites rely on: with
// Hermetic in place, every Helm-dependent helmutil entry point works offline
// and returns deterministic data.

func TestHermetic_ResolvesFakeHelm(t *testing.T) {
	testutil.Hermetic(t)
	repo := testutil.NewRepo(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})

	ctx := context.Background()
	env := helmutil.EnvForRepo(repo.Root)

	// Like real Helm, the fake serves a classic reference only once its
	// repository has been registered — so `helm repo add` stays load-bearing
	// in every suite built on this.
	if _, err := helmutil.ShowChart(ctx, env, "unregistered/nginx", "15.1.0"); err == nil {
		t.Fatal("expected an unregistered repo to be refused")
	}

	if err := helmutil.RepoAdd(ctx, env, "any", "https://example.invalid/charts"); err != nil {
		t.Fatalf("repo add: %v", err)
	}
	out, err := helmutil.ShowChart(ctx, env, "any/nginx", "15.1.0")
	if err != nil {
		t.Fatalf("show chart: %v", err)
	}
	if !strings.Contains(out, "name: nginx") || !strings.Contains(out, "version: 15.1.0") {
		t.Fatalf("unexpected chart metadata:\n%s", out)
	}
}

func TestHermetic_RepoChartVersions_FiltersSubstringMatches(t *testing.T) {
	testutil.Hermetic(t)
	repo := testutil.NewRepo(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})

	vs, err := helmutil.RepoChartVersions(context.Background(), repo.Root,
		"https://example.invalid/charts", "postgresql", time.Hour)
	if err != nil {
		t.Fatalf("versions: %v", err)
	}
	// Newest stable first, and postgresql-ha's versions must not leak in
	// despite `helm search repo` matching it as a substring.
	want := []string{"16.0.0", "15.6.0", "15.5.0", "15.4.0"}
	if strings.Join(vs, ",") != strings.Join(want, ",") {
		t.Fatalf("versions = %v, want %v", vs, want)
	}
}

func TestHermetic_RelockWritesLockAndVendorsCharts(t *testing.T) {
	logPath := testutil.Hermetic(t)
	repo := testutil.NewRepo(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})

	instPath := repo.InstancePath("alpha")
	chart := yamlchart.NewUmbrellaChart("alpha")
	chart.Dependencies = []yamlchart.Dependency{{
		Name:       "postgresql",
		Version:    "15.5.0",
		Repository: "https://example.invalid/charts",
	}}
	if err := yamlchart.WriteChart(filepath.Join(instPath, "Chart.yaml"), chart); err != nil {
		t.Fatalf("write chart: %v", err)
	}

	if err := instances.RelockDependencies(context.Background(), repo.Root, instPath); err != nil {
		t.Fatalf("relock: %v", err)
	}

	lock, err := yamlchart.ReadLock(filepath.Join(instPath, "Chart.lock"))
	if err != nil {
		t.Fatalf("read lock: %v", err)
	}
	if len(lock.Dependencies) != 1 || lock.Dependencies[0].Version != "15.5.0" {
		t.Fatalf("unexpected lock dependencies: %+v", lock.Dependencies)
	}
	if _, err := os.Stat(filepath.Join(instPath, "charts", "postgresql-15.5.0.tgz")); err != nil {
		t.Fatalf("vendored archive missing: %v", err)
	}

	// Chart.yaml and Chart.lock now agree, so a second reconcile must not
	// invoke Helm again.
	before := len(testutil.FakeHelmCalls(t, logPath))
	changed, err := instances.RelockIfDepsChanged(context.Background(), repo.Root, instPath)
	if err != nil {
		t.Fatalf("relock if changed: %v", err)
	}
	if changed {
		t.Fatal("expected no relock when Chart.yaml matches Chart.lock")
	}
	if after := len(testutil.FakeHelmCalls(t, logPath)); after != before {
		t.Fatalf("expected no further helm calls, got %v", testutil.FakeHelmCalls(t, logPath)[before:])
	}
}

func TestHermetic_DepInspectReadsVendoredArchive(t *testing.T) {
	testutil.Hermetic(t)
	repo := testutil.NewRepo(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})

	instPath := repo.InstancePath("alpha")
	dep := yamlchart.Dependency{
		Name:       "nginx",
		Version:    "15.0.0",
		Repository: "https://example.invalid/charts",
	}
	chart := yamlchart.NewUmbrellaChart("alpha")
	chart.Dependencies = []yamlchart.Dependency{dep}
	if err := yamlchart.WriteChart(filepath.Join(instPath, "Chart.yaml"), chart); err != nil {
		t.Fatalf("write chart: %v", err)
	}

	schema, err := instances.LoadDepInspectContent(context.Background(), repo.Root, instPath, dep, instances.InspectSchema)
	if err != nil {
		t.Fatalf("inspect schema: %v", err)
	}
	if !strings.Contains(schema, `"replicaCount"`) {
		t.Fatalf("unexpected schema:\n%s", schema)
	}

	values, err := instances.LoadDepInspectContent(context.Background(), repo.Root, instPath, dep, instances.InspectValues)
	if err != nil {
		t.Fatalf("inspect values: %v", err)
	}
	if !strings.Contains(values, "repository: fake/nginx") {
		t.Fatalf("unexpected values:\n%s", values)
	}
}

func TestNewRepo_AgnosticFixtureStaysUntouched(t *testing.T) {
	testutil.Hermetic(t)
	repo := testutil.NewRepo(t, testutil.RepoOpts{Agnostic: true})

	if repo.ConfigPath != "" {
		t.Fatalf("agnostic repo must not declare a helmdex.yaml, got %q", repo.ConfigPath)
	}
	if repo.Exists(t, "helmdex.yaml") {
		t.Fatal("agnostic repo must not contain helmdex.yaml")
	}
	if !repo.Exists(t, "apps", "demo-preprod", "Chart.yaml") {
		t.Fatal("agnostic fixture not copied")
	}
}
