package appinfo

import (
	"runtime/debug"
	"strings"
)

// Package appinfo centralizes end-user metadata for helmdex.
//
// Keep these values stable: they are shown in CLI help output and the TUI About
// view.
const (
	Name   = "helmdex"
	Short  = "helmdex scaffolds and maintains GitOps-friendly Helm umbrella chart instances"
	Long   = "helmdex is a TUI-first organizer for Helm umbrella chart instances (no template rendering, no deploy)."
	RepoURL = "https://github.com/SocialGouv/helmdex"
)

// Version is intended to be overridden at build time.
//
// Example:
//   go build -ldflags "-X helmdex/internal/appinfo.Version=v0.3.0 -X helmdex/internal/appinfo.Commit=$(git rev-parse --short HEAD)"
var Version = "dev"

// Commit optionally carries a VCS revision (preferably short SHA).
// It can be set via -ldflags or inferred from Go build settings.
var Commit = ""

func init() {
	// Best-effort: populate Commit from build settings if it wasn't injected.
	if strings.TrimSpace(Commit) != "" {
		return
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	for _, s := range bi.Settings {
		if s.Key == "vcs.revision" {
			Commit = strings.TrimSpace(s.Value)
			break
		}
	}
}

func FullVersion() string {
	v := strings.TrimSpace(Version)
	if v == "" {
		v = "dev"
	}
	c := strings.TrimSpace(Commit)
	if c == "" {
		return v
	}
	// Display a short commit when possible.
	if len(c) > 12 {
		c = c[:12]
	}
	return v + "+" + c
}

