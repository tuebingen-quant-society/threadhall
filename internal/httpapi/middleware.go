package httpapi

import (
	"context"
	"crypto/subtle"
	"net/http"

	"github.com/tuebingen-quant-society/threadhall/internal/auth"
)

type userContextKey struct{}

func disableAuthCaching(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, request)
	})
}

// RequireSession authenticates a canonical session cookie and places its user
// in the request context without rotating or otherwise mutating the session.
func RequireSession(api AuthAPI, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		user, ok := authenticateRequest(request, api)
		if !ok {
			WriteProblem(w, Problem{
				Status: http.StatusUnauthorized, Code: "authentication_required", Detail: "authentication is required",
			})
			return
		}
		ctx := context.WithValue(request.Context(), userContextKey{}, user)
		next.ServeHTTP(w, request.WithContext(ctx))
	})
}

// UserFromContext returns the identity established by RequireSession.
func UserFromContext(ctx context.Context) (auth.User, bool) {
	user, ok := ctx.Value(userContextKey{}).(auth.User)
	return user, ok
}

func authenticateRequest(request *http.Request, api AuthAPI) (auth.User, bool) {
	cookie, err := request.Cookie(sessionCookieName)
	if err != nil {
		return auth.User{}, false
	}
	raw, err := auth.DecodeToken(cookie.Value)
	if err != nil {
		return auth.User{}, false
	}
	user, err := api.Authenticate(request.Context(), raw)
	return user, err == nil
}

func requireMutationSecurity(publicOrigin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Origin") != publicOrigin {
			WriteProblem(w, Problem{
				Status: http.StatusForbidden, Code: "origin_forbidden", Detail: "request origin is not allowed",
			})
			return
		}
		cookie, err := request.Cookie(csrfCookieName)
		if err != nil || !matchingCSRF(cookie.Value, request.Header.Get(csrfHeaderName)) {
			WriteProblem(w, Problem{
				Status: http.StatusForbidden, Code: "csrf_invalid", Detail: "CSRF validation failed",
			})
			return
		}
		next.ServeHTTP(w, request)
	})
}

func matchingCSRF(cookieValue, headerValue string) bool {
	cookieToken, err := auth.DecodeToken(cookieValue)
	if err != nil {
		return false
	}
	headerToken, err := auth.DecodeToken(headerValue)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(cookieToken[:], headerToken[:]) == 1
}
