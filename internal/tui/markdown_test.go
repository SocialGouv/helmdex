package tui

import "testing"

func TestRenderMarkdownForDisplay_NoColorReturnsRaw(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	in := "# Title\n\nHello *world*\n"
	out := renderMarkdownForDisplay(80, in)
	if out != in {
		t.Fatalf("expected raw markdown when NO_COLOR is set")
	}
}

func TestRenderMarkdownForDisplay_EmptyIsEmpty(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	if got := renderMarkdownForDisplay(80, "\n\t\n"); got != "\n\t\n" {
		t.Fatalf("expected whitespace-only to be returned unchanged")
	}
}

