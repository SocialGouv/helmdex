package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func cmdEmitsQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	msg := cmd()
	switch m := msg.(type) {
	case tea.QuitMsg:
		return true
	case tea.BatchMsg:
		for _, innerCmd := range m {
			if cmdEmitsQuit(innerCmd) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func assertCmdQuits(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	if !cmdEmitsQuit(cmd) {
		if cmd == nil {
			t.Fatalf("expected quit command, got nil")
		}
		m := cmd()
		t.Fatalf("expected cmd to emit tea.QuitMsg, got %T", m)
	}
}
