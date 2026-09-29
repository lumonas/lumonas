package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContentSearchRequiresIndexAndIndexesTextWithinShareOnly(t *testing.T) {
	server := testServer(t)
	shareID := testFileShare(t, server)
	share, _ := server.store.ManagedShare(shareID)
	if err := os.Mkdir(filepath.Join(share.Path, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(share.Path, "docs", "notes.txt"), []byte("the blue heron lives beside the river"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(share.Path, "docs", "image.jpg"), []byte("the blue heron"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("blue heron outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(share.Path, "linked")); err != nil {
		t.Fatal(err)
	}
	searchURL := "/api/v1/files/content-search?share=" + shareID + "&q=blue%20heron"
	missing := httptest.NewRecorder()
	server.routes().ServeHTTP(missing, httptest.NewRequest(http.MethodGet, searchURL, nil))
	if missing.Code != http.StatusConflict {
		t.Fatalf("missing index status %d: %s", missing.Code, missing.Body.String())
	}
	build := httptest.NewRecorder()
	server.routes().ServeHTTP(build, httptest.NewRequest(http.MethodPost, "/api/v1/files/search-index", strings.NewReader(`{"shareId":"`+shareID+`"}`)))
	if build.Code != http.StatusOK {
		t.Fatalf("build index status %d: %s", build.Code, build.Body.String())
	}
	var status struct {
		Documents int `json:"documents"`
		Skipped   int `json:"skipped"`
	}
	if err := json.NewDecoder(build.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if status.Documents < 1 || status.Skipped < 1 {
		t.Fatalf("unexpected index result: %#v", status)
	}
	search := httptest.NewRecorder()
	server.routes().ServeHTTP(search, httptest.NewRequest(http.MethodGet, searchURL, nil))
	if search.Code != http.StatusOK || !strings.Contains(search.Body.String(), "notes.txt") || strings.Contains(search.Body.String(), "secret.txt") || !strings.Contains(search.Body.String(), "blue heron") {
		t.Fatalf("content search status %d: %s", search.Code, search.Body.String())
	}
}

func TestContentSearchRejectsShortQueryAndOversizedIndexFiles(t *testing.T) {
	server := testServer(t)
	shareID := testFileShare(t, server)
	for _, query := range []string{"ab", strings.Repeat("x", 129)} {
		response := httptest.NewRecorder()
		server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/files/content-search?share="+shareID+"&q="+query, nil))
		if response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("invalid query accepted (%d chars): %d %s", len(query), response.Code, response.Body.String())
		}
	}
}
