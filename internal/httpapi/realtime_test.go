package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tuebingen-quant-society/threadhall/internal/auth"
)

func TestRealtimeRequiresExactOriginAndCurrentSessionWithoutCSRF(t *testing.T) {
	authAPI := &fakeAuthAPI{user: auth.User{ID: 7, Username: "member"}}
	socket := &fakeSocketAPI{}
	handler := realtimeHTTPHandler(authAPI, socket)

	for _, test := range []struct {
		name, origin string
		session      bool
		want         int
	}{
		{name: "missing origin", session: true, want: http.StatusForbidden},
		{name: "different origin", origin: "https://evil.test", session: true, want: http.StatusForbidden},
		{name: "missing session", origin: testOrigin, want: http.StatusUnauthorized},
		{name: "valid without csrf", origin: testOrigin, session: true, want: http.StatusNoContent},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/v1/realtime?after_seq=9", nil)
			request.Header.Set("Origin", test.origin)
			if test.session {
				request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: tokenString(0x66)})
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != test.want || recorder.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("status/headers = %d/%#v, want %d", recorder.Code, recorder.Header(), test.want)
			}
		})
	}
	if socket.calls != 1 || socket.userID != 7 || socket.afterSeq != 9 {
		t.Fatalf("socket call = count %d user %d after %d", socket.calls, socket.userID, socket.afterSeq)
	}
}

func TestRealtimeRejectsInvalidCursorBeforeOriginAndAuthentication(t *testing.T) {
	handler := realtimeHTTPHandler(&fakeAuthAPI{}, &fakeSocketAPI{})
	for _, target := range []string{
		"/api/v1/realtime?after_seq=-1",
		"/api/v1/realtime?after_seq=+1",
		"/api/v1/realtime?after_seq=1&after_seq=2",
		"/api/v1/realtime?unknown=1",
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d, want 400", target, recorder.Code)
		}
	}
}

func realtimeHTTPHandler(authAPI AuthAPI, socket SocketAPI) http.Handler {
	mux := http.NewServeMux()
	RegisterRealtime(mux, authAPI, socket, testOrigin)
	return mux
}

type fakeSocketAPI struct {
	userID, afterSeq int64
	calls            int
}

func (s *fakeSocketAPI) Serve(w http.ResponseWriter, _ *http.Request, userID, afterSeq int64) {
	s.calls++
	s.userID, s.afterSeq = userID, afterSeq
	w.WriteHeader(http.StatusNoContent)
}
