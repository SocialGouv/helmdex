//go:build desktop

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"helmdex/internal/config"
	"helmdex/internal/instances"
	"helmdex/internal/server"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the Wails-bound application object. It owns the in-process API
// server and the "which repo is open" state.
type App struct {
	ctx    context.Context
	server *server.Server
	stop   func()
}

func NewApp() *App {
	// Start with the last opened repo (falling back to cwd when it still
	// looks like a repo) so relaunching lands where the user left off.
	root := loadLastRepo()
	if root == "" {
		if cwd, err := os.Getwd(); err == nil {
			root = cwd
		}
	}
	srv := server.New(workspaceParams(root))
	a := &App{server: srv}
	a.stop = srv.StartHelmEventForwarding()
	return a
}

func (a *App) Handler() http.Handler {
	return a.server.Handler()
}

func (a *App) onStartup(ctx context.Context) {
	a.ctx = ctx
}

// workspaceParams resolves config + layout for a repo root.
func workspaceParams(root string) server.Params {
	res, err := config.Resolve(root, "")
	if err != nil {
		// Surface the bad config through the API rather than crashing the
		// GUI at boot: fall back to defaults, the UI shows the repo state.
		res = config.Resolved{Config: config.DefaultConfig(), Source: config.SourceDefault}
	}
	return server.Params{
		RepoRoot: root,
		Config:   instances.ApplyLayout(root, res),
		Resolved: res,
	}
}

// OpenRepoDialog shows a native directory picker and switches the served
// workspace. Returns the chosen root ("" when cancelled).
func (a *App) OpenRepoDialog() (string, error) {
	dir, err := wruntime.OpenDirectoryDialog(a.ctx, wruntime.OpenDialogOptions{
		Title: "Open repository",
	})
	if err != nil {
		return "", err
	}
	if dir == "" {
		return "", nil
	}
	a.server.SetWorkspace(workspaceParams(dir))
	saveLastRepo(dir)
	return dir, nil
}

// CurrentRepo returns the repo root currently served.
func (a *App) CurrentRepo() string {
	return a.server.Workspace().RepoRoot
}

// --- last-repo persistence (~/.config/helmdex/desktop.json) ---

type desktopState struct {
	LastRepo string `json:"lastRepo"`
}

func desktopStatePath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(base, "helmdex", "desktop.json"), nil
}

func loadLastRepo() string {
	p, err := desktopStatePath()
	if err != nil {
		return ""
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	var st desktopState
	if err := json.Unmarshal(b, &st); err != nil {
		return ""
	}
	if st.LastRepo == "" {
		return ""
	}
	if _, err := os.Stat(st.LastRepo); err != nil {
		return ""
	}
	return st.LastRepo
}

func saveLastRepo(root string) {
	p, err := desktopStatePath()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	b, _ := json.Marshal(desktopState{LastRepo: root})
	_ = os.WriteFile(p, b, 0o644)
}
