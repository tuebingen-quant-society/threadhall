package httpapi

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

const maxRealtimeTargetBytes = 2048

// EventNotifier wakes the singleton ordered durable-event tailer.
type EventNotifier interface{ Notify(int64) }

// SocketAPI is the authenticated WebSocket transport boundary.
type SocketAPI interface {
	Serve(http.ResponseWriter, *http.Request, int64, int64)
}

type realtimeCursorKey struct{}

// RegisterRealtime installs the read-only browser event stream.
func RegisterRealtime(
	mux *http.ServeMux,
	authAPI AuthAPI,
	socket SocketAPI,
	publicOrigin string,
) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		user, _ := UserFromContext(request.Context())
		afterSeq, _ := request.Context().Value(realtimeCursorKey{}).(int64)
		socket.Serve(w, request, user.ID, afterSeq)
	})
	secured := requireExactOrigin(publicOrigin, RequireSession(authAPI, handler))
	mux.Handle("GET /api/v1/realtime", disableAuthCaching(preflightRealtime(secured)))
}

func preflightRealtime(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if len(request.URL.EscapedPath()) > maxRealtimeTargetBytes ||
			len(request.URL.RawQuery) > maxRealtimeTargetBytes {
			writeRealtimeInvalid(w)
			return
		}
		values, err := url.ParseQuery(request.URL.RawQuery)
		if err != nil {
			writeRealtimeInvalid(w)
			return
		}
		for key, entries := range values {
			if key != "after_seq" || len(entries) != 1 || entries[0] == "" {
				writeRealtimeInvalid(w)
				return
			}
		}
		var afterSeq int64
		if value := values.Get("after_seq"); value != "" {
			for _, character := range value {
				if character < '0' || character > '9' {
					writeRealtimeInvalid(w)
					return
				}
			}
			afterSeq, err = strconv.ParseInt(value, 10, 64)
			if err != nil {
				writeRealtimeInvalid(w)
				return
			}
		}
		ctx := context.WithValue(request.Context(), realtimeCursorKey{}, afterSeq)
		next.ServeHTTP(w, request.WithContext(ctx))
	})
}

func requireExactOrigin(publicOrigin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Origin") != publicOrigin {
			WriteProblem(w, Problem{
				Status: http.StatusForbidden, Code: "origin_forbidden", Detail: "request origin is not allowed",
			})
			return
		}
		next.ServeHTTP(w, request)
	})
}

func writeRealtimeInvalid(w http.ResponseWriter) {
	WriteProblem(w, Problem{
		Status: http.StatusBadRequest, Code: "invalid_request", Detail: "request is invalid",
	})
}
