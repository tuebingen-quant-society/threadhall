package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"unicode/utf8"

	"github.com/tuebingen-quant-society/threadhall/internal/auth"
)

const maxUserDirectoryTargetBytes = 512

type userDirectoryQueryKey struct{}

func requireUserDirectoryTarget(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if len(request.URL.RawQuery) > maxUserDirectoryTargetBytes {
			writeInvalidRequest(w)
			return
		}
		values, err := url.ParseQuery(request.URL.RawQuery)
		query, limit, valid := parseUserDirectoryQuery(values)
		if err != nil || !valid {
			writeInvalidRequest(w)
			return
		}
		ctx := context.WithValue(request.Context(), userDirectoryQueryKey{}, auth.FindUsers{Query: query, Limit: limit})
		next.ServeHTTP(w, request.WithContext(ctx))
	})
}

func parseUserDirectoryQuery(values url.Values) (string, int, bool) {
	for key, entries := range values {
		if (key != "query" && key != "limit") || len(entries) != 1 {
			return "", 0, false
		}
	}
	query := values.Get("query")
	if len(query) > 64 || !utf8.ValidString(query) {
		return "", 0, false
	}
	limit := 0
	if value := values.Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 || parsed > 50 {
			return "", 0, false
		}
		limit = parsed
	}
	return query, limit, true
}

func (h *authHandler) findUsers(w http.ResponseWriter, request *http.Request) {
	query, _ := request.Context().Value(userDirectoryQueryKey{}).(auth.FindUsers)
	user, _ := UserFromContext(request.Context())
	query.RequesterID = user.ID
	result, err := h.api.FindUsers(request.Context(), query)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidInput) {
			writeInvalidRequest(w)
		} else {
			writeInternalProblem(w)
		}
		return
	}
	writeJSON(w, http.StatusOK, result)
}
