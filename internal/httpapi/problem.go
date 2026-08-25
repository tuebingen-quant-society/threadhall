// Package httpapi contains common HTTP response contracts.
package httpapi

import (
	"encoding/json"
	"net/http"
)

// Problem is the stable public representation of an HTTP error.
type Problem struct {
	Status int    `json:"status"`
	Code   string `json:"code"`
	Detail string `json:"detail"`
}

// WriteProblem writes p without exposing internal error details.
func WriteProblem(w http.ResponseWriter, p Problem) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}
