package server

import (
	"net/http"
	"os"
	"strings"
	"testing"

	"helmdex/internal/config"
	"helmdex/internal/instances"
	"helmdex/internal/testutil"
)

func TestConfigSources_GetReportsResolutionOrigin(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceGit})

	var info sourcesInfo
	ts.get("/api/config/sources").expect(http.StatusOK).decode(&info)

	if info.Platform != "eks" {
		t.Fatalf("platform = %q", info.Platform)
	}
	if len(info.Sources) != 1 || info.Sources[0].Name != "Example" {
		t.Fatalf("sources = %+v", info.Sources)
	}
	if info.SaveSource != string(config.SourceRepo) {
		t.Fatalf("saveSource = %q, want %q", info.SaveSource, config.SourceRepo)
	}
	if info.SavePath != ts.Repo.Path("helmdex.yaml") {
		t.Fatalf("savePath = %q", info.SavePath)
	}
}

func TestConfigSources_PutPersistsAndReloads(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceGit})

	var before sourcesInfo
	ts.get("/api/config/sources").expect(http.StatusOK).decode(&before)

	updated := append([]config.Source{}, before.Sources...)
	updated[0].Catalog.Enabled = false

	var after sourcesInfo
	ts.put("/api/config/sources", sourcesPutRequest{Platform: "gke", Sources: updated}).
		expect(http.StatusOK).decode(&after)

	if after.Platform != "gke" || after.Sources[0].Catalog.Enabled {
		t.Fatalf("response does not reflect the save: %+v", after)
	}
	// Persisted to the repo config, and the live workspace reloaded.
	onDisk := ts.Repo.Read(t, "helmdex.yaml")
	if !strings.Contains(onDisk, "name: gke") {
		t.Fatalf("platform not persisted:\n%s", onDisk)
	}
	if got := ts.API.Workspace().Config.Platform.Name; got != "gke" {
		t.Fatalf("live workspace platform = %q, want gke", got)
	}
}

func TestConfigSources_PutRejectsInvalidConfig(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceGit})

	// A source without a git URL is invalid.
	ts.put("/api/config/sources", sourcesPutRequest{
		Platform: "eks",
		Sources:  []config.Source{{Name: "Broken"}},
	}).expect(http.StatusBadRequest)

	if onDisk := ts.Repo.Read(t, "helmdex.yaml"); strings.Contains(onDisk, "Broken") {
		t.Fatalf("an invalid config must not be written:\n%s", onDisk)
	}
}

// Saving from an agnostic repo goes to the user config. It must carry the
// global fields only: writing the discovered repo layout back would promote
// this repo's layout to the default for every other agnostic repo.
func TestConfigSources_PutDoesNotLeakRepoLayoutIntoUserConfig(t *testing.T) {
	// A repo whose instances live somewhere other than apps/, so layout
	// discovery produces a value that would be visible if it leaked.
	ts := newTestServer(t, testutil.RepoOpts{
		Agnostic: true,
		Seed: func(t *testing.T, r testutil.Repo) {
			if err := os.RemoveAll(r.Path("apps")); err != nil {
				t.Fatalf("drop apps dir: %v", err)
			}
			r.Write(t, "apiVersion: v2\nname: preprod\nversion: 0.1.0\n",
				"environments", "preprod", "Chart.yaml")
			r.Write(t, "apiVersion: v2\nname: prod\nversion: 0.1.0\n",
				"environments", "prod", "Chart.yaml")
		},
	})

	if got := ts.API.Workspace().Config.Repo.AppsDir; got != "environments" {
		t.Fatalf("layout discovery found appsDir %q, want environments", got)
	}

	var info sourcesInfo
	ts.get("/api/config/sources").expect(http.StatusOK).decode(&info)
	if info.SaveSource == string(config.SourceRepo) {
		t.Fatalf("an agnostic repo must not save to a repo config, got %q", info.SaveSource)
	}

	ts.put("/api/config/sources", sourcesPutRequest{
		Platform: "eks",
		Sources:  []config.Source{},
	}).expect(http.StatusOK)

	userConfig := os.Getenv("HELMDEX_USER_CONFIG")
	b, err := os.ReadFile(userConfig)
	if err != nil {
		t.Fatalf("read user config %s: %v", userConfig, err)
	}
	written := string(b)
	// The discovered layout must not become the default for every other repo.
	if strings.Contains(written, "environments") {
		t.Fatalf("the discovered repo layout leaked into the user config:\n%s", written)
	}
	if strings.Contains(written, "repos:") {
		t.Fatalf("per-repo overrides were invented on save:\n%s", written)
	}
	// The agnostic repo stays untouched.
	if ts.Repo.Exists(t, "helmdex.yaml") {
		t.Fatal("saving config wrote a helmdex.yaml into an agnostic repo")
	}
}

func TestWorkspace_SwitchServesTheNewRepo(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	ts.createInstance("alpha")

	other := testutil.NewRepo(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	other.Write(t, "apiVersion: v2\nname: bravo\nversion: 0.1.0\n", "apps", "bravo", "Chart.yaml")

	res, err := config.Resolve(other.Root, "")
	if err != nil {
		t.Fatalf("resolve other repo: %v", err)
	}
	ts.API.SetWorkspace(Params{
		RepoRoot: other.Root,
		Config:   instances.ApplyLayout(other.Root, res),
		Resolved: res,
	})

	var info repoInfo
	ts.get("/api/repo").expect(http.StatusOK).decode(&info)
	if info.Root != other.Root {
		t.Fatalf("repo root = %q, want %q", info.Root, other.Root)
	}

	var list []instanceInfo
	ts.get("/api/instances").expect(http.StatusOK).decode(&list)
	if len(list) != 1 || list[0].Name != "bravo" {
		t.Fatalf("instances after switch = %+v", list)
	}
	ts.get("/api/instances/alpha").expect(http.StatusNotFound)
}

func TestWorkspace_ConfigErrorIsSurfaced(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})

	ts.API.SetWorkspace(Params{
		RepoRoot:    ts.Repo.Root,
		Config:      ts.API.Workspace().Config,
		Resolved:    ts.API.Workspace().Resolved,
		ConfigError: "helmdex.yaml: line 3: mapping values are not allowed",
	})

	var info repoInfo
	ts.get("/api/repo").expect(http.StatusOK).decode(&info)
	if !strings.Contains(info.ConfigError, "mapping values") {
		t.Fatalf("configError not surfaced: %+v", info)
	}
}

func TestCatalog_SyncThenList(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceGit})

	var empty []map[string]any
	ts.get("/api/catalog").expect(http.StatusOK).decode(&empty)
	if len(empty) != 0 {
		t.Fatalf("catalog must be empty before a sync, got %+v", empty)
	}

	var results []map[string]any
	ts.post("/api/catalog/sync", nil).expect(http.StatusOK).decode(&results)
	if len(results) != 1 {
		t.Fatalf("expected one synced source, got %+v", results)
	}

	// catalog.EntryWithSource carries no json tags, so the wire shape is
	// PascalCase — webui/src/api/types.ts models it that way.
	var entries []struct {
		Entry struct {
			ID      string
			Version string
			Chart   struct{ Repo, Name string }
		}
		SourceName string
	}
	ts.get("/api/catalog").expect(http.StatusOK).decode(&entries)

	byID := map[string]string{}
	for _, e := range entries {
		if e.SourceName != "Example" {
			t.Fatalf("entry %q lost its source name: %+v", e.Entry.ID, e)
		}
		byID[e.Entry.ID] = e.Entry.Version
	}
	for id, version := range map[string]string{
		"bitnami-postgresql-15.5.0": "15.5.0",
		"bitnami-nginx-15.0.0":      "15.0.0",
	} {
		if byID[id] != version {
			t.Fatalf("catalog entry %q = %q, want %q (entries: %+v)", id, byID[id], version, entries)
		}
	}
}

func TestCatalog_SyncFailureIsSurfaced(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceGit})

	// Point the source at a path that is not a git repository.
	var info sourcesInfo
	ts.get("/api/config/sources").expect(http.StatusOK).decode(&info)
	info.Sources[0].Git.URL = ts.Repo.Path("absent-source")
	ts.put("/api/config/sources", sourcesPutRequest{Platform: info.Platform, Sources: info.Sources}).
		expect(http.StatusOK)

	res := ts.post("/api/catalog/sync", nil).expect(http.StatusBadGateway)
	if strings.TrimSpace(res.errorMessage()) == "" {
		t.Fatal("a failed sync must explain why")
	}
}

func TestArtifactHub_DisabledByConfig(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})

	res := ts.get("/api/artifacthub/search?q=nginx").expect(http.StatusForbidden)
	if !strings.Contains(res.errorMessage(), "disabled") {
		t.Fatalf("unexpected error: %s", res.errorMessage())
	}
}

// The limit is validated before any network call, so this stays hermetic.
func TestArtifactHub_LimitIsValidated(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone, ArtifactHub: true})

	for _, limit := range []string{"0", "-1", "101", "abc"} {
		t.Run(limit, func(t *testing.T) {
			ts.as(t).get("/api/artifacthub/search?q=nginx&limit=" + limit).
				expect(http.StatusBadRequest)
		})
	}
}
