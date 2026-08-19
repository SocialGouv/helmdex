// Package desktopstate persists the desktop app's cross-launch state
// (open folders, active folder, window geometry) in the user config dir.
// It is Wails-free so the logic stays testable with plain `go test`.
package desktopstate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Window is the persisted window geometry.
type Window struct {
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	Maximized bool `json:"maximized"`
}

// State is the content of desktop.json.
type State struct {
	// Repos are the open folders, in rail order.
	Repos []string `json:"repos"`
	// Active is the active folder (one of Repos, or "" when none).
	Active string  `json:"active,omitempty"`
	Window *Window `json:"window,omitempty"`

	// LastRepo is the pre-multi-folder field; Load migrates it into Repos.
	LastRepo string `json:"lastRepo,omitempty"`
}

// Path returns the state file location (~/.config/helmdex/desktop.json).
func Path() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(base, "helmdex", "desktop.json"), nil
}

// Load reads the state file. A missing file yields a zero State; a malformed
// one is an error so the caller can log it instead of silently dropping the
// user's folder list.
func Load() (State, error) {
	p, err := Path()
	if err != nil {
		return State{}, err
	}
	return loadFrom(p)
}

func loadFrom(path string) (State, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return State{}, nil
	}
	if err != nil {
		return State{}, fmt.Errorf("read desktop state %s: %w", path, err)
	}
	var st State
	if err := json.Unmarshal(b, &st); err != nil {
		return State{}, fmt.Errorf("parse desktop state %s: %w", path, err)
	}
	st.migrate()
	return st, nil
}

// migrate folds the legacy single-repo field into the folder list and
// re-normalizes every entry: older versions (and hand edits) may have stored
// non-canonical paths, which would otherwise dedup as distinct folders.
func (st *State) migrate() {
	if st.LastRepo != "" && len(st.Repos) == 0 {
		st.Repos = []string{st.LastRepo}
		st.Active = st.LastRepo
	}
	st.LastRepo = ""
	repos := make([]string, 0, len(st.Repos))
	for _, r := range st.Repos {
		norm, err := Normalize(r)
		if err != nil {
			norm = r
		}
		if !contains(repos, norm) {
			repos = append(repos, norm)
		}
	}
	st.Repos = repos
	if st.Active != "" {
		if norm, err := Normalize(st.Active); err == nil {
			st.Active = norm
		}
	}
	if st.Active != "" && !contains(st.Repos, st.Active) {
		st.Active = ""
	}
	if st.Active == "" && len(st.Repos) > 0 {
		st.Active = st.Repos[0]
	}
}

// Save writes the state file, creating the directory as needed.
func Save(st State) error {
	p, err := Path()
	if err != nil {
		return err
	}
	return saveTo(p, st)
}

func saveTo(path string, st State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create desktop state dir: %w", err)
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("encode desktop state: %w", err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return fmt.Errorf("write desktop state %s: %w", path, err)
	}
	return nil
}

// Normalize returns the canonical form of a folder path — absolute, cleaned,
// symlinks resolved — the dedup key across the state and the workspace
// manager. Without symlink resolution, one physical repo opened via two
// aliases would get two workspaces (two independent write locks on the same
// files). Falls back to the cleaned absolute path when the path does not
// exist (resolution is impossible then).
func Normalize(root string) (string, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", root, err)
	}
	abs = filepath.Clean(abs)
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved, nil
	}
	return abs, nil
}

// AddRepo appends root (normalized) to the folder list unless already open,
// and makes it active either way.
func AddRepo(st State, root string) (State, error) {
	norm, err := Normalize(root)
	if err != nil {
		return st, err
	}
	if !contains(st.Repos, norm) {
		st.Repos = append(append([]string(nil), st.Repos...), norm)
	}
	st.Active = norm
	return st, nil
}

// RemoveRepo drops root from the folder list. When it was active, the
// neighbor at the same index (or the new last entry) becomes active.
func RemoveRepo(st State, root string) State {
	idx := -1
	for i, r := range st.Repos {
		if r == root {
			idx = i
			break
		}
	}
	if idx == -1 {
		return st
	}
	repos := append(append([]string(nil), st.Repos[:idx]...), st.Repos[idx+1:]...)
	st.Repos = repos
	if st.Active == root {
		switch {
		case len(repos) == 0:
			st.Active = ""
		case idx < len(repos):
			st.Active = repos[idx]
		default:
			st.Active = repos[len(repos)-1]
		}
	}
	return st
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
