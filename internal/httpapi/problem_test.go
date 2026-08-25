package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteProblem(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	WriteProblem(recorder, Problem{
		Status: http.StatusBadRequest,
		Code:   "bad_request",
		Detail: "a request field is invalid",
	})

	if got := recorder.Code; got != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", got, http.StatusBadRequest)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want %q", got, "application/json")
	}

	var body map[string]any
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := body["status"]; got != float64(http.StatusBadRequest) {
		t.Errorf("body status = %#v, want %d", got, http.StatusBadRequest)
	}
	if got := body["code"]; got != "bad_request" {
		t.Errorf("code = %#v, want %q", got, "bad_request")
	}
	if _, ok := body["cause"]; ok {
		t.Error("response exposes an internal cause")
	}
}
