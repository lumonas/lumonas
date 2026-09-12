package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestHandlerProxiesAPIAndHealthRequests(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/server" && r.URL.Path != "/healthz" {
			t.Fatalf("unexpected proxied path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer backend.Close()
	target, err := url.Parse(backend.URL)
	if err != nil {
		t.Fatal(err)
	}
	handler := newHandler(t.TempDir(), target)
	for _, path := range []string{"/api/v1/server", "/healthz"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || response.Body.String() != `{"status":"ok"}` {
			t.Fatalf("unexpected response for %s: %d %q", path, response.Code, response.Body.String())
		}
	}
}

func TestHandlerServesAssetsAndFallsBackToSPA(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("index"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "asset.txt"), []byte("asset"), 0o644); err != nil {
		t.Fatal(err)
	}
	target, _ := url.Parse("http://127.0.0.1:1")
	handler := newHandler(root, target)
	for _, item := range []struct {
		path string
		want string
	}{
		{path: "/asset.txt", want: "asset"},
		{path: "/dashboard", want: "index"},
		{path: "/", want: "index"},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, item.path, nil))
		if response.Code != http.StatusOK || response.Body.String() != item.want {
			t.Fatalf("unexpected response for %s: %d %q location=%q", item.path, response.Code, response.Body.String(), response.Header().Get("Location"))
		}
	}
}
