package server

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"helmdex/internal/artifacthub"
	"helmdex/internal/catalog"
	"helmdex/internal/depmeta"
	"helmdex/internal/helmutil"
	"helmdex/internal/instances"
	"helmdex/internal/paths"
	"helmdex/internal/semverutil"
	"helmdex/internal/values"
	"helmdex/internal/yamlchart"
)

// --- repo ---

type repoInfo struct {
	Root         string `json:"root"`
	OptedIn      bool   `json:"optedIn"`
	AppsDir      string `json:"appsDir"`
	TemplatesDir string `json:"templatesDir"`
	ConfigPath   string `json:"configPath"`
	ConfigSource string `json:"configSource"`
	ConfigError  string `json:"configError,omitempty"`
	Platform     string `json:"platform"`
}

func (s *Server) handleRepo(w http.ResponseWriter, r *http.Request) {
	ws := s.ws()
	writeJSON(w, http.StatusOK, repoInfo{
		Root:         ws.RepoRoot,
		OptedIn:      paths.OptedIn(ws.RepoRoot),
		AppsDir:      ws.Config.Repo.AppsDir,
		TemplatesDir: ws.Config.Repo.TemplatesDir,
		ConfigPath:   ws.Resolved.Path,
		ConfigSource: string(ws.Resolved.Source),
		ConfigError:  ws.ConfigError,
		Platform:     ws.Config.Platform.Name,
	})
}

// --- instances ---

type depInfo struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Alias      string `json:"alias,omitempty"`
	Version    string `json:"version"`
	Repository string `json:"repository"`
	// Source metadata (catalog / artifacthub / arbitrary), when recorded.
	SourceKind    string `json:"sourceKind,omitempty"`
	CatalogID     string `json:"catalogID,omitempty"`
	CatalogSource string `json:"catalogSource,omitempty"`
}

type instanceInfo struct {
	Name     string    `json:"name"`
	Path     string    `json:"path"`
	Managed  bool      `json:"managed"`
	Deps     []depInfo `json:"deps"`
	DepError string    `json:"depError,omitempty"`
}

func (s *Server) instanceInfo(inst instances.Instance) instanceInfo {
	info := instanceInfo{
		Name:    inst.Name,
		Path:    inst.Path,
		Managed: values.IsManaged(inst.Path),
		Deps:    []depInfo{},
	}
	c, err := yamlchart.ReadChart(filepath.Join(inst.Path, "Chart.yaml"))
	if err != nil {
		info.DepError = err.Error()
		return info
	}
	for _, d := range c.Dependencies {
		di := depInfo{
			ID:         string(yamlchart.DependencyID(d)),
			Name:       d.Name,
			Alias:      d.Alias,
			Version:    d.Version,
			Repository: d.Repository,
		}
		if m, ok := depmeta.Read(s.ws().RepoRoot, inst.Name, yamlchart.DependencyID(d)); ok {
			di.SourceKind = string(m.Kind)
			di.CatalogID = m.CatalogID
			di.CatalogSource = m.CatalogSource
		}
		info.Deps = append(info.Deps, di)
	}
	return info
}

func (s *Server) getInstance(name string) (instances.Instance, error) {
	return instances.Get(s.ws().RepoRoot, s.ws().Config.Repo.AppsDir, name)
}

func (s *Server) handleInstancesList(w http.ResponseWriter, r *http.Request) {
	list, err := instances.List(s.ws().RepoRoot, s.ws().Config.Repo.AppsDir)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]instanceInfo, 0, len(list))
	for _, inst := range list {
		out = append(out, s.instanceInfo(inst))
	}
	writeJSON(w, http.StatusOK, out)
}

type createInstanceRequest struct {
	Name         string `json:"name"`
	FromTemplate string `json:"fromTemplate,omitempty"`
}

func (s *Server) handleInstanceCreate(w http.ResponseWriter, r *http.Request) {
	var req createInstanceRequest
	if err := decodeJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	var inst instances.Instance
	var err error
	if req.FromTemplate != "" {
		inst, err = instances.CreateFromTemplate(s.ws().RepoRoot, s.ws().Config.Repo.AppsDir, s.ws().Config.Repo.TemplatesDir, req.FromTemplate, req.Name)
	} else {
		inst, err = instances.Create(s.ws().RepoRoot, s.ws().Config.Repo.AppsDir, req.Name, paths.OptedIn(s.ws().RepoRoot))
		if err == nil {
			err = values.GenerateIfManaged(inst.Path)
		}
	}
	if err != nil {
		httpError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, s.instanceInfo(inst))
}

func (s *Server) handleInstanceGet(w http.ResponseWriter, r *http.Request) {
	inst, err := s.getInstance(r.PathValue("name"))
	if err != nil {
		httpError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, s.instanceInfo(inst))
}

func (s *Server) handleInstanceDelete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.getInstance(name); err != nil {
		httpError(w, http.StatusNotFound, err)
		return
	}
	if err := instances.Remove(s.ws().RepoRoot, s.ws().Config.Repo.AppsDir, name); err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type renameRequest struct {
	NewName string `json:"newName"`
}

func (s *Server) handleInstanceRename(w http.ResponseWriter, r *http.Request) {
	var req renameRequest
	if err := decodeJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	inst, err := instances.Rename(s.ws().RepoRoot, s.ws().Config.Repo.AppsDir, r.PathValue("name"), req.NewName)
	if err != nil {
		httpError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, s.instanceInfo(inst))
}

type applyRequest struct {
	Relock bool `json:"relock,omitempty"`
}

func (s *Server) handleInstanceApply(w http.ResponseWriter, r *http.Request) {
	var req applyRequest
	if r.ContentLength > 0 {
		if err := decodeJSON(r, &req); err != nil {
			httpError(w, http.StatusBadRequest, err)
			return
		}
	}
	inst, err := s.getInstance(r.PathValue("name"))
	if err != nil {
		httpError(w, http.StatusNotFound, err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events.publish(event{Type: "apply.start", Instance: inst.Name})
	if err := instances.Apply(r.Context(), s.ws().RepoRoot, s.ws().Config, inst, req.Relock); err != nil {
		s.events.publish(event{Type: "apply.error", Instance: inst.Name, Message: err.Error()})
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	s.events.publish(event{Type: "apply.done", Instance: inst.Name})
	writeJSON(w, http.StatusOK, s.instanceInfo(inst))
}

// --- files ---

type fileInfo struct {
	Path  string `json:"path"`
	Size  int64  `json:"size"`
	IsDir bool   `json:"isDir"`
}

func (s *Server) handleFilesList(w http.ResponseWriter, r *http.Request) {
	inst, err := s.getInstance(r.PathValue("name"))
	if err != nil {
		httpError(w, http.StatusNotFound, err)
		return
	}
	out := []fileInfo{}
	err = filepath.WalkDir(inst.Path, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(inst.Path, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		// The vendored charts/ dir is helm-managed bulk; skip its contents.
		if d.IsDir() && rel == "charts" {
			out = append(out, fileInfo{Path: rel, IsDir: true})
			return filepath.SkipDir
		}
		fi := fileInfo{Path: rel, IsDir: d.IsDir()}
		if !d.IsDir() {
			if st, err := d.Info(); err == nil {
				fi.Size = st.Size()
			}
		}
		out = append(out, fi)
		return nil
	})
	if err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	writeJSON(w, http.StatusOK, out)
}

// resolveInstanceFile validates a relative path stays inside the instance dir.
func resolveInstanceFile(instPath, rel string) (string, error) {
	if rel == "" {
		return "", fmt.Errorf("query parameter 'path' is required")
	}
	abs := filepath.Join(instPath, filepath.Clean("/"+rel))
	if abs != instPath && !strings.HasPrefix(abs, instPath+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes the instance directory", rel)
	}
	return abs, nil
}

func (s *Server) handleFileRead(w http.ResponseWriter, r *http.Request) {
	inst, err := s.getInstance(r.PathValue("name"))
	if err != nil {
		httpError(w, http.StatusNotFound, err)
		return
	}
	p, err := resolveInstanceFile(inst.Path, r.URL.Query().Get("path"))
	if err != nil {
		httpError(w, http.StatusBadRequest, err)
		return
	}
	b, err := os.ReadFile(p)
	if err != nil {
		httpError(w, http.StatusNotFound, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write(b)
}

func (s *Server) handleFileWrite(w http.ResponseWriter, r *http.Request) {
	inst, err := s.getInstance(r.PathValue("name"))
	if err != nil {
		httpError(w, http.StatusNotFound, err)
		return
	}
	p, err := resolveInstanceFile(inst.Path, r.URL.Query().Get("path"))
	if err != nil {
		httpError(w, http.StatusBadRequest, err)
		return
	}
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 10<<20))
	if err != nil {
		httpError(w, http.StatusBadRequest, err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	if err := os.WriteFile(p, b, 0o644); err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	// Editing a managed layer invalidates the merged output.
	if err := values.GenerateIfManaged(inst.Path); err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- values ---

func (s *Server) handleValuesGet(w http.ResponseWriter, r *http.Request) {
	inst, err := s.getInstance(r.PathValue("name"))
	if err != nil {
		httpError(w, http.StatusNotFound, err)
		return
	}
	pathQ := r.URL.Query().Get("path")
	if pathQ == "" {
		pathQ = "$"
	}
	p, err := values.ParsePath(pathQ)
	if err != nil {
		httpError(w, http.StatusBadRequest, err)
		return
	}
	v, ok, err := values.GetInFile(values.EditFilePath(inst.Path), p)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"file":  values.EditFileName(inst.Path),
		"path":  pathQ,
		"found": ok,
		"value": v,
	})
}

type valuesSetRequest struct {
	Path  string `json:"path"`
	Value any    `json:"value"`
	Regen *bool  `json:"regen,omitempty"`
}

func (s *Server) handleValuesSet(w http.ResponseWriter, r *http.Request) {
	inst, err := s.getInstance(r.PathValue("name"))
	if err != nil {
		httpError(w, http.StatusNotFound, err)
		return
	}
	var req valuesSetRequest
	if err := decodeJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, err)
		return
	}
	p, err := values.ParsePath(req.Path)
	if err != nil {
		httpError(w, http.StatusBadRequest, err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := values.SetInFile(values.EditFilePath(inst.Path), p, req.Value); err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	if req.Regen == nil || *req.Regen {
		if err := values.GenerateIfManaged(inst.Path); err != nil {
			httpError(w, http.StatusInternalServerError, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleValuesRegen(w http.ResponseWriter, r *http.Request) {
	inst, err := s.getInstance(r.PathValue("name"))
	if err != nil {
		httpError(w, http.StatusNotFound, err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := values.GenerateIfManaged(inst.Path); err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- deps ---

func (s *Server) handleDepsList(w http.ResponseWriter, r *http.Request) {
	inst, err := s.getInstance(r.PathValue("name"))
	if err != nil {
		httpError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, s.instanceInfo(inst).Deps)
}

type depAddRequest struct {
	Name       string `json:"name"`
	Repository string `json:"repository"`
	Version    string `json:"version"`
	Alias      string `json:"alias,omitempty"`
	// Source attribution recorded in depmeta ("catalog" | "artifacthub" |
	// "arbitrary"; empty defaults to arbitrary).
	SourceKind    string `json:"sourceKind,omitempty"`
	CatalogID     string `json:"catalogID,omitempty"`
	CatalogSource string `json:"catalogSource,omitempty"`
}

func (s *Server) handleDepAdd(w http.ResponseWriter, r *http.Request) {
	inst, err := s.getInstance(r.PathValue("name"))
	if err != nil {
		httpError(w, http.StatusNotFound, err)
		return
	}
	var req depAddRequest
	if err := decodeJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	chartPath := filepath.Join(inst.Path, "Chart.yaml")
	c, err := yamlchart.ReadChart(chartPath)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	dep := yamlchart.Dependency{Name: req.Name, Repository: req.Repository, Version: req.Version, Alias: req.Alias}
	if err := c.UpsertDependency(dep); err != nil {
		httpError(w, http.StatusBadRequest, err)
		return
	}
	if err := yamlchart.WriteChart(chartPath, c); err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	kind := depmeta.Kind(req.SourceKind)
	switch kind {
	case depmeta.KindCatalog, depmeta.KindArtifactHub:
	default:
		kind = depmeta.KindArbitrary
	}
	if err := depmeta.Write(s.ws().RepoRoot, inst.Name, yamlchart.DependencyID(dep), depmeta.Meta{
		Kind:          kind,
		CatalogID:     req.CatalogID,
		CatalogSource: req.CatalogSource,
	}); err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, s.instanceInfo(inst))
}

func (s *Server) handleDepDetach(w http.ResponseWriter, r *http.Request) {
	inst, err := s.getInstance(r.PathValue("name"))
	if err != nil {
		httpError(w, http.StatusNotFound, err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id := yamlchart.DepID(r.PathValue("depID"))
	m, ok := depmeta.Read(s.ws().RepoRoot, inst.Name, id)
	if !ok || m.Kind != depmeta.KindCatalog {
		httpError(w, http.StatusBadRequest, fmt.Errorf("dependency %q is not catalog-attached", id))
		return
	}
	if err := depmeta.Write(s.ws().RepoRoot, inst.Name, id, depmeta.Meta{Kind: depmeta.KindArbitrary}); err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, s.instanceInfo(inst))
}

func (s *Server) handleDepRemove(w http.ResponseWriter, r *http.Request) {
	inst, err := s.getInstance(r.PathValue("name"))
	if err != nil {
		httpError(w, http.StatusNotFound, err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	chartPath := filepath.Join(inst.Path, "Chart.yaml")
	c, err := yamlchart.ReadChart(chartPath)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	id := yamlchart.DepID(r.PathValue("depID"))
	if ok := c.RemoveDependencyByID(id); !ok {
		httpError(w, http.StatusNotFound, fmt.Errorf("dependency %q not found", id))
		return
	}
	if err := yamlchart.WriteChart(chartPath, c); err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type depSetVersionRequest struct {
	Version  string `json:"version"`
	Validate bool   `json:"validate,omitempty"`
}

func (s *Server) handleDepSetVersion(w http.ResponseWriter, r *http.Request) {
	inst, err := s.getInstance(r.PathValue("name"))
	if err != nil {
		httpError(w, http.StatusNotFound, err)
		return
	}
	var req depSetVersionRequest
	if err := decodeJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.Version) == "" {
		httpError(w, http.StatusBadRequest, fmt.Errorf("version is required"))
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	chartPath := filepath.Join(inst.Path, "Chart.yaml")
	c, err := yamlchart.ReadChart(chartPath)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	id := yamlchart.DepID(r.PathValue("depID"))
	found := false
	for i := range c.Dependencies {
		if yamlchart.DependencyID(c.Dependencies[i]) != id {
			continue
		}
		if req.Validate && !strings.HasPrefix(c.Dependencies[i].Repository, "oci://") {
			env := helmutil.EnvForRepoURL(s.ws().RepoRoot, c.Dependencies[i].Repository)
			repoName := helmutil.RepoNameForURL(c.Dependencies[i].Repository)
			ref := repoName + "/" + c.Dependencies[i].Name
			if err := helmutil.RepoAdd(r.Context(), env, repoName, c.Dependencies[i].Repository); err != nil {
				httpError(w, http.StatusBadGateway, err)
				return
			}
			if _, err := helmutil.ShowChart(r.Context(), env, ref, req.Version); err != nil {
				httpError(w, http.StatusBadRequest, fmt.Errorf("invalid version %q for %s: %w", req.Version, id, err))
				return
			}
		}
		c.Dependencies[i].Version = req.Version
		found = true
		break
	}
	if !found {
		httpError(w, http.StatusNotFound, fmt.Errorf("dependency %q not found", id))
		return
	}
	if err := yamlchart.WriteChart(chartPath, c); err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, s.instanceInfo(inst))
}

// Per-dependency overrides live under $.<depID> of the instance edit file.
// The depID is used as a literal map key via Path.Child — never routed
// through ParsePath, which would mis-split a dotted/bracketed dep id.

func (s *Server) handleDepValuesGet(w http.ResponseWriter, r *http.Request) {
	inst, err := s.getInstance(r.PathValue("name"))
	if err != nil {
		httpError(w, http.StatusNotFound, err)
		return
	}
	depID := r.PathValue("depID")
	v, ok, err := values.GetInFile(values.EditFilePath(inst.Path), values.Path{}.Child(depID))
	if err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"file":  values.EditFileName(inst.Path),
		"depID": depID,
		"found": ok,
		"value": v,
	})
}

type depValuesSetRequest struct {
	Value any   `json:"value"`
	Regen *bool `json:"regen,omitempty"`
}

func (s *Server) handleDepValuesSet(w http.ResponseWriter, r *http.Request) {
	inst, err := s.getInstance(r.PathValue("name"))
	if err != nil {
		httpError(w, http.StatusNotFound, err)
		return
	}
	var req depValuesSetRequest
	if err := decodeJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, err)
		return
	}
	depID := r.PathValue("depID")
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := values.SetInFile(values.EditFilePath(inst.Path), values.Path{}.Child(depID), req.Value); err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	if req.Regen == nil || *req.Regen {
		if err := values.GenerateIfManaged(inst.Path); err != nil {
			httpError(w, http.StatusInternalServerError, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDepVersions(w http.ResponseWriter, r *http.Request) {
	inst, err := s.getInstance(r.PathValue("name"))
	if err != nil {
		httpError(w, http.StatusNotFound, err)
		return
	}
	c, err := yamlchart.ReadChart(filepath.Join(inst.Path, "Chart.yaml"))
	if err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	dep, err := instances.DepByID(c, r.PathValue("depID"))
	if err != nil {
		httpError(w, http.StatusNotFound, err)
		return
	}
	if strings.HasPrefix(dep.Repository, "oci://") {
		httpError(w, http.StatusBadRequest, fmt.Errorf("version listing is not supported for OCI repositories"))
		return
	}
	vs, err := helmutil.RepoChartVersions(r.Context(), s.ws().RepoRoot, dep.Repository, dep.Name, 24*time.Hour)
	if err != nil {
		httpError(w, http.StatusBadGateway, err)
		return
	}
	best, _ := semverutil.BestStable(vs)
	writeJSON(w, http.StatusOK, map[string]any{
		"current":    dep.Version,
		"versions":   vs,
		"bestStable": best,
	})
}

func (s *Server) handleDepInspect(w http.ResponseWriter, r *http.Request) {
	inst, err := s.getInstance(r.PathValue("name"))
	if err != nil {
		httpError(w, http.StatusNotFound, err)
		return
	}
	kind := instances.InspectKind(r.URL.Query().Get("kind"))
	switch kind {
	case instances.InspectReadme, instances.InspectValues, instances.InspectSchema:
	default:
		httpError(w, http.StatusBadRequest, fmt.Errorf("kind must be readme|values|schema"))
		return
	}
	c, err := yamlchart.ReadChart(filepath.Join(inst.Path, "Chart.yaml"))
	if err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	dep, err := instances.DepByID(c, r.PathValue("depID"))
	if err != nil {
		httpError(w, http.StatusNotFound, err)
		return
	}
	// Optional version override — lets the UI diff artifacts between the
	// current and a candidate version.
	if v := r.URL.Query().Get("version"); v != "" {
		dep.Version = v
	}
	content, err := instances.LoadDepInspectContent(r.Context(), s.ws().RepoRoot, inst.Path, dep, kind)
	if err != nil {
		httpError(w, http.StatusBadGateway, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(content))
}

// --- templates ---

func (s *Server) handleTemplatesList(w http.ResponseWriter, r *http.Request) {
	list, err := instances.ListTemplates(s.ws().RepoRoot, s.ws().Config.Repo.TemplatesDir)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]map[string]string, 0, len(list))
	for _, t := range list {
		out = append(out, map[string]string{"name": t.Name, "path": t.Path})
	}
	writeJSON(w, http.StatusOK, out)
}

// --- catalog / artifact hub ---

func (s *Server) handleCatalogList(w http.ResponseWriter, r *http.Request) {
	entries, err := catalog.LoadLocalCatalogEntriesWithSource(s.ws().RepoRoot)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) handleCatalogSync(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events.publish(event{Type: "catalog.sync.start"})
	res, err := catalog.NewSyncer(s.ws().RepoRoot).Sync(r.Context(), s.ws().Config)
	if err != nil {
		s.events.publish(event{Type: "catalog.sync.error", Message: err.Error()})
		httpError(w, http.StatusBadGateway, err)
		return
	}
	s.events.publish(event{Type: "catalog.sync.done"})
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleArtifactHubSearch(w http.ResponseWriter, r *http.Request) {
	if !s.ws().Config.ArtifactHubEnabled() {
		httpError(w, http.StatusForbidden, fmt.Errorf("artifact hub is disabled in config"))
		return
	}
	q := r.URL.Query().Get("q")
	limit := 25
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 100 {
			httpError(w, http.StatusBadRequest, fmt.Errorf("limit must be an integer between 1 and 100"))
			return
		}
		limit = n
	}
	res, err := artifacthub.NewClient().SearchHelm(r.Context(), q, limit)
	if err != nil {
		httpError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
