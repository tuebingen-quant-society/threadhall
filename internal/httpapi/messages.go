package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/tuebingen-quant-society/threadhall/internal/message"
)

// MessageAPI is the HTTP-facing subset of message.Service.
type MessageAPI interface {
	Send(context.Context, message.Send) (message.Result, error)
	Edit(context.Context, message.Edit) (message.Result, error)
	Delete(context.Context, message.Delete) (message.Result, error)
	History(context.Context, message.History) (message.Page, error)
}

type messageHandler struct {
	api      MessageAPI
	notifier EventNotifier
}

// RegisterMessages installs authenticated root-text message routes.
func RegisterMessages(
	mux *http.ServeMux,
	authAPI AuthAPI,
	api MessageAPI,
	notifier EventNotifier,
	publicOrigin string,
) {
	handler := &messageHandler{api: api, notifier: notifier}
	read := func(preflight func(http.Handler) http.Handler, next http.HandlerFunc) http.Handler {
		return disableAuthCaching(preflight(RequireSession(authAPI, next)))
	}
	mutation := func(preflight func(http.Handler) http.Handler, next http.HandlerFunc) http.Handler {
		secured := requireMutationSecurity(publicOrigin, RequireSession(authAPI, next))
		return disableAuthCaching(preflight(secured))
	}
	mux.Handle("GET /api/v1/conversations/{conversation_id}/messages", read(preflightMessageHistory, handler.history))
	mux.Handle("POST /api/v1/conversations/{conversation_id}/messages", mutation(preflightMessageSend, handler.send))
	mux.Handle("PATCH /api/v1/messages/{message_id}", mutation(preflightMessageEdit, handler.edit))
	mux.Handle("DELETE /api/v1/messages/{message_id}", mutation(preflightMessageDelete, handler.delete))
}

func (h *messageHandler) send(w http.ResponseWriter, request *http.Request) {
	prepared, ok := preparedMessageFromContext(request.Context())
	if !ok {
		writeInternalProblem(w)
		return
	}
	user, _ := UserFromContext(request.Context())
	result, err := h.api.Send(request.Context(), message.Send{
		ConversationID: prepared.conversationID, AuthorID: user.ID,
		Body: prepared.body.Body, IdempotencyKey: prepared.body.IdempotencyKey,
	})
	if writeMessageProblem(w, err) {
		return
	}
	h.notifier.Notify(result.Event.Seq)
	writeJSON(w, http.StatusCreated, result)
}

func (h *messageHandler) edit(w http.ResponseWriter, request *http.Request) {
	prepared, ok := preparedMessageFromContext(request.Context())
	if !ok {
		writeInternalProblem(w)
		return
	}
	user, _ := UserFromContext(request.Context())
	result, err := h.api.Edit(request.Context(), message.Edit{
		MessageID: prepared.messageID, AuthorID: user.ID,
		Body: prepared.body.Body, IdempotencyKey: prepared.body.IdempotencyKey,
	})
	if writeMessageProblem(w, err) {
		return
	}
	h.notifier.Notify(result.Event.Seq)
	writeJSON(w, http.StatusOK, result)
}

func (h *messageHandler) delete(w http.ResponseWriter, request *http.Request) {
	prepared, ok := preparedMessageFromContext(request.Context())
	if !ok {
		writeInternalProblem(w)
		return
	}
	user, _ := UserFromContext(request.Context())
	result, err := h.api.Delete(request.Context(), message.Delete{
		MessageID: prepared.messageID, AuthorID: user.ID, IdempotencyKey: prepared.deletion.IdempotencyKey,
	})
	if writeMessageProblem(w, err) {
		return
	}
	h.notifier.Notify(result.Event.Seq)
	writeJSON(w, http.StatusOK, result)
}

func (h *messageHandler) history(w http.ResponseWriter, request *http.Request) {
	prepared, ok := preparedMessageFromContext(request.Context())
	if !ok {
		writeInternalProblem(w)
		return
	}
	user, _ := UserFromContext(request.Context())
	page, err := h.api.History(request.Context(), message.History{
		ConversationID: prepared.conversationID, UserID: user.ID,
		BeforeID: prepared.beforeID, Limit: prepared.limit,
	})
	if writeMessageProblem(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, page)
}
func writeMessageProblem(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	problem := Problem{Status: 500, Code: "internal_error", Detail: "request could not be completed"}
	switch {
	case errors.Is(err, message.ErrInvalidInput):
		problem = Problem{Status: 400, Code: "invalid_request", Detail: "request is invalid"}
	case errors.Is(err, message.ErrNotFound):
		problem = Problem{Status: 404, Code: "not_found", Detail: "resource was not found"}
	case errors.Is(err, message.ErrConflict):
		problem = Problem{Status: 409, Code: "idempotency_conflict", Detail: "request conflicts with an earlier operation"}
	case errors.Is(err, message.ErrBusy):
		problem = Problem{Status: 503, Code: "temporarily_unavailable", Detail: "service is temporarily unavailable"}
	}
	WriteProblem(w, problem)
	return true
}
