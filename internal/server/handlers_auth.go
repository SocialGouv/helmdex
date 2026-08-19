package server

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"helmdex/internal/authsvc"
	"helmdex/internal/creds"
	"helmdex/internal/instances"
	"helmdex/internal/yamlchart"
)

// --- auth-aware error reporting ---

// authRequiredPayload rides along API errors that look like a remote
// authentication failure, so the UI can offer a sign-in flow.
type authRequiredPayload struct {
	Candidates []creds.AuthCandidate `json:"candidates"`
}

type apiAuthError struct {
	Error        string               `json:"error"`
	AuthRequired *authRequiredPayload `json:"authRequired,omitempty"`
}

// httpOpError writes err with the given status; when the error text looks
// like an authentication failure against one of the candidate remotes, it
// responds 401 with the candidates attached instead.
func httpOpError(w http.ResponseWriter, status int, err error, cands ...creds.AuthCandidate) {
	valid := make([]creds.AuthCandidate, 0, len(cands))
	for _, c := range cands {
		if c.Host != "" {
			valid = append(valid, c)
		}
	}
	cands = valid
	if err != nil && len(cands) > 0 && creds.IsAuthError(err.Error()) {
		writeJSON(w, http.StatusUnauthorized, apiAuthError{
			Error:        err.Error(),
			AuthRequired: &authRequiredPayload{Candidates: creds.MatchCandidates(err.Error(), cands)},
		})
		return
	}
	httpError(w, status, err)
}

// authCandidatesForChart lists the remotes an instance's dependency
// operations may need credentials for.
func authCandidatesForChart(instPath string) []creds.AuthCandidate {
	c, err := yamlchart.ReadChart(filepath.Join(instPath, "Chart.yaml"))
	if err != nil {
		return nil
	}
	var out []creds.AuthCandidate
	seen := map[string]struct{}{}
	for _, d := range c.Dependencies {
		cand, ok := creds.CandidateForRepo(d.Repository)
		if !ok {
			continue
		}
		key := cand.Host + "|" + string(cand.Kind)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, cand)
	}
	return out
}

// authCandidatesForSources lists the git remotes of the configured sources.
func (s *Server) authCandidatesForSources() []creds.AuthCandidate {
	var out []creds.AuthCandidate
	for _, src := range s.ws().Config.Sources {
		if cand, ok := creds.CandidateForGit(src.Git.URL); ok {
			out = append(out, cand)
		}
	}
	return out
}

// --- endpoints ---

func (s *Server) handleAuthCredsList(w http.ResponseWriter, r *http.Request) {
	list, err := creds.List()
	if err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	// creds.Credential marshals without the secret; keep the response an
	// array even when empty.
	if list == nil {
		list = []creds.Credential{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleAuthCredRemove(w http.ResponseWriter, r *http.Request) {
	host := strings.TrimSpace(r.PathValue("host"))
	kind := creds.Kind(strings.TrimSpace(r.URL.Query().Get("kind")))
	if kind != "" && !creds.ValidKind(kind) {
		httpError(w, http.StatusBadRequest, fmt.Errorf("invalid kind %q", kind))
		return
	}
	removed, err := authsvc.Logout(s.ws().RepoRoot, host, kind)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	if !removed {
		httpError(w, http.StatusNotFound, fmt.Errorf("no stored credential for %q", host))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type authDetectRequest struct {
	Host string     `json:"host"`
	Kind creds.Kind `json:"kind"`
}

type authDetectResponse struct {
	Candidates []creds.Candidate `json:"candidates"`
	TokenPage  creds.TokenPage   `json:"tokenPage"`
}

func (s *Server) handleAuthDetect(w http.ResponseWriter, r *http.Request) {
	var req authDetectRequest
	if err := decodeJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.Host) == "" {
		httpError(w, http.StatusBadRequest, fmt.Errorf("host is required"))
		return
	}
	if !creds.ValidKind(req.Kind) {
		httpError(w, http.StatusBadRequest, fmt.Errorf("kind must be one of oci, git, helm-repo"))
		return
	}
	cands := creds.Detect(r.Context(), req.Host, req.Kind)
	if cands == nil {
		cands = []creds.Candidate{}
	}
	writeJSON(w, http.StatusOK, authDetectResponse{
		Candidates: cands,
		TokenPage:  creds.TokenPageFor(req.Host, req.Kind),
	})
}

func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	var req authsvc.LoginRequest
	if err := decodeJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, err)
		return
	}
	res, err := authsvc.Login(r.Context(), s.ws().RepoRoot, req)
	if err != nil {
		// A failed verification is a client-fixable problem (wrong token,
		// missing scope), not a server fault.
		httpError(w, http.StatusBadRequest, err)
		return
	}
	s.events.publish(event{Type: "auth.login", Message: res.Host})
	writeJSON(w, http.StatusOK, res)
}

type authTokenPageRequest struct {
	Host string     `json:"host"`
	Kind creds.Kind `json:"kind"`
	// Open requests opening the page in the local browser (server side:
	// works for both `helmdex ui` and the desktop app).
	Open bool `json:"open,omitempty"`
}

func (s *Server) handleAuthTokenPage(w http.ResponseWriter, r *http.Request) {
	var req authTokenPageRequest
	if err := decodeJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.Host) == "" {
		httpError(w, http.StatusBadRequest, fmt.Errorf("host is required"))
		return
	}
	page := creds.TokenPageFor(req.Host, req.Kind)
	if req.Open {
		if err := authsvc.OpenBrowser(page.URL); err != nil {
			// Return the URL anyway so the UI can show it for manual opening.
			writeJSON(w, http.StatusOK, map[string]any{
				"provider": page.Provider, "url": page.URL, "host": page.Host,
				"openError": err.Error(),
			})
			return
		}
	}
	writeJSON(w, http.StatusOK, page)
}

// workspaceAuthHost summarizes one remote the open workspace references.
type workspaceAuthHost struct {
	creds.AuthCandidate
	HasCredential bool `json:"hasCredential"`
}

// handleAuthHosts lists every remote host the workspace references
// (dependency repositories across instances + configured git sources) and
// whether a credential is stored for it.
func (s *Server) handleAuthHosts(w http.ResponseWriter, r *http.Request) {
	ws := s.ws()
	var cands []creds.AuthCandidate
	list, err := instances.List(ws.RepoRoot, ws.Config.Repo.AppsDir)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	for _, inst := range list {
		cands = append(cands, authCandidatesForChart(inst.Path)...)
	}
	cands = append(cands, s.authCandidatesForSources()...)

	out := []workspaceAuthHost{}
	seen := map[string]struct{}{}
	for _, c := range cands {
		key := c.Host + "|" + string(c.Kind)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		_, has := creds.ForHost(c.Host, c.Kind)
		out = append(out, workspaceAuthHost{AuthCandidate: c, HasCredential: has})
	}
	writeJSON(w, http.StatusOK, out)
}
