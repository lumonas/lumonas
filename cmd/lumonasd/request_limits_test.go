package main

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestMiddlewareRejectsOversizedJSONBeforeHandler(t *testing.T) {
	server := testServer(t)
	called := false
	handler := server.requestMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/settings", strings.NewReader("{}"))
	request.ContentLength = (32 << 20) + 1
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusRequestEntityTooLarge || called {
		t.Fatalf("oversized JSON was not rejected before handler: status=%d called=%v", response.Code, called)
	}
}

func TestRequestMiddlewareRejectsOversizedMultipartBeforeHandler(t *testing.T) {
	server := testServer(t)
	called := false
	handler := server.requestMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/files/upload", strings.NewReader("multipart"))
	request.Header.Set("Content-Type", "multipart/form-data; boundary=ignored")
	request.ContentLength = (2 << 30) + 1
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusRequestEntityTooLarge || called {
		t.Fatalf("oversized multipart was not rejected before handler: status=%d called=%v", response.Code, called)
	}
}

func TestRequestMiddlewareDoesNotParseMultipartBeforeHandler(t *testing.T) {
	server := testServer(t)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormField("name")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(part, "value")
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	handler := server.requestMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.MultipartForm != nil {
			t.Error("middleware parsed multipart data before authentication and routing")
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("handler could not parse bounded multipart request: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/files/upload", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("bounded multipart request failed: status=%d body=%s", response.Code, response.Body.String())
	}
}
