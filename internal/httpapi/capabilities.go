package httpapi

import (
	"context"
	"net/http"

	"github.com/tuebingen-quant-society/threadhall/internal/agenttask"
)

type CapabilityAPI interface {
	ConversationCapabilities(context.Context, int64, int64) ([]agenttask.Capability, error)
}

func RegisterCapabilities(mux *http.ServeMux, authAPI AuthAPI, api CapabilityAPI) {
	mux.Handle("GET /api/v1/conversations/{conversation_id}/capabilities", disableAuthCaching(RequireSession(authAPI,
		http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			conversationID, err := positivePathID(request, "conversation_id")
			if err != nil {
				writeAgentProblem(w, agenttask.ErrInvalidInput)
				return
			}
			user, _ := UserFromContext(request.Context())
			items, err := api.ConversationCapabilities(request.Context(), user.ID, conversationID)
			if writeAgentProblem(w, err) {
				return
			}
			writeJSON(w, http.StatusOK, agenttask.CapabilityPage{Capabilities: items})
		}))))
}
