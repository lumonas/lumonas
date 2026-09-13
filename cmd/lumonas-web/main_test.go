package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandlerProxiesAPIAndHealthRequests(t *testing.T) {
	target, err := url.Parse("http://backend.invalid")
	if err != nil {
		t.Fatal(err)
	}
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v1/server" && r.URL.Path != "/healthz" {
			t.Fatalf("unexpected proxied path %s", r.URL.Path)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"status":"ok"}`)),
			Request:    r,
		}, nil
	})
	handler := newHandlerWithTransport(t.TempDir(), target, transport)
	for _, path := range []string{"/api/v1/server", "/healthz"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || response.Body.String() != `{"status":"ok"}` {
			t.Fatalf("unexpected response for %s: %d %q", path, response.Code, response.Body.String())
		}
		if response.Header().Get("X-Content-Type-Options") != "nosniff" || response.Header().Get("X-Frame-Options") != "DENY" {
			t.Fatalf("proxy response lost baseline security headers: %v", response.Header())
		}
	}
}

func TestHandlerMarksHTTPSProxyRequestsAndStaticResponses(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("index"), 0o644); err != nil {
		t.Fatal(err)
	}
	target, err := url.Parse("http://backend.invalid")
	if err != nil {
		t.Fatal(err)
	}
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if got := r.Header.Get("X-Forwarded-Proto"); got != "https" {
			t.Fatalf("expected HTTPS forwarding marker, got %q", got)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`ok`)), Header: make(http.Header), Request: r}, nil
	})
	handler := newHandlerWithTransport(root, target, transport)

	apiResponse := httptest.NewRecorder()
	handler.ServeHTTP(apiResponse, httptest.NewRequest(http.MethodGet, "https://lumonas.local/api/v1/server", nil))
	if apiResponse.Header().Get("Strict-Transport-Security") == "" {
		t.Fatal("HTTPS API response is missing HSTS")
	}

	staticResponse := httptest.NewRecorder()
	handler.ServeHTTP(staticResponse, httptest.NewRequest(http.MethodGet, "https://lumonas.local/", nil))
	if staticResponse.Header().Get("Strict-Transport-Security") == "" || staticResponse.Header().Get("Content-Security-Policy") != "frame-ancestors 'none'" {
		t.Fatalf("HTTPS static response is missing security headers: %v", staticResponse.Header())
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

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
