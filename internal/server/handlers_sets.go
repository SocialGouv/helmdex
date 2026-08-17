package server

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"helmdex/internal/values"
)

// Sets are a managed-mode concept: marker files select which preset layers
// get imported on apply (values.set.<set>.yaml globally,
// values.dep-set.<depID>--<set>.yaml per dependency).

type setsInfo struct {
	Managed bool                `json:"managed"`
	Sets    []string            `json:"sets"`
	DepSets map[string][]string `json:"depSets"`
}

func (s *Server) handleSetsList(w http.ResponseWriter, r *http.Request) {
	inst, err := s.getInstance(r.PathValue("name"))
	if err != nil {
		httpError(w, http.StatusNotFound, err)
		return
	}
	info := setsInfo{Managed: values.IsManaged(inst.Path), Sets: []string{}, DepSets: map[string][]string{}}

	setFiles, _ := filepath.Glob(filepath.Join(inst.Path, "values.set.*.yaml"))
	for _, p := range setFiles {
		name := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(p), "values.set."), ".yaml")
		if name != "" {
			info.Sets = append(info.Sets, name)
		}
	}
	sort.Strings(info.Sets)

	depSetFiles, _ := filepath.Glob(filepath.Join(inst.Path, "values.dep-set.*--*.yaml"))
	for _, p := range depSetFiles {
		name := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(p), "values.dep-set."), ".yaml")
		parts := strings.SplitN(name, "--", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			continue
		}
		info.DepSets[parts[0]] = append(info.DepSets[parts[0]], parts[1])
	}
	for k := range info.DepSets {
		sort.Strings(info.DepSets[k])
	}

	writeJSON(w, http.StatusOK, info)
}

type setRequest struct {
	Set string `json:"set"`
	// DepID scopes the marker to one dependency (values.dep-set.<dep>--<set>);
	// empty targets the global values.set.<set> marker.
	DepID string `json:"depID,omitempty"`
}

func setMarkerPath(instPath string, req setRequest) (string, error) {
	set := strings.TrimSpace(req.Set)
	if set == "" || strings.ContainsAny(set, "/\\") {
		return "", fmt.Errorf("invalid set name %q", req.Set)
	}
	dep := strings.TrimSpace(req.DepID)
	if strings.ContainsAny(dep, "/\\") {
		return "", fmt.Errorf("invalid depID %q", req.DepID)
	}
	if dep != "" {
		return filepath.Join(instPath, fmt.Sprintf("values.dep-set.%s--%s.yaml", dep, set)), nil
	}
	return filepath.Join(instPath, fmt.Sprintf("values.set.%s.yaml", set)), nil
}

func (s *Server) handleSetEnable(w http.ResponseWriter, r *http.Request) {
	inst, err := s.getInstance(r.PathValue("name"))
	if err != nil {
		httpError(w, http.StatusNotFound, err)
		return
	}
	if !values.IsManaged(inst.Path) {
		httpError(w, http.StatusBadRequest, fmt.Errorf("sets are a managed-mode feature; this instance is direct-mode"))
		return
	}
	var req setRequest
	if err := decodeJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, err)
		return
	}
	p, err := setMarkerPath(inst.Path, req)
	if err != nil {
		httpError(w, http.StatusBadRequest, err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := os.Stat(p); err == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := os.WriteFile(p, []byte("{}\n"), 0o644); err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) handleSetDisable(w http.ResponseWriter, r *http.Request) {
	inst, err := s.getInstance(r.PathValue("name"))
	if err != nil {
		httpError(w, http.StatusNotFound, err)
		return
	}
	var req setRequest
	if err := decodeJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, err)
		return
	}
	p, err := setMarkerPath(inst.Path, req)
	if err != nil {
		httpError(w, http.StatusBadRequest, err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	if err := values.GenerateIfManaged(inst.Path); err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
