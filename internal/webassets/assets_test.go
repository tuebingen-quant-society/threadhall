package webassets

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerCachesHashedAssetsImmutably(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, embeddedAssetPath(t), nil)
	recorder := httptest.NewRecorder()

	Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	const expected = "public, max-age=31536000, immutable"
	if actual := recorder.Header().Get("Cache-Control"); actual != expected {
		t.Fatalf("Cache-Control = %q, want %q", actual, expected)
	}
}

func TestHandlerRevalidatesShellEntryPoints(t *testing.T) {
	for _, path := range []string{"/", "/index.html", "/sw.js", "/manifest.webmanifest"} {
		t.Run(path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, path, nil)
			recorder := httptest.NewRecorder()

			Handler().ServeHTTP(recorder, request)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
			}
			if actual := recorder.Header().Get("Cache-Control"); actual != "no-cache" {
				t.Fatalf("Cache-Control = %q, want %q", actual, "no-cache")
			}
		})
	}
}

func TestHandlerServesShellForBrowserNavigation(t *testing.T) {
	request := navigationRequest("/conversations/example")
	recorder := httptest.NewRecorder()

	Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if actual := recorder.Header().Get("Cache-Control"); actual != "no-cache" {
		t.Fatalf("Cache-Control = %q, want %q", actual, "no-cache")
	}
	body, err := io.ReadAll(recorder.Result().Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(body), `<div id="app"></div>`) {
		t.Fatal("response is not the embedded application shell")
	}
}

func TestHandlerDoesNotFallbackForReservedOrNonNavigationPaths(t *testing.T) {
	tests := []struct {
		name    string
		request *http.Request
	}{
		{name: "api", request: navigationRequest("/api/v1/unknown")},
		{name: "health", request: navigationRequest("/healthz/unknown")},
		{name: "mcp", request: navigationRequest("/mcp/unknown")},
		{name: "manifest", request: navigationRequest("/manifest.webmanifest/unknown")},
		{name: "worker", request: navigationRequest("/sw.js/unknown")},
		{name: "icons", request: navigationRequest("/icons/unknown")},
		{name: "assets", request: navigationRequest("/assets/unknown")},
		{name: "extension", request: navigationRequest("/conversations/unknown.json")},
		{name: "not a navigation", request: httptest.NewRequest(http.MethodGet, "/conversations/example", nil)},
		{name: "not a GET", request: httptest.NewRequest(http.MethodPost, "/conversations/example", nil)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			Handler().ServeHTTP(recorder, test.request)

			if recorder.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
			}
		})
	}
}

func TestHandlerDoesNotMarkIconsImmutable(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/icons/threadhall-192.png", nil)
	recorder := httptest.NewRecorder()

	Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if actual := recorder.Header().Get("Cache-Control"); strings.Contains(actual, "immutable") {
		t.Fatalf("Cache-Control = %q, must not be immutable", actual)
	}
}

func navigationRequest(requestPath string) *http.Request {
	request := httptest.NewRequest(http.MethodGet, requestPath, nil)
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	request.Header.Set("Sec-Fetch-Mode", "navigate")
	return request
}

func embeddedAssetPath(t *testing.T) string {
	t.Helper()
	paths, err := fs.Glob(files, "dist/assets/*")
	if err != nil {
		t.Fatalf("list embedded assets: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("embedded build has no hashed assets")
	}
	return strings.TrimPrefix(paths[0], "dist")
}
