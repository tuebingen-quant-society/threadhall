package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/tuebingen-quant-society/threadhall/internal/conversation"
)

// ConversationAPI is the HTTP-facing subset of conversation.Service.
type ConversationAPI interface {
	CreateChannel(context.Context, conversation.CreateChannel) (conversation.Conversation, error)
	CreateDM(context.Context, conversation.CreateDM) (conversation.Conversation, error)
	Fork(context.Context, conversation.ForkConversation) (conversation.Fork, error)
	List(context.Context, conversation.ListConversations) (conversation.ConversationPage, error)
	Detail(context.Context, int64, int64) (conversation.Conversation, error)
	Members(context.Context, conversation.ListMembers) (conversation.MemberPage, error)
	AddMember(context.Context, conversation.ChangeMember) error
	RemoveMember(context.Context, conversation.ChangeMember) error
	Delete(context.Context, conversation.DeleteConversation) error
	Rename(context.Context, conversation.RenameConversation) (conversation.Conversation, error)
	MarkRead(context.Context, conversation.MarkRead) error
}

type conversationHandler struct {
	api      ConversationAPI
	notifier EventNotifier
}

const maxConversationTargetBytes = 2048

type conversationQueryKey struct{}

// RegisterConversations installs the authenticated conversation HTTP surface.
func RegisterConversations(
	mux *http.ServeMux,
	authAPI AuthAPI,
	api ConversationAPI,
	notifier EventNotifier,
	publicOrigin string,
) {
	handler := &conversationHandler{api: api, notifier: notifier}
	pageRead := func(next http.HandlerFunc) http.Handler {
		return disableAuthCaching(requireConversationTarget(validateConversationPageQuery,
			RequireSession(authAPI, next)))
	}
	read := func(next http.HandlerFunc) http.Handler {
		return disableAuthCaching(requireConversationTarget(validateNoConversationQuery,
			RequireSession(authAPI, next)))
	}
	mutation := func(next http.HandlerFunc) http.Handler {
		return disableAuthCaching(requireConversationTarget(validateNoConversationQuery,
			requireMutationSecurity(publicOrigin, RequireSession(authAPI, next))))
	}
	mux.Handle("GET /api/v1/conversations", pageRead(handler.list))
	mux.Handle("POST /api/v1/conversations", mutation(handler.create))
	mux.Handle("POST /api/v1/conversations/{conversation_id}/forks", mutation(handler.fork))
	mux.Handle("GET /api/v1/conversations/{conversation_id}", read(handler.detail))
	mux.Handle("GET /api/v1/conversations/{conversation_id}/members", pageRead(handler.members))
	mux.Handle("POST /api/v1/conversations/{conversation_id}/members", mutation(handler.addMember))
	mux.Handle("DELETE /api/v1/conversations/{conversation_id}/members/{user_id}", mutation(handler.removeMember))
	mux.Handle("DELETE /api/v1/conversations/{conversation_id}", mutation(handler.delete))
	mux.Handle("PATCH /api/v1/conversations/{conversation_id}", mutation(handler.rename))
	mux.Handle("PUT /api/v1/conversations/{conversation_id}/read", mutation(handler.markRead))
}

func (h *conversationHandler) rename(w http.ResponseWriter, request *http.Request) {
	conversationID, err := positivePathID(request, "conversation_id")
	var body struct {
		Name           string `json:"name"`
		IdempotencyKey string `json:"idempotency_key"`
	}
	if err != nil || decodeAuthJSON(w, request, &body) != nil {
		writeInvalidRequest(w)
		return
	}
	user, _ := UserFromContext(request.Context())
	renamed, err := h.api.Rename(request.Context(), conversation.RenameConversation{
		ActorID: user.ID, ConversationID: conversationID, Name: body.Name, IdempotencyKey: body.IdempotencyKey,
	})
	if writeConversationProblem(w, err) {
		return
	}
	h.notifier.Notify(0)
	writeJSON(w, http.StatusOK, renamed)
}

type createConversationRequest struct {
	Kind           conversation.Kind `json:"kind"`
	Name           string            `json:"name"`
	OtherUserID    int64             `json:"other_user_id"`
	MemberIDs      []int64           `json:"member_ids"`
	IdempotencyKey string            `json:"idempotency_key"`
}

func (h *conversationHandler) create(w http.ResponseWriter, request *http.Request) {
	var body createConversationRequest
	if decodeAuthJSON(w, request, &body) != nil {
		writeInvalidRequest(w)
		return
	}
	user, _ := UserFromContext(request.Context())
	var created conversation.Conversation
	var err error
	switch body.Kind {
	case conversation.KindDM:
		if body.Name != "" || len(body.MemberIDs) > 0 {
			writeInvalidRequest(w)
			return
		}
		created, err = h.api.CreateDM(request.Context(), conversation.CreateDM{
			RequesterID: user.ID, OtherUserID: body.OtherUserID, IdempotencyKey: body.IdempotencyKey,
		})
	case conversation.KindChannel, conversation.KindPrivate:
		if body.OtherUserID != 0 || (body.Kind == conversation.KindChannel && len(body.MemberIDs) > 0) {
			writeInvalidRequest(w)
			return
		}
		created, err = h.api.CreateChannel(request.Context(), conversation.CreateChannel{
			CreatorID: user.ID, Kind: body.Kind, Name: body.Name, MemberIDs: body.MemberIDs, IdempotencyKey: body.IdempotencyKey,
		})
	default:
		err = conversation.ErrInvalidInput
	}
	if writeConversationProblem(w, err) {
		return
	}
	h.notifier.Notify(0)
	writeJSON(w, http.StatusCreated, created)
}

func (h *conversationHandler) delete(w http.ResponseWriter, request *http.Request) {
	conversationID, err := positivePathID(request, "conversation_id")
	if err != nil {
		writeConversationProblem(w, err)
		return
	}
	user, _ := UserFromContext(request.Context())
	err = h.api.Delete(request.Context(), conversation.DeleteConversation{ActorID: user.ID, ConversationID: conversationID})
	if writeConversationProblem(w, err) {
		return
	}
	h.notifier.Notify(0)
	w.WriteHeader(http.StatusNoContent)
}

func (h *conversationHandler) markRead(w http.ResponseWriter, request *http.Request) {
	conversationID, err := positivePathID(request, "conversation_id")
	if err != nil {
		writeConversationProblem(w, err)
		return
	}
	user, _ := UserFromContext(request.Context())
	err = h.api.MarkRead(request.Context(), conversation.MarkRead{UserID: user.ID, ConversationID: conversationID})
	if writeConversationProblem(w, err) {
		return
	}
	h.notifier.Notify(0)
	w.WriteHeader(http.StatusNoContent)
}

func (h *conversationHandler) list(w http.ResponseWriter, request *http.Request) {
	beforeID, limit, err := boundedPage(conversationQuery(request.Context()))
	if err != nil {
		writeConversationProblem(w, err)
		return
	}
	user, _ := UserFromContext(request.Context())
	page, err := h.api.List(request.Context(), conversation.ListConversations{
		UserID: user.ID, BeforeID: beforeID, Limit: limit,
	})
	if writeConversationProblem(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *conversationHandler) detail(w http.ResponseWriter, request *http.Request) {
	conversationID, err := positivePathID(request, "conversation_id")
	if err != nil {
		writeConversationProblem(w, err)
		return
	}
	user, _ := UserFromContext(request.Context())
	item, err := h.api.Detail(request.Context(), user.ID, conversationID)
	if writeConversationProblem(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *conversationHandler) members(w http.ResponseWriter, request *http.Request) {
	conversationID, err := positivePathID(request, "conversation_id")
	if err != nil {
		writeConversationProblem(w, err)
		return
	}
	beforeID, limit, err := boundedPage(conversationQuery(request.Context()))
	if err != nil {
		writeConversationProblem(w, err)
		return
	}
	user, _ := UserFromContext(request.Context())
	page, err := h.api.Members(request.Context(), conversation.ListMembers{
		UserID: user.ID, ConversationID: conversationID, BeforeID: beforeID, Limit: limit,
	})
	if writeConversationProblem(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, page)
}

type memberMutationRequest struct {
	UserID         int64  `json:"user_id"`
	IdempotencyKey string `json:"idempotency_key"`
}

func (h *conversationHandler) addMember(w http.ResponseWriter, request *http.Request) {
	conversationID, err := positivePathID(request, "conversation_id")
	var body memberMutationRequest
	if err != nil || decodeAuthJSON(w, request, &body) != nil {
		writeInvalidRequest(w)
		return
	}
	user, _ := UserFromContext(request.Context())
	err = h.api.AddMember(request.Context(), conversation.ChangeMember{
		ActorID: user.ID, ConversationID: conversationID, UserID: body.UserID, IdempotencyKey: body.IdempotencyKey,
	})
	if writeConversationProblem(w, err) {
		return
	}
	h.notifier.Notify(0)
	w.WriteHeader(http.StatusNoContent)
}

func (h *conversationHandler) removeMember(w http.ResponseWriter, request *http.Request) {
	conversationID, err := positivePathID(request, "conversation_id")
	userID, userErr := positivePathID(request, "user_id")
	var body struct {
		IdempotencyKey string `json:"idempotency_key"`
	}
	if err != nil || userErr != nil || decodeAuthJSON(w, request, &body) != nil {
		writeInvalidRequest(w)
		return
	}
	user, _ := UserFromContext(request.Context())
	err = h.api.RemoveMember(request.Context(), conversation.ChangeMember{
		ActorID: user.ID, ConversationID: conversationID, UserID: userID, IdempotencyKey: body.IdempotencyKey,
	})
	if writeConversationProblem(w, err) {
		return
	}
	h.notifier.Notify(0)
	w.WriteHeader(http.StatusNoContent)
}

func positivePathID(request *http.Request, name string) (int64, error) {
	id, err := strconv.ParseInt(request.PathValue(name), 10, 64)
	if err != nil || id <= 0 {
		return 0, conversation.ErrInvalidInput
	}
	return id, nil
}

type conversationQueryPolicy func(url.Values) error

func requireConversationTarget(policy conversationQueryPolicy, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if len(request.URL.EscapedPath()) > maxConversationTargetBytes || len(request.URL.RawQuery) > maxConversationTargetBytes {
			writeConversationProblem(w, conversation.ErrInvalidInput)
			return
		}
		values, err := url.ParseQuery(request.URL.RawQuery)
		if err != nil {
			writeConversationProblem(w, conversation.ErrInvalidInput)
			return
		}
		if err := policy(values); err != nil {
			writeConversationProblem(w, err)
			return
		}
		ctx := context.WithValue(request.Context(), conversationQueryKey{}, values)
		next.ServeHTTP(w, request.WithContext(ctx))
	})
}

func conversationQuery(ctx context.Context) url.Values {
	values, _ := ctx.Value(conversationQueryKey{}).(url.Values)
	return values
}

func validateConversationPageQuery(values url.Values) error {
	_, _, err := boundedPage(values)
	return err
}

func validateNoConversationQuery(values url.Values) error {
	if len(values) != 0 {
		return conversation.ErrInvalidInput
	}
	return nil
}

func boundedPage(values url.Values) (int64, int, error) {
	for key, entries := range values {
		if (key != "before_id" && key != "limit") || len(entries) != 1 || entries[0] == "" {
			return 0, 0, conversation.ErrInvalidInput
		}
	}
	var beforeID int64
	var limit int
	var err error
	if value := values.Get("before_id"); value != "" {
		beforeID, err = strconv.ParseInt(value, 10, 64)
		if err != nil || beforeID <= 0 {
			return 0, 0, conversation.ErrInvalidInput
		}
	}
	if value := values.Get("limit"); value != "" {
		limit, err = strconv.Atoi(value)
		if err != nil || limit <= 0 || limit > conversation.MaxPageLimit {
			return 0, 0, conversation.ErrInvalidInput
		}
	}
	return beforeID, limit, nil
}

func writeConversationProblem(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	problem := Problem{Status: http.StatusInternalServerError, Code: "internal_error", Detail: "request could not be completed"}
	switch {
	case errors.Is(err, conversation.ErrInvalidInput):
		problem = Problem{Status: 400, Code: "invalid_request", Detail: "request is invalid"}
	case errors.Is(err, conversation.ErrNotFound):
		problem = Problem{Status: 404, Code: "not_found", Detail: "resource was not found"}
	case errors.Is(err, conversation.ErrConflict):
		problem = Problem{Status: 409, Code: "conversation_conflict", Detail: "conversation request conflicts with existing state"}
	case errors.Is(err, conversation.ErrForbidden):
		problem = Problem{Status: 403, Code: "forbidden", Detail: "you are not allowed to perform this action"}
	case errors.Is(err, conversation.ErrBusy):
		problem = Problem{Status: 503, Code: "temporarily_unavailable", Detail: "service is temporarily unavailable"}
	}
	WriteProblem(w, problem)
	return true
}
