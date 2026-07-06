package api

import (
	"embed"
	"io/fs"
	"net/http"
)

// The gsmnode web panel: a Vue 3 + Tailwind app (source in panel/, built
// with `npm run build` into dist/) embedded at compile time and served at
// the server root.
//
//go:embed all:dist
var panelFS embed.FS

// panelHandler serves the built panel assets.
func panelHandler() http.Handler {
	sub, err := fs.Sub(panelFS, "dist")
	if err != nil {
		panic(err) // embedded dist is malformed; unreachable in a valid build
	}
	return http.FileServerFS(sub)
}
