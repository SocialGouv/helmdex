package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"helmdex/internal/appinfo"
	"helmdex/internal/testutil"
)

func TestVersion_ReportsBuildInfo(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})

	var v versionInfo
	ts.get("/api/version").expect(http.StatusOK).decode(&v)
	if v.Version != appinfo.Version {
		t.Fatalf("version = %q, want %q", v.Version, appinfo.Version)
	}
	if v.RepoURL != appinfo.RepoURL {
		t.Fatalf("repoUrl = %q, want %q", v.RepoURL, appinfo.RepoURL)
	}
}

// stubGitHub points the update check at a local release payload; restores on
// cleanup.
func stubGitHub(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	stub := httptest.NewServer(handler)
	prev := githubAPIBase
	githubAPIBase = stub.URL
	t.Cleanup(func() {
		githubAPIBase = prev
		stub.Close()
	})
}

// setVersion overrides the build-time version for the duration of a test.
func setVersion(t *testing.T, v string) {
	t.Helper()
	prev := appinfo.Version
	appinfo.Version = v
	t.Cleanup(func() { appinfo.Version = prev })
}

func TestVersionCheck_ReportsAvailableUpdate(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	setVersion(t, "v0.1.0")
	stubGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/releases/latest") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"tag_name":"v9.9.9","html_url":"https://github.com/SocialGouv/helmdex/releases/tag/v9.9.9"}`))
	})

	var chk versionCheck
	ts.get("/api/version/check").expect(http.StatusOK).decode(&chk)
	if !chk.UpdateAvailable || chk.Latest != "v9.9.9" || chk.Current != "v0.1.0" {
		t.Fatalf("check = %+v", chk)
	}
	if !strings.Contains(chk.ReleaseURL, "/releases/tag/v9.9.9") {
		t.Fatalf("releaseUrl = %q", chk.ReleaseURL)
	}
}

func TestVersionCheck_UpToDateAndDevBuildsNeverClaimUpdates(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	stubGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v9.9.9","html_url":"u"}`))
	})

	// A dev build has no comparable baseline: report the tag, claim nothing.
	setVersion(t, "dev")
	var chk versionCheck
	ts.as(t).get("/api/version/check").expect(http.StatusOK).decode(&chk)
	if chk.UpdateAvailable {
		t.Fatalf("dev build must not claim an update: %+v", chk)
	}

	setVersion(t, "v9.9.9")
	ts.as(t).get("/api/version/check").expect(http.StatusOK).decode(&chk)
	if chk.UpdateAvailable {
		t.Fatalf("same version must not claim an update: %+v", chk)
	}
}

func TestVersionCheck_SurfacesGitHubFailure(t *testing.T) {
	ts := newTestServer(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	stubGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"boom"}`, http.StatusInternalServerError)
	})

	res := ts.get("/api/version/check").expect(http.StatusBadGateway)
	if msg := res.errorMessage(); !strings.Contains(msg, "500") {
		t.Fatalf("error should carry the upstream status, got %q", msg)
	}
}
