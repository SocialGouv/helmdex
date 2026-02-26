package tui

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

// syntaxColorEnabled returns whether we should emit ANSI escape sequences.
//
// We follow the requested behavior:
// - disable when NO_COLOR is set (any value)
// - disable when TERM=dumb
func syntaxColorEnabled() bool {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("TERM")), "dumb") {
		return false
	}
	return true
}

func supportsTruecolor() bool {
	ct := strings.ToLower(strings.TrimSpace(os.Getenv("COLORTERM")))
	if strings.Contains(ct, "truecolor") || strings.Contains(ct, "24bit") {
		return true
	}
	// TERM heuristics: many terminals advertise 256-color but still support 24-bit.
	// Only treat "-direct" as a strong signal.
	term := strings.ToLower(strings.TrimSpace(os.Getenv("TERM")))
	return strings.Contains(term, "-direct")
}

func pickChromaFormatter() chroma.Formatter {
	if supportsTruecolor() {
		if f := formatters.Get("terminal16m"); f != nil {
			return f
		}
	}
	term := strings.ToLower(strings.TrimSpace(os.Getenv("TERM")))
	if strings.Contains(term, "256color") {
		if f := formatters.Get("terminal256"); f != nil {
			return f
		}
	}
	if f := formatters.Get("terminal"); f != nil {
		return f
	}
	return formatters.Fallback
}

func highlightYAMLForDisplay(yamlText string) string {
	if !syntaxColorEnabled() {
		return yamlText
	}
	if strings.TrimSpace(yamlText) == "" {
		return yamlText
	}

	lexer := lexers.Get("yaml")
	if lexer == nil {
		return yamlText
	}
	lexer = chroma.Coalesce(lexer)

	style := styles.Get("dracula")
	if style == nil {
		style = styles.Fallback
	}
	formatter := pickChromaFormatter()

	it, err := lexer.Tokenise(nil, yamlText)
	if err != nil {
		return yamlText
	}

	var buf bytes.Buffer
	if err := formatter.Format(&buf, style, it); err != nil {
		return yamlText
	}
	return buf.String()
}

func maybeHighlightYAMLForDisplay(path string, content string) string {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(path)))
	if ext != ".yaml" && ext != ".yml" {
		return content
	}
	return highlightYAMLForDisplay(content)
}
