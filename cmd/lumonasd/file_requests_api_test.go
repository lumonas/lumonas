package main

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/store"
)

func TestFileRequestCapabilityUploadsWithinLimitsAndCanBeRevoked(t *testing.T) {
	server := testServer(t)
	shareID := testFileShare(t, server)
	share, err := server.store.ManagedShare(shareID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(share.Path, "incoming"), 0o755); err != nil {
		t.Fatal(err)
	}

	create := httptest.NewRecorder()
	server.routes().ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v1/file-requests", strings.NewReader(`{"shareId":"`+shareID+`","path":"incoming","expiresInHours":24,"maxFiles":1,"maxBytes":32}`)))
	if create.Code != http.StatusCreated {
		t.Fatalf("create status %d: %s", create.Code, create.Body.String())
	}
	var created struct {
		Request struct {
			ID        string `json:"id"`
			TokenHash string `json:"tokenHash"`
		} `json:"request"`
		URL string `json:"url"`
	}
	if err := json.NewDecoder(create.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Request.ID == "" || created.URL == "" || created.Request.TokenHash != "" {
		t.Fatalf("unexpected creation response: %#v", created)
	}
	token := created.URL[strings.LastIndex(created.URL, "/")+1:]

	meta := httptest.NewRecorder()
	server.routes().ServeHTTP(meta, httptest.NewRequest(http.MethodGet, "/api/v1/public/file-requests/"+token, nil))
	if meta.Code != http.StatusOK || !strings.Contains(meta.Body.String(), "TestFiles") {
		t.Fatalf("metadata status %d: %s", meta.Code, meta.Body.String())
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "from-phone.jpg")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("photo-bytes"))
	_ = writer.Close()
	upload := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/public/file-requests/"+token+"/upload", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	server.routes().ServeHTTP(upload, request)
	if upload.Code != http.StatusCreated {
		t.Fatalf("upload status %d: %s", upload.Code, upload.Body.String())
	}
	if data, err := os.ReadFile(filepath.Join(share.Path, "incoming", "from-phone.jpg")); err != nil || string(data) != "photo-bytes" {
		t.Fatalf("uploaded file: %q err=%v", data, err)
	}

	second := httptest.NewRecorder()
	server.routes().ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/api/v1/public/file-requests/"+token, nil))
	if second.Code != http.StatusOK || !strings.Contains(second.Body.String(), `"remainingFiles":0`) {
		t.Fatalf("consumption not recorded: %d %s", second.Code, second.Body.String())
	}

	revoke := httptest.NewRecorder()
	server.routes().ServeHTTP(revoke, httptest.NewRequest(http.MethodDelete, "/api/v1/file-requests/"+created.Request.ID, nil))
	if revoke.Code != http.StatusNoContent {
		t.Fatalf("revoke status %d: %s", revoke.Code, revoke.Body.String())
	}
	missing := httptest.NewRecorder()
	server.routes().ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/api/v1/public/file-requests/"+token, nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("revoked link still works: %d", missing.Code)
	}
}

func TestFileRequestRejectsExpiredLinksAndDoesNotFollowSymlinkPath(t *testing.T) {
	server := testServer(t)
	shareID := testFileShare(t, server)
	share, _ := server.store.ManagedShare(shareID)
	if err := os.Symlink(t.TempDir(), filepath.Join(share.Path, "outside")); err != nil {
		t.Fatal(err)
	}
	err := server.store.CreateFileRequest(store.FileRequest{ID: "expired", ShareID: shareID, TokenHash: hashFileRequestToken("expired-capability-token-1234567890"), CreatedAt: time.Now().UTC(), ExpiresAt: time.Now().Add(-time.Hour), MaxFiles: 1, MaxBytes: 20})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/public/file-requests/expired-capability-token-1234567890", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("expired request status %d", response.Code)
	}

	create := httptest.NewRecorder()
	server.routes().ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v1/file-requests", strings.NewReader(`{"shareId":"`+shareID+`","path":"outside","expiresInHours":24,"maxFiles":1,"maxBytes":10}`)))
	if create.Code != http.StatusUnprocessableEntity {
		t.Fatalf("symlink request path accepted: %d %s", create.Code, create.Body.String())
	}
}
