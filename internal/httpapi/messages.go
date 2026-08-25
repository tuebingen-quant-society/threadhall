package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"unicode/utf8"

	"github.com/tuebingen-quant-society/threadhall/internal/message"
)

const (
	maxMessageTargetBytes  = 2048
	maxMessageRequestBytes = 6*(message.MaxBodyBytes+message.MaxIdempotencyKeyBytes) + 256
)

// MessageAPI is the HTTP-facing subset of message.Service.
type MessageAPI interface {
	Send(context.Context, message.Send) (message.Result, error)
	Edit(context.Context, message.Edit) (message.Result, error)
	Delete(context.Context, message.Delete) (message.Result, error)
	History(context.Context, message.History) (message.Page, error)
}

type messageHandler struct{ api MessageAPI }

// RegisterMessages installs authenticated root-text message routes.
func RegisterMessages(mux *http.ServeMux, authAPI AuthAPI, api MessageAPI, publicOrigin string) {
	handler := &messageHandler{api: api}
	read := func(next http.HandlerFunc) http.Handler {
		return disableAuthCaching(requireMessageTarget(validateMessagePageQuery,
			RequireSession(authAPI, next)))
	}
	mutation := func(next http.HandlerFunc) http.Handler {
		return disableAuthCaching(requireMessageTarget(validateNoMessageQuery,
			requireMutationSecurity(publicOrigin, RequireSession(authAPI, next))))
	}
	mux.Handle("GET /api/v1/conversations/{conversation_id}/messages", read(handler.history))
	mux.Handle("POST /api/v1/conversations/{conversation_id}/messages", mutation(handler.send))
	mux.Handle("PATCH /api/v1/messages/{message_id}", mutation(handler.edit))
	mux.Handle("DELETE /api/v1/messages/{message_id}", mutation(handler.delete))
}

type messageBodyRequest struct {
	Body           string `json:"body"`
	IdempotencyKey string `json:"idempotency_key"`
}

func (h *messageHandler) send(w http.ResponseWriter, request *http.Request) {
	conversationID, err := positiveMessagePathID(request, "conversation_id")
	var body messageBodyRequest
	if err != nil || decodeMessageJSON(request, &body) != nil || !boundedMessageBody(body.Body) {
		writeMessageProblem(w, message.ErrInvalidInput)
		return
	}
	user, _ := UserFromContext(request.Context())
	result, err := h.api.Send(request.Context(), message.Send{
		ConversationID: conversationID, AuthorID: user.ID,
		Body: body.Body, IdempotencyKey: body.IdempotencyKey,
	})
	if writeMessageProblem(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h *messageHandler) edit(w http.ResponseWriter, request *http.Request) {
	messageID, err := positiveMessagePathID(request, "message_id")
	var body messageBodyRequest
	if err != nil || decodeMessageJSON(request, &body) != nil || !boundedMessageBody(body.Body) {
		writeMessageProblem(w, message.ErrInvalidInput)
		return
	}
	user, _ := UserFromContext(request.Context())
	result, err := h.api.Edit(request.Context(), message.Edit{
		MessageID: messageID, AuthorID: user.ID, Body: body.Body, IdempotencyKey: body.IdempotencyKey,
	})
	if writeMessageProblem(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *messageHandler) delete(w http.ResponseWriter, request *http.Request) {
	messageID, err := positiveMessagePathID(request, "message_id")
	var body struct {
		IdempotencyKey string `json:"idempotency_key"`
	}
	if err != nil || decodeMessageJSON(request, &body) != nil {
		writeMessageProblem(w, message.ErrInvalidInput)
		return
	}
	user, _ := UserFromContext(request.Context())
	result, err := h.api.Delete(request.Context(), message.Delete{
		MessageID: messageID, AuthorID: user.ID, IdempotencyKey: body.IdempotencyKey,
	})
	if writeMessageProblem(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *messageHandler) history(w http.ResponseWriter, request *http.Request) {
	conversationID, err := positiveMessagePathID(request, "conversation_id")
	if err != nil {
		writeMessageProblem(w, err)
		return
	}
	beforeID, limit, err := boundedMessagePage(messageQuery(request.Context()))
	if err != nil {
		writeMessageProblem(w, err)
		return
	}
	user, _ := UserFromContext(request.Context())
	page, err := h.api.History(request.Context(), message.History{
		ConversationID: conversationID, UserID: user.ID, BeforeID: beforeID, Limit: limit,
	})
	if writeMessageProblem(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func decodeMessageJSON(request *http.Request, destination any) error {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" || request.ContentLength > maxMessageRequestBytes {
		return message.ErrInvalidInput
	}
	raw, err := io.ReadAll(io.LimitReader(request.Body, maxMessageRequestBytes+1))
	if err != nil || len(raw) > maxMessageRequestBytes || !utf8.Valid(raw) {
		return message.ErrInvalidInput
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return message.ErrInvalidInput
	}
	return nil
}

func boundedMessageBody(body string) bool {
	return utf8.ValidString(body) && len(body) <= message.MaxBodyBytes
}

type messageQueryKey struct{}
type messageQueryPolicy func(url.Values) error

func requireMessageTarget(policy messageQueryPolicy, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if len(request.URL.EscapedPath()) > maxMessageTargetBytes || len(request.URL.RawQuery) > maxMessageTargetBytes {
			writeMessageProblem(w, message.ErrInvalidInput)
			return
		}
		values, err := url.ParseQuery(request.URL.RawQuery)
		if err != nil || policy(values) != nil {
			writeMessageProblem(w, message.ErrInvalidInput)
			return
		}
		ctx := context.WithValue(request.Context(), messageQueryKey{}, values)
		next.ServeHTTP(w, request.WithContext(ctx))
	})
}

func messageQuery(ctx context.Context) url.Values {
	values, _ := ctx.Value(messageQueryKey{}).(url.Values)
	return values
}

func validateMessagePageQuery(values url.Values) error {
	_, _, err := boundedMessagePage(values)
	return err
}

func validateNoMessageQuery(values url.Values) error {
	if len(values) != 0 {
		return message.ErrInvalidInput
	}
	return nil
}

func boundedMessagePage(values url.Values) (int64, int, error) {
	for key, entries := range values {
		if (key != "before_id" && key != "limit") || len(entries) != 1 || entries[0] == "" {
			return 0, 0, message.ErrInvalidInput
		}
	}
	var beforeID int64
	var limit int
	var err error
	if value := values.Get("before_id"); value != "" {
		beforeID, err = strconv.ParseInt(value, 10, 64)
		if err != nil || beforeID <= 0 {
			return 0, 0, message.ErrInvalidInput
		}
	}
	if value := values.Get("limit"); value != "" {
		limit, err = strconv.Atoi(value)
		if err != nil || limit <= 0 || limit > message.MaxPageLimit {
			return 0, 0, message.ErrInvalidInput
		}
	}
	return beforeID, limit, nil
}

func positiveMessagePathID(request *http.Request, name string) (int64, error) {
	id, err := strconv.ParseInt(request.PathValue(name), 10, 64)
	if err != nil || id <= 0 {
		return 0, message.ErrInvalidInput
	}
	return id, nil
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
