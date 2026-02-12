package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

type ScreenID string

const (
	ScreenDashboard ScreenID = "dashboard"
	ScreenInstance  ScreenID = "instance"
)

type Params struct {
	RepoRoot   string
	ConfigPath string
	Config     any

	StartScreen ScreenID
}

func Run(ctx context.Context, p Params) error {
	if p.RepoRoot == "" {
		return fmt.Errorf("repoRoot is required")
	}
	if p.StartScreen == "" {
		p.StartScreen = ScreenDashboard
	}

	m := NewAppModel(p)
	prog := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx))
	_, err := prog.Run()
	return err
}

