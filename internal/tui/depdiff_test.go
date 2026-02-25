package tui

import (
	"strings"
	"testing"

	"helmdex/internal/yamlchart"

	tea "github.com/charmbracelet/bubbletea"
)

func TestDiffJSONSchema_Basic(t *testing.T) {
	oldS := `{"type":"object","properties":{"a":{"type":"string"}}}`
	newS := `{"type":"object","properties":{"a":{"type":"number"},"b":{"type":"boolean"}}}`

	r, c, ok, err := DiffJSONSchema(oldS, newS)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !ok {
		t.Fatalf("expected ok")
	}
	if r == "" {
		t.Fatalf("expected rendered diff")
	}
	if c.Changed == 0 {
		t.Fatalf("expected changed>0, got %+v", c)
	}
	if c.Added == 0 {
		t.Fatalf("expected added>0, got %+v", c)
	}
}

func TestDiffYAMLValues_Basic(t *testing.T) {
	oldV := "a: 1\n"
	newV := "a: 2\nb: true\n"

	r, c, ok, err := DiffYAMLValues(oldV, newV)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !ok {
		t.Fatalf("expected ok")
	}
	if r == "" {
		t.Fatalf("expected rendered diff")
	}
	if c.Changed != 1 {
		t.Fatalf("expected changed=1, got %+v", c)
	}
	if c.Added != 1 {
		t.Fatalf("expected added=1, got %+v", c)
	}
}

func TestDepDiffUpdate_CancelAndApply(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "/tmp", StartScreen: ScreenDashboard})
	m.depDiffOpen = true
	m.depDiffNewDep = yamlchart.Dependency{Name: "nginx", Repository: "https://example.invalid", Version: "2.0.0"}

	// Cancel closes.
	nm, _ := m.depDiffUpdate(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	mm := nm.(AppModel)
	if mm.depDiffOpen {
		t.Fatalf("expected closed on cancel")
	}

	// Apply closes and returns a cmd.
	m.depDiffOpen = true
	nm2, cmd := m.depDiffUpdate(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	mm2 := nm2.(AppModel)
	if mm2.depDiffOpen {
		t.Fatalf("expected closed on apply")
	}
	if cmd == nil {
		t.Fatalf("expected cmd on apply")
	}
}

func TestDepDiffUpdate_ToggleSideBySide(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "/tmp", StartScreen: ScreenDashboard})
	m.depDiffOpen = true
	if m.depDiffSideBySide {
		t.Fatalf("expected default false")
	}
	nm, _ := m.depDiffUpdate(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	mm := nm.(AppModel)
	if !mm.depDiffSideBySide {
		t.Fatalf("expected toggled true")
	}
	if !mm.depDiffSideBySideUser {
		t.Fatalf("expected user override set")
	}
}

func TestDepDiffUpdate_ToggleWrap(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "/tmp", StartScreen: ScreenDashboard})
	m.depDiffOpen = true
	if m.depDiffWrap {
		t.Fatalf("expected default false")
	}
	nm, _ := m.depDiffUpdate(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	mm := nm.(AppModel)
	if !mm.depDiffWrap {
		t.Fatalf("expected toggled true")
	}
	if !mm.depDiffWrapUser {
		t.Fatalf("expected user override set")
	}
}

func TestWrapLine_NoWrap(t *testing.T) {
	parts := wrapLine("hello world", 5, false)
	if len(parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(parts))
	}
}

func TestRenderSideBySide_WrapExpandsRows(t *testing.T) {
	rows := []diffRow{{
		Path: "$.x",
		Old:  strings.Repeat("a", 80),
		New:  strings.Repeat("b", 80),
		Kind: "chg",
	}}

	// Small width to force wrapping.
	out := renderSideBySide(rows, 60, true)
	if strings.Count(out, "\n") < 3 {
		t.Fatalf("expected wrapped output to span multiple lines; got:\n%s", out)
	}
}

func TestIntralineSplit_ValuePortionOnly(t *testing.T) {
	pfx, val, ok := splitValueForIntraline("- $.a.b: hello")
	if !ok {
		t.Fatalf("expected ok")
	}
	if pfx != "- $.a.b: " {
		t.Fatalf("unexpected prefix: %q", pfx)
	}
	if val != "hello" {
		t.Fatalf("unexpected value: %q", val)
	}
}

func TestCommonPrefixSuffixRunes(t *testing.T) {
	a := "abcdXYZef"
	b := "abcd123ef"
	p := commonPrefixLenRunes(a, b)
	if p != 4 {
		t.Fatalf("expected prefix=4, got %d", p)
	}
	s := commonSuffixLenRunes(a, b, p)
	if s != 2 {
		t.Fatalf("expected suffix=2, got %d", s)
	}
}
