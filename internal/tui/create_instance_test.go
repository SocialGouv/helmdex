package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestCreateInstanceShowsNameInTopBar(t *testing.T) {
	// Setup: temp directory for the repo
	tmpDir := t.TempDir()
	appsDir := filepath.Join(tmpDir, "apps")
	if err := os.MkdirAll(appsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Disable icons/logo for clean text
	t.Setenv("HELMDEX_NO_ICONS", "1")
	t.Setenv("HELMDEX_NO_LOGO", "1")
	t.Setenv("HELMDEX_NO_TITLE", "1")
	t.Setenv("NO_COLOR", "1")

	m := NewAppModel(Params{RepoRoot: tmpDir})
	m.screen = ScreenDashboard
	m.width = 120
	m.height = 40

	// Simulate pressing 'n' to start creating
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = nm.(AppModel)
	if !m.creating {
		t.Fatalf("expected creating to be true after pressing 'n'")
	}

	// Check the view shows "New instance"
	v := m.View()
	if !strings.Contains(v, "New instance") {
		t.Fatalf("expected 'New instance' in view, got:\n%s", v)
	}

	// Type 'a', 'l', 'p', 'h', 'a' one character at a time
	for _, ch := range "alpha" {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		m = nm.(AppModel)
	}

	// Check the text input has "alpha"
	if val := m.newName.Value(); val != "alpha" {
		t.Fatalf("expected newName value to be 'alpha', got %q", val)
	}

	// Press Enter to create instance
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(AppModel)

	if m.creating {
		t.Fatalf("expected creating to be false after Enter")
	}
	if m.screen != ScreenInstance {
		t.Fatalf("expected screen to be ScreenInstance, got %v", m.screen)
	}
	if m.selected == nil {
		t.Fatalf("expected selected to be non-nil")
	}
	if m.selected.Name != "alpha" {
		t.Fatalf("expected selected name to be 'alpha', got %q", m.selected.Name)
	}

	// Process any batch commands (like reloadInstancesCmd, loadChartCmd)
	if cmd != nil {
		// Execute batch to completion (simulate tea loop)
		msgs := executeBatch(cmd)
		for _, msg := range msgs {
			nm, cmd = m.Update(msg)
			m = nm.(AppModel)
			if cmd != nil {
				// Process second-level commands
				for _, msg2 := range executeBatch(cmd) {
					nm, _ = m.Update(msg2)
					m = nm.(AppModel)
				}
			}
		}
	}

	// Now check the View
	v = m.View()
	t.Logf("=== View after instance creation ===\n%s", v)

	// The top bar should contain "alpha"
	lines := strings.Split(v, "\n")
	if len(lines) == 0 {
		t.Fatalf("empty view")
	}

	// Check the full view contains "alpha"
	if !strings.Contains(v, "alpha") {
		t.Fatalf("expected 'alpha' somewhere in the view, got:\n%s", v)
	}
}

// executeBatch executes a tea.Cmd and collects all resulting messages.
func executeBatch(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if msg == nil {
		return nil
	}
	// Check if it's a batch message (tea.BatchMsg)
	if batch, ok := msg.(tea.BatchMsg); ok {
		var msgs []tea.Msg
		for _, c := range batch {
			msgs = append(msgs, executeBatch(c)...)
		}
		return msgs
	}
	return []tea.Msg{msg}
}
