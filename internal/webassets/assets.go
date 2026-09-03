// Package webassets serves the compiled web application embedded in the binary.
package webassets

import (
	"bytes"
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
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
	index, err := fs.ReadFile(assets, "index.html")
	if err != nil {
		panic(err)
	}

	files := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requestPath := request.URL.Path
		if strings.HasPrefix(requestPath, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		if requestPath == "/" || requestPath == "/index.html" || requestPath == "/sw.js" || requestPath == "/manifest.webmanifest" {
			w.Header().Set("Cache-Control", "no-cache")
		}
		if requestPath == "/index.html" || shouldServeShell(request) {
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeContent(w, request, "index.html", time.Time{}, bytes.NewReader(index))
			return
		}
		files.ServeHTTP(w, request)
	})
}

func shouldServeShell(request *http.Request) bool {
	requestPath := request.URL.Path
	return request.Method == http.MethodGet &&
		request.Header.Get("Sec-Fetch-Mode") == "navigate" &&
		strings.Contains(request.Header.Get("Accept"), "text/html") &&
		path.Ext(requestPath) == "" &&
		!isReservedPath(requestPath)
}

func isReservedPath(requestPath string) bool {
	for _, prefix := range []string{"/api", "/healthz", "/mcp", "/manifest.webmanifest", "/sw.js", "/icons", "/assets"} {
		if requestPath == prefix || strings.HasPrefix(requestPath, prefix+"/") {
			return true
		}
	}
	return false
}
