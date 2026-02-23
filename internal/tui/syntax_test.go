package tui

import (
	"os"
	"strings"
	"testing"
)

func unsetEnv(t *testing.T, key string) {
	old, had := os.LookupEnv(key)
	_ = os.Unsetenv(key)
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(key, old)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}

func TestHighlightYAMLForDisplay_EmitsANSIWhenEnabled(t *testing.T) {
	// Force a predictable terminal profile.
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("COLORTERM", "truecolor")
	// Ensure we don't get disabled by NO_COLOR.
	unsetEnv(t, "NO_COLOR")

	in := "replicaCount: 1\nimage:\n  tag: 1.2.3\n"
	out := highlightYAMLForDisplay(in)
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("expected ANSI escape sequences in output")
	}
}

func TestHighlightYAMLForDisplay_DisabledByNO_COLOR(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	// Even if the terminal supports colors, NO_COLOR must win.
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("COLORTERM", "truecolor")

	in := "a: 1\n"
	out := highlightYAMLForDisplay(in)
	if out != in {
		t.Fatalf("expected output to equal input when NO_COLOR is set")
	}
}

func TestHighlightYAMLForDisplay_DisabledByTermDumb(t *testing.T) {
	t.Setenv("TERM", "dumb")
	// Ensure NO_COLOR doesn't accidentally mask this path.
	unsetEnv(t, "NO_COLOR")

	in := "a: 1\n"
	out := highlightYAMLForDisplay(in)
	if out != in {
		t.Fatalf("expected output to equal input when TERM=dumb")
	}
}

func TestMaybeHighlightYAMLForDisplay_IgnoresNonYAML(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("COLORTERM", "truecolor")
	t.Setenv("NO_COLOR", "")

	in := "a: 1\n"
	out := maybeHighlightYAMLForDisplay("README.md", in)
	if out != in {
		t.Fatalf("expected non-yaml paths to be returned unchanged")
	}
}
