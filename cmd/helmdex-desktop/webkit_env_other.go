//go:build desktop && !linux

package main

// applyLinuxWebKitFixes is a no-op off Linux: the DMABUF renderer it works
// around is WebKitGTK's, and macOS (WKWebView) and Windows (WebView2) use
// neither.
func applyLinuxWebKitFixes() {}
