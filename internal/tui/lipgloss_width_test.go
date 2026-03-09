package tui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestLipglossWidthZero(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	s := lipgloss.NewStyle().Width(0).Padding(0, 1)
	out := s.Render("hello world")
	t.Logf("Width(0) render: %q (len=%d)", out, len(out))

	s2 := lipgloss.NewStyle().Width(118).Padding(0, 1)
	out2 := s2.Render("hello world")
	t.Logf("Width(118) render: %q (len=%d)", out2, len(out2))

	// What about max(0, 0-2) which is 0?
	s3 := lipgloss.NewStyle().Width(0)
	out3 := s3.Render("test content")
	t.Logf("Width(0) no padding: %q (len=%d)", out3, len(out3))
}
