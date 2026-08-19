//go:build desktop

package main

import (
	_ "embed"
	"fmt"
	"log"
	"os"
	"strings"

	"helmdex/internal/desktopstate"

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
	// Must precede wails.Run: WebKit reads its renderer settings when the
	// web view is created. Without this the window is a flat rectangle of
	// BackgroundColour on hybrid-GPU and NVIDIA machines.
	applyLinuxWebKitFixes()

	app := NewApp(initialDirFromArgs(os.Args[1:]))

	width, height, maximized := app.initialWindow()
	startState := options.Normal
	if maximized {
		startState = options.Maximised
	}

	// The web UI and API are served by the in-process internal/server
	// multi-workspace handler plugged directly into the Wails AssetServer —
	// no TCP port, no separate process. Assets stays nil so every request
	// (SPA assets and /ws/<id>/api/*) goes through the handler; Wails injects
	// its runtime into the index.html response it serves.
	err := wails.Run(&options.App{
		Title:            app.InitialTitle(),
		Width:            width,
		Height:           height,
		MinWidth:         800,
		MinHeight:        600,
		WindowStartState: startState,
		AssetServer: &assetserver.Options{
			Assets:  nil,
			Handler: app.Handler(),
		},
		EnableDefaultContextMenu: true,
		BackgroundColour:         &options.RGBA{R: 15, G: 17, B: 23, A: 255},
		Linux: &linux.Options{
			Icon: appIcon,
			// Sets the Wayland app_id (via g_set_prgname). It must equal the
			// window's X11 WM_CLASS — derived from the launch name
			// "helmdex-desktop" — and the installed desktop entry's basename,
			// so GNOME resolves the window icon from helmdex-desktop.desktop
			// on both Wayland and X11.
			ProgramName: "helmdex-desktop",
		},
		OnStartup:     app.onStartup,
		OnShutdown:    app.onShutdown,
		OnBeforeClose: app.onBeforeClose,
		Bind:          []any{app},
	})
	if err != nil {
		log.Fatalf("helmdex-desktop: wails run failed: %v", err)
	}
}

// initialDirFromArgs handles `helmdex-desktop [dir]`: the folder to open at
// launch, validated and normalized. No argument means "restore the last
// session".
func initialDirFromArgs(args []string) string {
	// macOS LaunchServices/Cocoa can inject framework flags ("-psn_*",
	// "-NSDocumentRevisionsDebugMode", …) on GUI launches: ignore every
	// dash-prefixed token instead of dying before the window opens.
	dirs := make([]string, 0, len(args))
	for _, a := range args {
		if a == "-h" || a == "--help" {
			fmt.Println("usage: helmdex-desktop [dir]")
			os.Exit(0)
		}
		if strings.HasPrefix(a, "-") {
			continue
		}
		dirs = append(dirs, a)
	}
	if len(dirs) == 0 {
		return ""
	}
	if len(dirs) > 1 || dirs[0] == "" {
		fmt.Fprintln(os.Stderr, "usage: helmdex-desktop [dir]")
		os.Exit(2)
	}
	dir, err := desktopstate.Normalize(dirs[0])
	if err != nil {
		log.Fatalf("helmdex-desktop: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		log.Fatalf("helmdex-desktop: cannot open %s: %v", dirs[0], err)
	}
	if !info.IsDir() {
		log.Fatalf("helmdex-desktop: %s is not a directory", dirs[0])
	}
	return dir
}
