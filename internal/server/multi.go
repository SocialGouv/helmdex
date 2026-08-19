package server

import (
	"fmt"
	"net/http"
	"path/filepath"
	"sync"
)

// Multi serves several isolated workspaces side by side — one Server per
// open folder, each with its own mutation lock and event broker. It is the
// backend of the desktop app's folder tabs. Requests are routed by prefix:
//
//	/ws/{id}/api/...  → that workspace's Server (prefix stripped)
//	/                 → the embedded SPA
//
// Workspace ids are process-local ("w1", "w2", …) and never reused.
type Multi struct {
	mu      sync.RWMutex
	seq     int
	servers map[string]*Server // id → server
	order   []string           // ids in open order
	roots   map[string]string  // cleaned root → id (dedup)
	mux     *http.ServeMux
}

// WorkspaceDesc identifies one open workspace.
type WorkspaceDesc struct {
	ID   string
	Root string
}

func NewMulti() *Multi {
	m := &Multi{
		servers: map[string]*Server{},
		roots:   map[string]string{},
		mux:     http.NewServeMux(),
	}
	m.mux.HandleFunc("/ws/{id}/", m.dispatch)
	// Workspace-less API calls are a caller bug; answer explicitly instead of
	// letting the SPA fallback serve index.html for them.
	m.mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		httpError(w, http.StatusNotFound,
			fmt.Errorf("the API is workspace-scoped here: use /ws/<id>%s", r.URL.Path))
	})
	m.mux.Handle("/", spaHandler())
	return m
}

// canonicalRoot is the dedup key for a workspace root: absolute, cleaned,
// symlinks resolved. Two aliases of one physical folder must map to ONE
// workspace — separate workspaces would hold separate write locks over the
// same files. Falls back progressively when resolution is impossible.
func canonicalRoot(root string) string {
	key := root
	if abs, err := filepath.Abs(root); err == nil {
		key = abs
	}
	key = filepath.Clean(key)
	if resolved, err := filepath.EvalSymlinks(key); err == nil {
		return resolved
	}
	return key
}

// Open adds a workspace for p.RepoRoot and returns its id. Opening a root
// that is already open (under any alias of the same folder) returns the
// existing workspace untouched.
func (m *Multi) Open(p Params) (id string, existed bool) {
	key := canonicalRoot(p.RepoRoot)

	m.mu.Lock()
	defer m.mu.Unlock()
	if id, ok := m.roots[key]; ok {
		return id, true
	}
	m.seq++
	id = fmt.Sprintf("w%d", m.seq)
	m.servers[id] = New(p)
	m.roots[key] = id
	m.order = append(m.order, id)
	return id, false
}

// Close removes a workspace. In-flight requests holding its Server finish
// normally; new requests to its prefix get a 404.
func (m *Multi) Close(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	srv, ok := m.servers[id]
	if !ok {
		return fmt.Errorf("unknown workspace %q", id)
	}
	delete(m.servers, id)
	delete(m.roots, canonicalRoot(srv.ws().RepoRoot))
	for i, v := range m.order {
		if v == id {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
	return nil
}

// List returns the open workspaces in open order.
func (m *Multi) List() []WorkspaceDesc {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]WorkspaceDesc, 0, len(m.order))
	for _, id := range m.order {
		out = append(out, WorkspaceDesc{ID: id, Root: m.servers[id].ws().RepoRoot})
	}
	return out
}

func (m *Multi) Handler() http.Handler {
	return m.mux
}

func (m *Multi) dispatch(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	m.mu.RLock()
	srv, ok := m.servers[id]
	m.mu.RUnlock()
	if !ok {
		httpError(w, http.StatusNotFound, fmt.Errorf("unknown workspace %q", id))
		return
	}
	http.StripPrefix("/ws/"+id, srv.Handler()).ServeHTTP(w, r)
}

// StartHelmEventForwarding wires bundled-helm download events into every open
// workspace's broker (downloads are process-global, so each tab should see
// their progress). Call once per process — the sink is global. Returns a
// stop func.
func (m *Multi) StartHelmEventForwarding() func() {
	return forwardHelmEvents(m.publishAll)
}

func (m *Multi) publishAll(ev event) {
	m.mu.RLock()
	servers := make([]*Server, 0, len(m.servers))
	for _, srv := range m.servers {
		servers = append(servers, srv)
	}
	m.mu.RUnlock()
	// Publish outside the lock: a broker never blocks, but Close must not
	// have to wait on fan-out either.
	for _, srv := range servers {
		srv.events.publish(ev)
	}
}
