package server

import (
	"net/http"
	"strings"

	"helmdex/internal/appinfo"
	"helmdex/internal/semverutil"
	"helmdex/internal/updates"
)

// githubAPIBase is the GitHub API root; tests swap it for a local stub.
var githubAPIBase = "https://api.github.com"

type versionInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit,omitempty"`
	RepoURL string `json:"repoUrl"`
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	commit := appinfo.Commit
	if len(commit) > 12 {
		commit = commit[:12]
	}
	writeJSON(w, http.StatusOK, versionInfo{
		Version: appinfo.Version,
		Commit:  commit,
		RepoURL: appinfo.RepoURL,
	})
}

type versionCheck struct {
	Current         string `json:"current"`
	Latest          string `json:"latest"`
	UpdateAvailable bool   `json:"updateAvailable"`
	ReleaseURL      string `json:"releaseUrl"`
}

// handleVersionCheck asks GitHub for the latest release. Unparseable current
// versions (dev builds) report the latest tag but never claim an update.
func (s *Server) handleVersionCheck(w http.ResponseWriter, r *http.Request) {
	repo := strings.TrimPrefix(appinfo.RepoURL, "https://github.com/")
	rel, err := updates.Latest(r.Context(), githubAPIBase, repo)
	if err != nil {
		httpError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, versionCheck{
		Current:         appinfo.Version,
		Latest:          rel.TagName,
		UpdateAvailable: semverutil.Newer(rel.TagName, appinfo.Version),
		ReleaseURL:      rel.HTMLURL,
	})
}
