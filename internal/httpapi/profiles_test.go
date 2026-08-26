package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tuebingen-quant-society/threadhall/internal/auth"
)

func TestProfileAvatarHTTPUsesAuthenticatedUserAndServesPrivateRaster(t *testing.T) {
	authAPI := &fakeAuthAPI{user: auth.User{ID: 4, Username: "member"}}
	avatar := auth.Avatar{MIME: "image/png", Data: []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, UpdatedAt: time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)}
	api := &fakeProfileAPI{avatar: avatar}
	mux := http.NewServeMux()
	RegisterProfiles(mux, authAPI, api, testOrigin)
	csrf := tokenString(0x62)

	putRequest := mutationRequest(http.MethodPut, "/api/v1/profile/avatar", bytes.NewReader(avatar.Data), csrf)
	putRequest.Header.Set("Content-Type", "image/png")
	putRequest.AddCookie(&http.Cookie{Name: sessionCookieName, Value: tokenString(0x61)})
	put := httptest.NewRecorder()
	mux.ServeHTTP(put, putRequest)
	if put.Code != http.StatusNoContent || api.setUserID != 4 || api.setMIME != "image/png" || !bytes.Equal(api.setData, avatar.Data) {
		t.Fatalf("put = status %d user %d mime %q data %x", put.Code, api.setUserID, api.setMIME, api.setData)
	}

	getRequest := httptest.NewRequest(http.MethodGet, "/api/v1/users/7/avatar", nil)
	getRequest.AddCookie(&http.Cookie{Name: sessionCookieName, Value: tokenString(0x61)})
	get := httptest.NewRecorder()
	mux.ServeHTTP(get, getRequest)
	if get.Code != http.StatusOK || api.requesterID != 4 || api.targetID != 7 || get.Header().Get("Content-Type") != "image/png" || get.Header().Get("Cache-Control") != "private, max-age=3600" || !bytes.Equal(get.Body.Bytes(), avatar.Data) {
		t.Fatalf("get = status %d headers %#v body %x", get.Code, get.Header(), get.Body.Bytes())
	}

	deleteRequest := mutationRequest(http.MethodDelete, "/api/v1/profile/avatar", nil, csrf)
	deleteRequest.AddCookie(&http.Cookie{Name: sessionCookieName, Value: tokenString(0x61)})
	deleted := httptest.NewRecorder()
	mux.ServeHTTP(deleted, deleteRequest)
	if deleted.Code != http.StatusNoContent || api.deletedUserID != 4 {
		t.Fatalf("delete = status %d user %d", deleted.Code, api.deletedUserID)
	}
}

type fakeProfileAPI struct {
	avatar                                          auth.Avatar
	setUserID, requesterID, targetID, deletedUserID int64
	setMIME                                         string
	setData                                         []byte
}

func (a *fakeProfileAPI) SetAvatar(_ context.Context, userID int64, mime string, data []byte) error {
	a.setUserID, a.setMIME, a.setData = userID, mime, append([]byte(nil), data...)
	return nil
}
func (a *fakeProfileAPI) DeleteAvatar(_ context.Context, userID int64) error {
	a.deletedUserID = userID
	return nil
}
func (a *fakeProfileAPI) Avatar(_ context.Context, requesterID, targetID int64) (auth.Avatar, error) {
	a.requesterID, a.targetID = requesterID, targetID
	return a.avatar, nil
}
