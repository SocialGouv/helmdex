//go:build desktop

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"helmdex/internal/config"
	"helmdex/internal/desktopstate"
	"helmdex/internal/instances"
	"helmdex/internal/server"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the Wails-bound application object. It owns the multi-workspace
// API server (one isolated server.Server per open folder) and the persisted
// folder list / window state.
type App struct {
	ctx  context.Context
	mgr  *server.Multi
	stop func()

	// mu guards state — bindings and the close hook mutate it.
	mu    sync.Mutex
	state desktopstate.State
}

// NewApp restores the persisted folder list (dropping folders that no longer
// exist) and opens a workspace per folder. initialDir, when non-empty, is a
// normalized directory from argv that gets opened and activated.
func NewApp(initialDir string) *App {
	st, err := desktopstate.Load()
	if err != nil {
		// A corrupt prefs file must not brick the GUI; start clean but say so.
		log.Printf("helmdex-desktop: ignoring desktop state: %v", err)
		st = desktopstate.State{}
	}

	kept := make([]string, 0, len(st.Repos))
	for _, root := range st.Repos {
		if _, err := os.Stat(root); err != nil {
			log.Printf("helmdex-desktop: dropping vanished folder %s", root)
			if st.Active == root {
				st.Active = ""
			}
			continue
		}
		kept = append(kept, root)
	}
	st.Repos = kept
	if st.Active == "" && len(st.Repos) > 0 {
		st.Active = st.Repos[0]
	}

	if initialDir != "" {
		if st, err = desktopstate.AddRepo(st, initialDir); err != nil {
			log.Printf("helmdex-desktop: cannot open %s: %v", initialDir, err)
		}
	}
	if len(st.Repos) == 0 {
		// First launch: open the working directory so `helmdex-desktop` from a
		// project folder just works — but not for launcher-style starts, whose
		// cwd ($HOME, /) is meaningless as a repo. Those get the Welcome screen.
		if cwd, err := os.Getwd(); err == nil && !isGenericLaunchDir(cwd) {
			if st, err = desktopstate.AddRepo(st, cwd); err != nil {
				log.Printf("helmdex-desktop: cannot open cwd: %v", err)
			}
		}
	}

	mgr := server.NewMulti()
	for _, root := range st.Repos {
		mgr.Open(workspaceParams(root))
	}

	a := &App{mgr: mgr, state: st}
	a.stop = mgr.StartHelmEventForwarding()
	a.persistLocked()
	return a
}

// isGenericLaunchDir reports whether dir is a launcher-style cwd ($HOME or a
// filesystem root) rather than a deliberately chosen project folder.
func isGenericLaunchDir(dir string) bool {
	clean := filepath.Clean(dir)
	if clean == filepath.VolumeName(clean)+string(filepath.Separator) {
		return true
	}
	home, err := os.UserHomeDir()
	return err == nil && clean == filepath.Clean(home)
}

func (a *App) Handler() http.Handler {
	return a.mgr.Handler()
}

func (a *App) onStartup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) onShutdown(ctx context.Context) {
	a.stop()
}

// workspaceParams resolves config + layout for a repo root.
func workspaceParams(root string) server.Params {
	res, err := config.Resolve(root, "")
	if err != nil {
		// Don't crash the GUI on a malformed config; fall back to defaults
		// but keep the error so the API can surface it (otherwise a broken
		// config is indistinguishable from no config).
		return server.Params{
			RepoRoot:    root,
			Config:      instances.ApplyLayout(root, config.Resolved{Config: config.DefaultConfig(), Source: config.SourceDefault}),
			Resolved:    config.Resolved{Config: config.DefaultConfig(), Source: config.SourceDefault},
			ConfigError: err.Error(),
		}
	}
	return server.Params{
		RepoRoot: root,
		Config:   instances.ApplyLayout(root, res),
		Resolved: res,
	}
}

// --- workspace bindings (window.go.main.App.*) ---

// WorkspaceInfo describes one open folder tab.
type WorkspaceInfo struct {
	ID   string `json:"id"`
	Root string `json:"root"`
	Name string `json:"name"`
}

// WorkspacesState is the full tab state; every mutating binding returns it so
// the frontend just mirrors it.
type WorkspacesState struct {
	Workspaces []WorkspaceInfo `json:"workspaces"`
	ActiveID   string          `json:"activeId"`
}

// Workspaces returns the current tab state.
func (a *App) Workspaces() WorkspacesState {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.snapshotLocked()
}

// OpenRepoDialog shows a native directory picker, opens a workspace for the
// chosen folder (or activates it when already open). Cancelling returns the
// state unchanged.
func (a *App) OpenRepoDialog() (WorkspacesState, error) {
	dir, err := wruntime.OpenDirectoryDialog(a.ctx, wruntime.OpenDialogOptions{
		Title: "Open folder",
	})
	if err != nil {
		return WorkspacesState{}, err
	}
	if dir == "" {
		return a.Workspaces(), nil
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	st, err := desktopstate.AddRepo(a.state, dir)
	if err != nil {
		return WorkspacesState{}, err
	}
	a.state = st
	a.mgr.Open(workspaceParams(a.state.Active))
	a.persistLocked()
	a.refreshTitleLocked()
	return a.snapshotLocked(), nil
}

// SetActiveWorkspace makes the given tab active (window title + persistence).
func (a *App) SetActiveWorkspace(id string) (WorkspacesState, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	root, ok := a.rootByIDLocked(id)
	if !ok {
		return WorkspacesState{}, fmt.Errorf("unknown workspace %q", id)
	}
	a.state.Active = root
	a.persistLocked()
	a.refreshTitleLocked()
	return a.snapshotLocked(), nil
}

// CloseWorkspace closes a tab. Closing the active one activates its neighbor.
func (a *App) CloseWorkspace(id string) (WorkspacesState, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	root, ok := a.rootByIDLocked(id)
	if !ok {
		return WorkspacesState{}, fmt.Errorf("unknown workspace %q", id)
	}
	if err := a.mgr.Close(id); err != nil {
		return WorkspacesState{}, err
	}
	a.state = desktopstate.RemoveRepo(a.state, root)
	a.persistLocked()
	a.refreshTitleLocked()
	return a.snapshotLocked(), nil
}

func (a *App) rootByIDLocked(id string) (string, bool) {
	for _, d := range a.mgr.List() {
		if d.ID == id {
			return d.Root, true
		}
	}
	return "", false
}

// snapshotLocked derives the binding-facing state: rail order comes from the
// persisted folder list, ids from the workspace manager.
func (a *App) snapshotLocked() WorkspacesState {
	idByRoot := map[string]string{}
	for _, d := range a.mgr.List() {
		idByRoot[filepath.Clean(d.Root)] = d.ID
	}
	out := WorkspacesState{Workspaces: []WorkspaceInfo{}}
	for _, root := range a.state.Repos {
		id, ok := idByRoot[filepath.Clean(root)]
		if !ok {
			// State and manager are mutated together under mu; a mismatch is
			// a programming error worth seeing, not hiding.
			log.Printf("helmdex-desktop: folder %s has no workspace", root)
			continue
		}
		out.Workspaces = append(out.Workspaces, WorkspaceInfo{
			ID:   id,
			Root: root,
			Name: filepath.Base(root),
		})
		if root == a.state.Active {
			out.ActiveID = id
		}
	}
	return out
}

func (a *App) persistLocked() {
	if err := desktopstate.Save(a.state); err != nil {
		// Persistence failure must not block the running session.
		log.Printf("helmdex-desktop: %v", err)
	}
}

// --- window title & geometry ---

func windowTitle(activeRoot string) string {
	if activeRoot == "" {
		return "Helmdex"
	}
	return filepath.Base(activeRoot) + " — Helmdex"
}

// InitialTitle is the window title at launch (before the runtime is up).
func (a *App) InitialTitle() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return windowTitle(a.state.Active)
}

func (a *App) refreshTitleLocked() {
	if a.ctx == nil {
		return
	}
	wruntime.WindowSetTitle(a.ctx, windowTitle(a.state.Active))
}

// initialWindow returns the persisted window geometry, with defaults.
func (a *App) initialWindow() (width, height int, maximized bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	width, height = 1400, 900
	if w := a.state.Window; w != nil {
		if w.Width > 0 && w.Height > 0 {
			width, height = w.Width, w.Height
		}
		maximized = w.Maximized
	}
	return width, height, maximized
}

// onBeforeClose persists the window geometry; it never prevents closing.
func (a *App) onBeforeClose(ctx context.Context) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	win := a.state.Window
	if win == nil {
		win = &desktopstate.Window{}
		a.state.Window = win
	}
	win.Maximized = wruntime.WindowIsMaximised(ctx)
	if !win.Maximized {
		if w, h := wruntime.WindowGetSize(ctx); w > 0 && h > 0 {
			win.Width, win.Height = w, h
		}
	}
	a.persistLocked()
	return false
}
