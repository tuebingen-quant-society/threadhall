// Package app wires Threadhall's HTTP application.
package app

import (
	"database/sql"
	"net/http"

	"github.com/tuebingen-quant-society/threadhall/internal/webassets"
)

// New returns the Threadhall HTTP application backed by db.
func New(db *sql.DB) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", health(db))
	mux.Handle("/", webassets.Handler())
	return mux
}

func health(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		if db == nil || db.PingContext(request.Context()) != nil {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	}
}
