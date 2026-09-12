package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lumonas/lumonas/internal/shares"
)

func testFileShare(t *testing.T, server *apiServer) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "Documents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "readme.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Documents", "notes.md"), []byte("# Notes"), 0o644); err != nil {
		t.Fatal(err)
	}
	shareID := "share-test-files"
	if _, err := server.store.CreateManagedShare(shares.ManagedShare{
		ID: shareID, Name: "TestFiles", Path: root, Enabled: true,
		Protocols: []shares.Protocol{{Name: "smb"}},
	}); err != nil {
		t.Fatal(err)
	}
	return shareID
}

func TestFilesAPIListDirectory(t *testing.T) {
	server := testServer(t)
	shareID := testFileShare(t, server)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/files?share="+shareID+"&path=/", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list failed: %d %s", rec.Code, rec.Body.String())
	}
	var result struct {
		ShareID string `json:"shareId"`
		Path    string `json:"path"`
		Entries []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"entries"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.ShareID != shareID {
		t.Fatalf("expected share %s, got %s", shareID, result.ShareID)
	}
	if len(result.Entries) < 2 {
		t.Fatalf("expected at least 2 entries, got %d", len(result.Entries))
	}
}

func TestFilesAPIMkdirAndRename(t *testing.T) {
	server := testServer(t)
	shareID := testFileShare(t, server)

	mkdirReq := httptest.NewRequest(http.MethodPost, "/api/v1/files/mkdir",
		strings.NewReader(`{"shareId":"`+shareID+`","path":"/","name":"NewFolder"}`))
	mkdirRec := httptest.NewRecorder()
	server.routes().ServeHTTP(mkdirRec, mkdirReq)
	if mkdirRec.Code != http.StatusCreated {
		t.Fatalf("mkdir failed: %d %s", mkdirRec.Code, mkdirRec.Body.String())
	}

	renameReq := httptest.NewRequest(http.MethodPost, "/api/v1/files/rename",
		strings.NewReader(`{"shareId":"`+shareID+`","path":"/","oldName":"NewFolder","newName":"RenamedFolder"}`))
	renameRec := httptest.NewRecorder()
	server.routes().ServeHTTP(renameRec, renameReq)
	if renameRec.Code != http.StatusOK {
		t.Fatalf("rename failed: %d %s", renameRec.Code, renameRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/files?share="+shareID+"&path=/", nil)
	listRec := httptest.NewRecorder()
	server.routes().ServeHTTP(listRec, listReq)
	if !strings.Contains(listRec.Body.String(), "RenamedFolder") {
		t.Fatalf("rename not reflected in listing: %s", listRec.Body.String())
	}
}

func TestFilesAPIDuplicateMkdirReturns409(t *testing.T) {
	server := testServer(t)
	shareID := testFileShare(t, server)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/files/mkdir",
		strings.NewReader(`{"shareId":"`+shareID+`","path":"/","name":"Documents"}`))
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for duplicate, got %d", rec.Code)
	}
}

func TestFilesAPIDownload(t *testing.T) {
	server := testServer(t)
	shareID := testFileShare(t, server)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/download?share="+shareID+"&path=/&name=readme.txt", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("download failed: %d", rec.Code)
	}
	if rec.Body.String() != "hello" {
		t.Fatalf("expected content 'hello', got %q", rec.Body.String())
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "readme.txt") {
		t.Fatalf("expected Content-Disposition with filename, got %q", cd)
	}
}

func TestFilesAPISearch(t *testing.T) {
	server := testServer(t)
	shareID := testFileShare(t, server)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/search?share="+shareID+"&path=/&q=notes", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("search failed: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "notes.md") {
		t.Fatalf("search did not find notes.md: %s", rec.Body.String())
	}
}

func TestFilesAPIProperties(t *testing.T) {
	server := testServer(t)
	shareID := testFileShare(t, server)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/properties?share="+shareID+"&path=/&name=readme.txt", nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("properties failed: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"name":"readme.txt"`) {
		t.Fatalf("unexpected properties: %s", rec.Body.String())
	}
}

func TestFilesAPIRecycleBinListEndpoint(t *testing.T) {
	server := testServer(t)
	shareID := testFileShare(t, server)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/recycle?share="+shareID, nil)
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("recycle list failed: %d", rec.Code)
	}
	var entries []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&entries); err != nil {
		t.Fatal(err)
	}
	if entries == nil {
		entries = []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}{}
	}
	_ = entries
}

func TestFilesAPIDeleteQueuesJob(t *testing.T) {
	server := testServer(t)
	shareID := testFileShare(t, server)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/files/delete",
		strings.NewReader(`{"shareId":"`+shareID+`","path":"/","names":["readme.txt"]}`))
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for async delete, got %d: %s", rec.Code, rec.Body.String())
	}
	var result struct {
		JobID string `json:"jobId"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.JobID == "" {
		t.Fatal("expected a job ID in the response")
	}
}

func TestFilesAPIMkdirRequiresName(t *testing.T) {
	server := testServer(t)
	shareID := testFileShare(t, server)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/files/mkdir",
		strings.NewReader(`{"shareId":"`+shareID+`","path":"/","name":""}`))
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)
	if rec.Code == http.StatusOK || rec.Code == http.StatusCreated {
		t.Fatalf("expected error for empty name, got %d", rec.Code)
	}
}

func TestFilesAPITransferInvalidOpReturns422(t *testing.T) {
	server := testServer(t)
	shareID := testFileShare(t, server)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/files/transfer",
		strings.NewReader(`{"shareId":"`+shareID+`","sourcePath":"/","names":["readme.txt"],"targetShareId":"`+shareID+`","targetPath":"/","op":"invalid"}`))
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for invalid op, got %d", rec.Code)
	}
}

func TestFilesAPIDeleteNonexistentShareReturns404(t *testing.T) {
	server := testServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/files/mkdir",
		strings.NewReader(`{"shareId":"nonexistent","path":"/","name":"test"}`))
	rec := httptest.NewRecorder()
	server.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for nonexistent share, got %d", rec.Code)
	}
}

func TestFilesAPIPurgeRecycleBin(t *testing.T) {
	server := testServer(t)
	shareID := testFileShare(t, server)

	purgeReq := httptest.NewRequest(http.MethodPost, "/api/v1/files/recycle/purge",
		strings.NewReader(`{"shareId":"`+shareID+`"}`))
	purgeRec := httptest.NewRecorder()
	server.routes().ServeHTTP(purgeRec, purgeReq)
	if purgeRec.Code != http.StatusOK {
		t.Fatalf("purge failed: %d %s", purgeRec.Code, purgeRec.Body.String())
	}
	var result struct {
		Purged int `json:"purged"`
	}
	if err := json.NewDecoder(purgeRec.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Purged < 0 {
		t.Fatalf("expected non-negative purged count, got %d", result.Purged)
	}
}
