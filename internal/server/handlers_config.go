package server

import (
	"net/http"

	"helmdex/internal/config"
	"helmdex/internal/instances"
)

// Sources/platform configuration. Saves go back to wherever the config was
// resolved from: the repo helmdex.yaml for opted-in repos, the user config
// (per-repo overrides preserved) for agnostic ones.

type sourcesInfo struct {
	Platform string          `json:"platform"`
	Sources  []config.Source `json:"sources"`
	// SavePath is where a save would be written.
	SavePath   string `json:"savePath"`
	SaveSource string `json:"saveSource"`
}

func (s *Server) handleConfigSourcesGet(w http.ResponseWriter, r *http.Request) {
	ws := s.ws()
	info := sourcesInfo{
		Platform:   ws.Config.Platform.Name,
		Sources:    ws.Config.Sources,
		SavePath:   ws.Resolved.Path,
		SaveSource: string(ws.Resolved.Source),
	}
	if info.Sources == nil {
		info.Sources = []config.Source{}
	}
	writeJSON(w, http.StatusOK, info)
}

type sourcesPutRequest struct {
	Platform string          `json:"platform"`
	Sources  []config.Source `json:"sources"`
}

func (s *Server) handleConfigSourcesPut(w http.ResponseWriter, r *http.Request) {
	var req sourcesPutRequest
	if err := decodeJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	ws := s.ws()
	cfg := ws.Config
	cfg.Platform.Name = req.Platform
	cfg.Sources = req.Sources
	if err := cfg.Validate(); err != nil {
		httpError(w, http.StatusBadRequest, err)
		return
	}
	if _, err := config.Save(ws.Resolved, cfg); err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}

	// Reload through the resolution chain so the workspace reflects exactly
	// what a fresh start would see.
	explicit := ""
	if ws.Resolved.Source == config.SourceFlag {
		explicit = ws.Resolved.Path
	}
	res, err := config.Resolve(ws.RepoRoot, explicit)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	s.SetWorkspace(Params{
		RepoRoot: ws.RepoRoot,
		Config:   instances.ApplyLayout(ws.RepoRoot, res),
		Resolved: res,
	})
	s.handleConfigSourcesGet(w, r)
}
