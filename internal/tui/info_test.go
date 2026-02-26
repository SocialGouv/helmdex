package tui

import (
	"strings"
	"testing"

	"helmdex/internal/appinfo"
)

func TestRenderInfoOverlay_AboutTabContainsVersionAndRepo(t *testing.T) {
	m := NewAppModel(Params{RepoRoot: "."})
	m.width, m.height = 120, 40
	m.infoOpen = true
	m.infoTab = 1 // About

	out := renderInfoOverlay(m)
	if !strings.Contains(out, "Version:") {
		t.Fatalf("expected info/about to contain Version line")
	}
	if !strings.Contains(out, appinfo.RepoURL) {
		t.Fatalf("expected info/about to contain repo URL")
	}
}

