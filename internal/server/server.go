// Package server exposes helmdex operations over a local HTTP API and serves
// the embedded web UI. It is the backend for `helmdex ui` and the desktop app.
package server

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"sync"

	"helmdex/internal/config"
)

// Params configures a Server for one repo workspace.
type Params struct {
	RepoRoot string
	Config   config.Config
	Resolved config.Resolved
}

type Server struct {
	// paramsMu guards params — the desktop app swaps the workspace at
	// runtime via SetWorkspace.
	paramsMu sync.RWMutex
	params   Params

	// mu serializes mutating operations (chart/values writes, applies) —
	// they are file-level read-modify-write sequences.
	mu sync.Mutex

	events *eventBroker
	mux    *http.ServeMux
}

func New(p Params) *Server {
	s := &Server{
		params: p,
		events: newEventBroker(),
		mux:    http.NewServeMux(),
	}
	s.routes()
	return s
}

// ws returns the current workspace params.
func (s *Server) ws() Params {
	s.paramsMu.RLock()
	defer s.paramsMu.RUnlock()
	return s.params
}

// Workspace returns the current workspace params.
func (s *Server) Workspace() Params {
	return s.ws()
}

// SetWorkspace switches the served repo (used by the desktop app's
// "open repository" flow).
func (s *Server) SetWorkspace(p Params) {
	s.paramsMu.Lock()
	s.params = p
	s.paramsMu.Unlock()
	s.events.publish(event{Type: "workspace.changed", Message: p.RepoRoot})
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	m := s.mux

	m.HandleFunc("GET /api/repo", s.handleRepo)

	m.HandleFunc("GET /api/instances", s.handleInstancesList)
	m.HandleFunc("POST /api/instances", s.handleInstanceCreate)
	m.HandleFunc("GET /api/instances/{name}", s.handleInstanceGet)
	m.HandleFunc("DELETE /api/instances/{name}", s.handleInstanceDelete)
	m.HandleFunc("POST /api/instances/{name}/rename", s.handleInstanceRename)
	m.HandleFunc("POST /api/instances/{name}/apply", s.handleInstanceApply)

	m.HandleFunc("GET /api/instances/{name}/files", s.handleFilesList)
	m.HandleFunc("GET /api/instances/{name}/file", s.handleFileRead)
	m.HandleFunc("PUT /api/instances/{name}/file", s.handleFileWrite)

	m.HandleFunc("GET /api/instances/{name}/values", s.handleValuesGet)
	m.HandleFunc("PUT /api/instances/{name}/values", s.handleValuesSet)
	m.HandleFunc("POST /api/instances/{name}/values/regen", s.handleValuesRegen)

	m.HandleFunc("GET /api/instances/{name}/deps", s.handleDepsList)
	m.HandleFunc("POST /api/instances/{name}/deps", s.handleDepAdd)
	m.HandleFunc("DELETE /api/instances/{name}/deps/{depID}", s.handleDepRemove)
	m.HandleFunc("POST /api/instances/{name}/deps/{depID}/detach", s.handleDepDetach)
	m.HandleFunc("POST /api/instances/{name}/deps/{depID}/version", s.handleDepSetVersion)
	m.HandleFunc("GET /api/instances/{name}/deps/{depID}/versions", s.handleDepVersions)
	m.HandleFunc("GET /api/instances/{name}/deps/{depID}/inspect", s.handleDepInspect)

	m.HandleFunc("GET /api/instances/{name}/sets", s.handleSetsList)
	m.HandleFunc("POST /api/instances/{name}/sets", s.handleSetEnable)
	m.HandleFunc("DELETE /api/instances/{name}/sets", s.handleSetDisable)

	m.HandleFunc("GET /api/templates", s.handleTemplatesList)

	m.HandleFunc("GET /api/config/sources", s.handleConfigSourcesGet)
	m.HandleFunc("PUT /api/config/sources", s.handleConfigSourcesPut)

	m.HandleFunc("GET /api/catalog", s.handleCatalogList)
	m.HandleFunc("POST /api/catalog/sync", s.handleCatalogSync)
	m.HandleFunc("GET /api/artifacthub/search", s.handleArtifactHubSearch)

	m.HandleFunc("GET /api/events", s.handleEvents)

	m.Handle("/", spaHandler())
}

// spaHandler serves the embedded SPA, falling back to index.html for
// client-side routes.
func spaHandler() http.Handler {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		// The embed is part of the binary; failure here is a build defect.
		panic(fmt.Sprintf("embedded static assets missing: %v", err))
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p != "" {
			if _, err := fs.Stat(sub, p); err == nil {
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		r2 := *r
		r2.URL.Path = "/"
		fileServer.ServeHTTP(w, &r2)
	})
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type apiError struct {
	Error string `json:"error"`
}

func httpError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, apiError{Error: err.Error()})
}

func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("invalid request body: %w", err)
	}
	return nil
}
