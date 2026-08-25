package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/tuebingen-quant-society/threadhall/internal/auth"
)

const (
	sessionCookieName = "threadhall_session"
	csrfCookieName    = "threadhall_csrf"
	csrfHeaderName    = "X-CSRF-Token"
	maxAuthBodyBytes  = 2048
)

// AuthAPI is the HTTP-facing subset of auth.Service.
type AuthAPI interface {
	Login(context.Context, auth.Login) (auth.Session, error)
	RedeemInvite(context.Context, auth.CreateUser) (auth.Session, error)
	CreateInvite(context.Context, int64) (auth.Invite, error)
	Authenticate(context.Context, [32]byte) (auth.User, error)
	Revoke(context.Context, [32]byte) error
	NewCSRFToken() (string, error)
}

type authHandler struct {
	api           AuthAPI
	publicOrigin  string
	secureCookies bool
}

// RegisterAuth installs the complete human-authentication HTTP surface.
func RegisterAuth(mux *http.ServeMux, api AuthAPI, publicOrigin string, secureCookies bool) {
	handler := &authHandler{api: api, publicOrigin: publicOrigin, secureCookies: secureCookies}
	mux.Handle("GET /api/v1/session", disableAuthCaching(http.HandlerFunc(handler.getSession)))
	mux.Handle("POST /api/v1/session", disableAuthCaching(requireMutationSecurity(publicOrigin, http.HandlerFunc(handler.login))))
	mux.Handle("DELETE /api/v1/session", disableAuthCaching(requireMutationSecurity(publicOrigin, RequireSession(api, http.HandlerFunc(handler.logout)))))
	mux.Handle("POST /api/v1/invites", disableAuthCaching(requireMutationSecurity(publicOrigin, RequireSession(api, http.HandlerFunc(handler.createInvite)))))
	mux.Handle("POST /api/v1/users", disableAuthCaching(requireMutationSecurity(publicOrigin, http.HandlerFunc(handler.redeemInvite))))
}

func (h *authHandler) getSession(w http.ResponseWriter, request *http.Request) {
	if !h.ensureCSRF(w, request) {
		return
	}
	user, ok := authenticateRequest(request, h.api)
	if !ok {
		WriteProblem(w, Problem{
			Status: http.StatusUnauthorized, Code: "authentication_required", Detail: "authentication is required",
		})
		return
	}
	writeJSON(w, http.StatusOK, struct {
		User auth.User `json:"user"`
	}{User: user})
}

func (h *authHandler) login(w http.ResponseWriter, request *http.Request) {
	var command auth.Login
	if err := decodeAuthJSON(w, request, &command); err != nil {
		writeInvalidRequest(w)
		return
	}
	csrf, ok := h.generateCSRF(w)
	if !ok {
		return
	}
	session, err := h.api.Login(request.Context(), command)
	if errors.Is(err, auth.ErrInvalidCredentials) || errors.Is(err, auth.ErrInvalidInput) {
		WriteProblem(w, Problem{
			Status: http.StatusUnauthorized, Code: "invalid_credentials", Detail: "authentication failed",
		})
		return
	}
	if writeUnavailable(w, err) {
		return
	}
	if err != nil {
		writeInternalProblem(w)
		return
	}
	h.issueSessionCookies(w, session, csrf)
	writeJSON(w, http.StatusOK, sessionResponse(session))
}

func (h *authHandler) logout(w http.ResponseWriter, request *http.Request) {
	cookie, _ := request.Cookie(sessionCookieName)
	raw, err := auth.DecodeToken(cookie.Value)
	if err != nil {
		writeInternalProblem(w)
		return
	}
	if err := h.api.Revoke(request.Context(), raw); err != nil {
		if !writeUnavailable(w, err) {
			writeInternalProblem(w)
		}
		return
	}
	h.setCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: "", HttpOnly: true, MaxAge: -1, Expires: time.Unix(1, 0),
	})
	w.WriteHeader(http.StatusNoContent)
}

func (h *authHandler) createInvite(w http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	invite, err := h.api.CreateInvite(request.Context(), user.ID)
	if errors.Is(err, auth.ErrForbidden) {
		WriteProblem(w, Problem{Status: http.StatusForbidden, Code: "forbidden", Detail: "administrator access is required"})
		return
	}
	if writeUnavailable(w, err) {
		return
	}
	if err != nil {
		writeInternalProblem(w)
		return
	}
	writeJSON(w, http.StatusCreated, invite)
}

func (h *authHandler) redeemInvite(w http.ResponseWriter, request *http.Request) {
	var command auth.CreateUser
	if err := decodeAuthJSON(w, request, &command); err != nil {
		writeInvalidRequest(w)
		return
	}
	csrf, ok := h.generateCSRF(w)
	if !ok {
		return
	}
	session, err := h.api.RedeemInvite(request.Context(), command)
	if errors.Is(err, auth.ErrInvalidInput) || errors.Is(err, auth.ErrInvalidInvite) || errors.Is(err, auth.ErrUsernameUnavailable) {
		WriteProblem(w, Problem{Status: http.StatusBadRequest, Code: "account_creation_failed", Detail: "account could not be created"})
		return
	}
	if writeUnavailable(w, err) {
		return
	}
	if err != nil {
		writeInternalProblem(w)
		return
	}
	h.issueSessionCookies(w, session, csrf)
	writeJSON(w, http.StatusCreated, sessionResponse(session))
}

func (h *authHandler) issueSessionCookies(w http.ResponseWriter, session auth.Session, csrf string) {
	h.setCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: session.Token, HttpOnly: true, Expires: session.ExpiresAt,
	})
	h.setCookie(w, &http.Cookie{Name: csrfCookieName, Value: csrf})
}

func (h *authHandler) ensureCSRF(w http.ResponseWriter, request *http.Request) bool {
	if cookie, err := request.Cookie(csrfCookieName); err == nil {
		if _, err := auth.DecodeToken(cookie.Value); err == nil {
			return true
		}
	}
	token, ok := h.generateCSRF(w)
	if !ok {
		return false
	}
	h.setCookie(w, &http.Cookie{Name: csrfCookieName, Value: token})
	return true
}

func (h *authHandler) generateCSRF(w http.ResponseWriter) (string, bool) {
	token, err := h.api.NewCSRFToken()
	if err == nil {
		_, err = auth.DecodeToken(token)
	}
	if err != nil {
		writeInternalProblem(w)
		return "", false
	}
	return token, true
}

func (h *authHandler) setCookie(w http.ResponseWriter, cookie *http.Cookie) {
	cookie.Path = "/"
	cookie.Secure = h.secureCookies
	cookie.SameSite = http.SameSiteLaxMode
	http.SetCookie(w, cookie)
}

func decodeAuthJSON(w http.ResponseWriter, request *http.Request, destination any) error {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return auth.ErrInvalidInput
	}
	request.Body = http.MaxBytesReader(w, request.Body, maxAuthBodyBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return auth.ErrInvalidInput
	}
	return nil
}

func sessionResponse(session auth.Session) any {
	return struct {
		ExpiresAt time.Time `json:"expires_at"`
		User      auth.User `json:"user"`
	}{ExpiresAt: session.ExpiresAt, User: session.User}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeInvalidRequest(w http.ResponseWriter) {
	WriteProblem(w, Problem{Status: http.StatusBadRequest, Code: "invalid_request", Detail: "request body is invalid"})
}

func writeInternalProblem(w http.ResponseWriter) {
	WriteProblem(w, Problem{Status: http.StatusInternalServerError, Code: "internal_error", Detail: "request could not be completed"})
}

func writeUnavailable(w http.ResponseWriter, err error) bool {
	if !errors.Is(err, auth.ErrBusy) {
		return false
	}
	WriteProblem(w, Problem{
		Status: http.StatusServiceUnavailable, Code: "temporarily_unavailable", Detail: "service is temporarily unavailable",
	})
	return true
}
