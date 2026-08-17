package server

import "embed"

// staticFS embeds the built web UI. During development the real SPA is served
// by the Vite dev server (proxying /api here); release builds copy
// webui/dist into static/ before compiling.
//
//go:embed static
var staticFS embed.FS
