// Package webassets serves the compiled web application embedded in the binary.
package webassets

import (
	"embed"
	"io/fs"
	"net/http"
)

// files contains output from `npm --prefix web run build`.
//
//go:embed all:dist
var files embed.FS

// Handler serves the embedded web application.
func Handler() http.Handler {
	assets, err := fs.Sub(files, "dist")
	if err != nil {
		panic(err)
	}

	return http.FileServer(http.FS(assets))
}
