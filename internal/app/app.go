// Package app wires Threadhall's HTTP application.
package app

import (
	"net/http"

	"github.com/tuebingen-quant-society/threadhall/internal/webassets"
)

// New returns the Threadhall HTTP application.
func New() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", health)
	mux.Handle("/", webassets.Handler())
	return mux
}

func health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok\n"))
}
