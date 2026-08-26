package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/tuebingen-quant-society/threadhall/internal/agenttask"
	"github.com/tuebingen-quant-society/threadhall/internal/auth"
)

type AgentWorkerAPI interface {
	Claim(context.Context, [32]byte, time.Time) (agenttask.Work, bool, error)
	Progress(context.Context, [32]byte, int64, string, time.Time) error
	Complete(context.Context, [32]byte, agenttask.Completion) error
	Fail(context.Context, [32]byte, agenttask.Failure) error
	ReplaceCapabilities(context.Context, [32]byte, []agenttask.Capability, time.Time) error
}

type agentWorkerHandler struct {
	api      AgentWorkerAPI
	notifier EventNotifier
	now      func() time.Time
}

// RegisterAgentWorker installs the credential-isolated outbound worker surface.
func RegisterAgentWorker(mux *http.ServeMux, api AgentWorkerAPI, notifier EventNotifier) {
	handler := &agentWorkerHandler{api: api, notifier: notifier, now: time.Now}
	mux.Handle("GET /api/v1/agent/work", disableAuthCaching(http.HandlerFunc(handler.claim)))
	mux.Handle("POST /api/v1/agent/capabilities", disableAuthCaching(http.HandlerFunc(handler.capabilities)))
	mux.Handle("POST /api/v1/agent/tasks/{task_id}/progress", disableAuthCaching(http.HandlerFunc(handler.progress)))
	mux.Handle("POST /api/v1/agent/tasks/{task_id}/complete", disableAuthCaching(http.HandlerFunc(handler.complete)))
	mux.Handle("POST /api/v1/agent/tasks/{task_id}/fail", disableAuthCaching(http.HandlerFunc(handler.fail)))
}

func (h *agentWorkerHandler) capabilities(w http.ResponseWriter, request *http.Request) {
	hash, ok := workerToken(w, request)
	if !ok {
		return
	}
	var body agenttask.CapabilityPage
	if decodeWorkerJSON(w, request, &body, 1<<20) != nil {
		writeInvalidRequest(w)
		return
	}
	if writeAgentProblem(w, h.api.ReplaceCapabilities(request.Context(), hash, body.Capabilities, h.now().UTC())) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *agentWorkerHandler) claim(w http.ResponseWriter, request *http.Request) {
	hash, ok := workerToken(w, request)
	if !ok {
		return
	}
	work, found, err := h.api.Claim(request.Context(), hash, h.now().UTC())
	if writeAgentProblem(w, err) {
		return
	}
	if !found {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	h.notify()
	writeJSON(w, http.StatusOK, work)
}

func (h *agentWorkerHandler) progress(w http.ResponseWriter, request *http.Request) {
	hash, taskID, ok := h.authenticatedTask(w, request)
	if !ok {
		return
	}
	var body struct {
		Summary string `json:"summary"`
	}
	if decodeWorkerJSON(w, request, &body, 5<<10) != nil || strings.TrimSpace(body.Summary) == "" {
		writeInvalidRequest(w)
		return
	}
	if writeAgentProblem(w, h.api.Progress(request.Context(), hash, taskID, body.Summary, h.now().UTC())) {
		return
	}
	h.notify()
	w.WriteHeader(http.StatusNoContent)
}

func (h *agentWorkerHandler) complete(w http.ResponseWriter, request *http.Request) {
	hash, taskID, ok := h.authenticatedTask(w, request)
	if !ok {
		return
	}
	var body struct {
		Output          string                `json:"output"`
		RuntimeThreadID string                `json:"runtime_thread_id"`
		Apps            []agenttask.InlineApp `json:"apps"`
		Questions       []agenttask.Question  `json:"questions"`
	}
	if decodeWorkerJSON(w, request, &body, agenttask.MaxOutputBytes+agenttask.MaxInlineAppsBytes+16<<10) != nil ||
		strings.TrimSpace(body.Output) == "" || len(body.Output) > agenttask.MaxOutputBytes || !agenttask.ValidInlineApps(body.Apps) || !agenttask.ValidQuestions(body.Questions) {
		writeInvalidRequest(w)
		return
	}
	err := h.api.Complete(request.Context(), hash, agenttask.Completion{
		TaskID: taskID, Output: body.Output, RuntimeThreadID: body.RuntimeThreadID, Apps: body.Apps, Questions: body.Questions, CompletedAt: h.now().UTC(),
	})
	if writeAgentProblem(w, err) {
		return
	}
	h.notify()
	w.WriteHeader(http.StatusNoContent)
}

func (h *agentWorkerHandler) fail(w http.ResponseWriter, request *http.Request) {
	hash, taskID, ok := h.authenticatedTask(w, request)
	if !ok {
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if decodeWorkerJSON(w, request, &body, 1024) != nil ||
		(body.Reason != "runtime_failed" && body.Reason != "interaction_unsupported") {
		writeInvalidRequest(w)
		return
	}
	err := h.api.Fail(request.Context(), hash, agenttask.Failure{
		TaskID: taskID, Reason: body.Reason, FailedAt: h.now().UTC(),
	})
	if writeAgentProblem(w, err) {
		return
	}
	h.notify()
	w.WriteHeader(http.StatusNoContent)
}

func (h *agentWorkerHandler) authenticatedTask(w http.ResponseWriter, request *http.Request) ([32]byte, int64, bool) {
	hash, ok := workerToken(w, request)
	if !ok {
		return hash, 0, false
	}
	taskID, err := positivePathID(request, "task_id")
	if err != nil {
		writeInvalidRequest(w)
		return hash, 0, false
	}
	return hash, taskID, true
}

func (h *agentWorkerHandler) notify() {
	if h.notifier != nil {
		h.notifier.Notify(0)
	}
}

func workerToken(w http.ResponseWriter, request *http.Request) ([32]byte, bool) {
	var zero [32]byte
	header := request.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") || strings.Count(header, " ") != 1 {
		WriteProblem(w, Problem{Status: http.StatusUnauthorized, Code: "agent_authentication_required", Detail: "worker authentication is required"})
		return zero, false
	}
	raw, err := auth.DecodeToken(strings.TrimPrefix(header, "Bearer "))
	if err != nil {
		WriteProblem(w, Problem{Status: http.StatusUnauthorized, Code: "agent_authentication_required", Detail: "worker authentication is required"})
		return zero, false
	}
	return sha256.Sum256(raw[:]), true
}

func decodeWorkerJSON(w http.ResponseWriter, request *http.Request, destination any, limit int64) error {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return agenttask.ErrInvalidInput
	}
	request.Body = http.MaxBytesReader(w, request.Body, limit)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return agenttask.ErrInvalidInput
	}
	return nil
}

func writeAgentProblem(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	problem := Problem{Status: 500, Code: "internal_error", Detail: "agent request could not be completed"}
	switch {
	case errors.Is(err, agenttask.ErrUnauthenticated):
		problem = Problem{Status: 401, Code: "agent_authentication_required", Detail: "worker authentication is required"}
	case errors.Is(err, agenttask.ErrInvalidInput):
		problem = Problem{Status: 400, Code: "invalid_request", Detail: "agent request is invalid"}
	case errors.Is(err, agenttask.ErrForbidden):
		problem = Problem{Status: 403, Code: "agent_scope_forbidden", Detail: "agent scope does not allow this operation"}
	case errors.Is(err, agenttask.ErrNotFound):
		problem = Problem{Status: 404, Code: "not_found", Detail: "agent task was not found"}
	case errors.Is(err, agenttask.ErrConflict):
		problem = Problem{Status: 409, Code: "agent_task_conflict", Detail: "agent task state has changed"}
	case errors.Is(err, agenttask.ErrBusy):
		problem = Problem{Status: 503, Code: "temporarily_unavailable", Detail: "service is temporarily unavailable"}
	}
	WriteProblem(w, problem)
	return true
}
