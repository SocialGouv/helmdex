//go:build desktop && linux

package main

import (
	"log"
	"os"
)

// applyLinuxWebKitFixes sets the WebKitGTK environment defaults that make the
// embedded webview render at all. It MUST run before Wails boots GTK/WebKit:
// WebKit reads these at web-view creation and propagates them to the renderer
// subprocesses it spawns.
//
// WebKitGTK 2.36+ composites through a DMABUF renderer that fails on a wide
// range of Linux setups — NVIDIA and hybrid-GPU laptops above all, plus some
// Intel/Mesa drivers and most VMs. The window then shows nothing but the
// app's background colour, which reads as "the app is black". Falling back to
// the universally-compatible renderer costs nothing measurable here: the UI is
// a local SPA, not a compositing-heavy page.
//
// A value the user already exported wins, so an exotic GPU can be pointed at
// WEBKIT_DISABLE_COMPOSITING_MODE instead, or the DMABUF path forced back on.
func applyLinuxWebKitFixes() {
	if _, set := os.LookupEnv("WEBKIT_DISABLE_DMABUF_RENDERER"); set {
		return
	}
	if err := os.Setenv("WEBKIT_DISABLE_DMABUF_RENDERER", "1"); err != nil {
		log.Printf("desktop: could not set WEBKIT_DISABLE_DMABUF_RENDERER: %v", err)
		return
	}
	log.Printf("desktop: WEBKIT_DISABLE_DMABUF_RENDERER=1 (renders reliably across GPUs; export your own value to override)")
}
