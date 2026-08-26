package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tuebingen-quant-society/threadhall/internal/agenttask"
	"github.com/tuebingen-quant-society/threadhall/internal/auth"
)

type fakeCapabilityAPI struct {
	userID, conversationID int64
	items                  []agenttask.Capability
	err                    error
}

func (f *fakeCapabilityAPI) ConversationCapabilities(_ context.Context, userID, conversationID int64) ([]agenttask.Capability, error) {
	f.userID, f.conversationID = userID, conversationID
	return f.items, f.err
}

func TestCapabilitiesAreSessionAndConversationScoped(t *testing.T) {
	authAPI := &fakeAuthAPI{user: auth.User{ID: 7, Username: "lin"}}
	api := &fakeCapabilityAPI{items: []agenttask.Capability{{Kind: "skill", ID: "better-layout", Name: "Better Layout", Description: "Layout"}}}
	mux := http.NewServeMux()
	RegisterCapabilities(mux, authAPI, api)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/conversations/9/capabilities", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: tokenString(0x31)})
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)

	if response.Code != http.StatusOK || api.userID != 7 || api.conversationID != 9 || response.Body.String() !=
		"{\"capabilities\":[{\"kind\":\"skill\",\"id\":\"better-layout\",\"name\":\"Better Layout\",\"description\":\"Layout\"}]}\n" {
		t.Fatalf("capabilities response = %d %s; scope %d/%d", response.Code, response.Body.String(), api.userID, api.conversationID)
	}
}
