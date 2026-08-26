package httpapi

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"

	"github.com/tuebingen-quant-society/threadhall/internal/auth"
)

type ProfileAPI interface {
	SetAvatar(context.Context, int64, string, []byte) error
	DeleteAvatar(context.Context, int64) error
	Avatar(context.Context, int64, int64) (auth.Avatar, error)
}

type profileHandler struct{ api ProfileAPI }

func RegisterProfiles(mux *http.ServeMux, authAPI AuthAPI, api ProfileAPI, publicOrigin string) {
	handler := &profileHandler{api: api}
	mutation := func(next http.HandlerFunc) http.Handler {
		return disableAuthCaching(requireMutationSecurity(publicOrigin, RequireSession(authAPI, next)))
	}
	mux.Handle("PUT /api/v1/profile/avatar", mutation(handler.setAvatar))
	mux.Handle("DELETE /api/v1/profile/avatar", mutation(handler.deleteAvatar))
	mux.Handle("GET /api/v1/users/{user_id}/avatar", RequireSession(authAPI, http.HandlerFunc(handler.avatar)))
}

func (h *profileHandler) setAvatar(w http.ResponseWriter, request *http.Request) {
	if request.URL.RawQuery != "" {
		writeInvalidRequest(w)
		return
	}
	mimeType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || (mimeType != "image/png" && mimeType != "image/jpeg" && mimeType != "image/webp") {
		writeInvalidRequest(w)
		return
	}
	limited := http.MaxBytesReader(w, request.Body, auth.MaxAvatarBytes)
	data, err := io.ReadAll(limited)
	_ = limited.Close()
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		WriteProblem(w, Problem{Status: 413, Code: "request_too_large", Detail: "avatar is too large"})
		return
	}
	if err != nil || len(data) == 0 || http.DetectContentType(data) != mimeType {
		writeInvalidRequest(w)
		return
	}
	user, _ := UserFromContext(request.Context())
	if err := h.api.SetAvatar(request.Context(), user.ID, mimeType, data); err != nil {
		writeProfileProblem(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *profileHandler) deleteAvatar(w http.ResponseWriter, request *http.Request) {
	if request.URL.RawQuery != "" {
		writeInvalidRequest(w)
		return
	}
	user, _ := UserFromContext(request.Context())
	if err := h.api.DeleteAvatar(request.Context(), user.ID); err != nil {
		writeProfileProblem(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *profileHandler) avatar(w http.ResponseWriter, request *http.Request) {
	if !validAvatarQuery(request.URL.Query()) {
		writeInvalidRequest(w)
		return
	}
	targetID, err := positivePathID(request, "user_id")
	if err != nil {
		writeInvalidRequest(w)
		return
	}
	user, _ := UserFromContext(request.Context())
	avatar, err := h.api.Avatar(request.Context(), user.ID, targetID)
	if err != nil {
		writeProfileProblem(w, err)
		return
	}
	etag := fmt.Sprintf(`"%x"`, sha256.Sum256(avatar.Data))
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("Content-Type", avatar.MIME)
	w.Header().Set("ETag", etag)
	if request.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(avatar.Data)
}

func validAvatarQuery(values url.Values) bool {
	if len(values) == 0 {
		return true
	}
	entries, ok := values["v"]
	if !ok || len(values) != 1 || len(entries) != 1 || entries[0] == "" {
		return false
	}
	_, err := strconv.ParseInt(entries[0], 10, 64)
	return err == nil
}

func writeProfileProblem(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrInvalidInput):
		writeInvalidRequest(w)
	case errors.Is(err, auth.ErrCredentialNotFound):
		WriteProblem(w, Problem{Status: 404, Code: "not_found", Detail: "avatar was not found"})
	case errors.Is(err, auth.ErrBusy):
		WriteProblem(w, Problem{Status: 503, Code: "temporarily_unavailable", Detail: "service is temporarily unavailable"})
	default:
		writeInternalProblem(w)
	}
}
