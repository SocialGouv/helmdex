//go:build desktop

package main

import (
	_ "embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
)

// Embedded window icon: Linux WMs otherwise fall back to a fragile
// StartupWMClass → .desktop → theme lookup.
//
//go:embed appicon.png
var appIcon []byte

func main() {
	app := NewApp()

	// The web UI and API are served by the in-process internal/server
	// handler plugged directly into the Wails AssetServer — no TCP port,
	// no separate process. Assets stays nil so every request (SPA assets
	// and /api/*) goes through the handler; Wails injects its runtime
	// into the index.html response it serves.
	err := wails.Run(&options.App{
		Title:     "Helmdex",
		Width:     1400,
		Height:    900,
		MinWidth:  800,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets:  nil,
			Handler: app.Handler(),
		},
		EnableDefaultContextMenu: true,
		BackgroundColour:         &options.RGBA{R: 15, G: 17, B: 23, A: 255},
		Linux: &linux.Options{
			Icon:        appIcon,
			ProgramName: "helmdex",
		},
		OnStartup: app.onStartup,
		Bind:      []any{app},
	})
	if err != nil {
		log.Fatalf("helmdex-desktop: wails run failed: %v", err)
	}
}
