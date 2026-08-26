package httpapi

import (
	"net/http"

	"github.com/tuebingen-quant-society/threadhall/internal/conversation"
)

type forkConversationRequest struct {
	SourceMessageID int64             `json:"source_message_id"`
	Kind            conversation.Kind `json:"kind"`
	Name            string            `json:"name"`
	IdempotencyKey  string            `json:"idempotency_key"`
}

func (h *conversationHandler) fork(w http.ResponseWriter, request *http.Request) {
	sourceConversationID, err := positivePathID(request, "conversation_id")
	var body forkConversationRequest
	if err != nil || decodeAuthJSON(w, request, &body) != nil {
		writeInvalidRequest(w)
		return
	}
	user, _ := UserFromContext(request.Context())
	created, err := h.api.Fork(request.Context(), conversation.ForkConversation{
		ActorID: user.ID, SourceConversationID: sourceConversationID, SourceMessageID: body.SourceMessageID,
		Kind: body.Kind, Name: body.Name, IdempotencyKey: body.IdempotencyKey,
	})
	if writeConversationProblem(w, err) {
		return
	}
	h.notifier.Notify(0)
	writeJSON(w, http.StatusCreated, created)
}
