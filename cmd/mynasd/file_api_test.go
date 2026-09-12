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

	"github.com/lumonas/lumonas/internal/shares"
)

func TestFileAPIProvidesSearchPropertiesDownloadAndDependencyConfirmation(t *testing.T) {
	server := testServer(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "report.txt"), []byte("important"), 0o660); err != nil {
		t.Fatal(err)
	}
	if _, err := server.store.CreateManagedShare(shares.ManagedShare{ID: "share-search", Name: "Search", Path: root, Enabled: true, Protocols: []shares.Protocol{{Name: "smb"}}}); err != nil {
		t.Fatal(err)
	}
	search := httptest.NewRecorder()
	server.routes().ServeHTTP(search, httptest.NewRequest(http.MethodGet, "/api/v1/files/search?share=share-search&path=/&q=report", nil))
	if search.Code != http.StatusOK || !strings.Contains(search.Body.String(), "report.txt") {
		t.Fatalf("search failed: %d %s", search.Code, search.Body.String())
	}
	properties := httptest.NewRecorder()
	server.routes().ServeHTTP(properties, httptest.NewRequest(http.MethodGet, "/api/v1/files/properties?share=share-search&path=/&name=report.txt", nil))
	if properties.Code != http.StatusOK || !strings.Contains(properties.Body.String(), `"sizeBytes":9`) {
		t.Fatalf("properties failed: %d %s", properties.Code, properties.Body.String())
	}
	download := httptest.NewRecorder()
	server.routes().ServeHTTP(download, httptest.NewRequest(http.MethodGet, "/api/v1/files/download?share=share-search&path=/&name=report.txt", nil))
	if download.Code != http.StatusOK || download.Body.String() != "important" {
		t.Fatalf("download failed: %d %q", download.Code, download.Body.String())
	}
	multipartBody := &bytes.Buffer{}
	writer := multipart.NewWriter(multipartBody)
	_ = writer.WriteField("shareId", "share-search")
	_ = writer.WriteField("path", "/")
	part, err := writer.CreateFormFile("file", "uploaded.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("uploaded"))
	_ = writer.Close()
	upload := httptest.NewRecorder()
	uploadRequest := httptest.NewRequest(http.MethodPost, "/api/v1/files/upload", multipartBody)
	uploadRequest.Header.Set("Content-Type", writer.FormDataContentType())
	server.routes().ServeHTTP(upload, uploadRequest)
	if upload.Code != http.StatusAccepted || !strings.Contains(upload.Body.String(), "uploaded.txt") {
		t.Fatalf("multipart upload failed: %d %s", upload.Code, upload.Body.String())
	}

	t.Setenv("MYNAS_DOCKER_APPDATA_ROOT", root)
	dependency := httptest.NewRecorder()
	server.routes().ServeHTTP(dependency, httptest.NewRequest(http.MethodPost, "/api/v1/files/delete", strings.NewReader(`{"shareId":"share-search","path":"/","names":["report.txt"]}`)))
	if dependency.Code != http.StatusConflict || !strings.Contains(dependency.Body.String(), "dependencyWarning") {
		t.Fatalf("expected dependency confirmation: %d %s", dependency.Code, dependency.Body.String())
	}
	confirmed := httptest.NewRecorder()
	server.routes().ServeHTTP(confirmed, httptest.NewRequest(http.MethodPost, "/api/v1/files/delete", strings.NewReader(`{"shareId":"share-search","path":"/","names":["report.txt"],"confirmDependencies":true}`)))
	if confirmed.Code != http.StatusOK {
		t.Fatalf("confirmed delete failed: %d %s", confirmed.Code, confirmed.Body.String())
	}
}

func TestFileAPIUsesShareRootAndRecycleBin(t *testing.T) {
	server := testServer(t)
	t.Setenv("MYNAS_SHARES_FILE", filepath.Join(t.TempDir(), "legacy-shares.json"))
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "Documents"), 0o770); err != nil {
		t.Fatal(err)
	}
	if _, err := server.store.CreateManagedShare(shares.ManagedShare{ID: "share-files", Name: "Documents", Path: root, Enabled: true, Protocols: []shares.Protocol{{Name: "smb"}}}); err != nil {
		t.Fatal(err)
	}

	listing := httptest.NewRecorder()
	server.routes().ServeHTTP(listing, httptest.NewRequest(http.MethodGet, "/api/v1/files?share=share-files&path=/Documents", nil))
	if listing.Code != http.StatusOK || !strings.Contains(listing.Body.String(), `"entries":[]`) {
		t.Fatalf("unexpected initial listing: %d %s", listing.Code, listing.Body.String())
	}

	mkdir := httptest.NewRecorder()
	server.routes().ServeHTTP(mkdir, httptest.NewRequest(http.MethodPost, "/api/v1/files/mkdir", strings.NewReader(`{"shareId":"share-files","path":"/Documents","name":"Archive"}`)))
	if mkdir.Code != http.StatusCreated {
		t.Fatalf("mkdir failed: %d %s", mkdir.Code, mkdir.Body.String())
	}

	upload := httptest.NewRecorder()
	server.routes().ServeHTTP(upload, httptest.NewRequest(http.MethodPost, "/api/v1/files/upload", strings.NewReader(`{"shareId":"share-files","path":"/Documents","name":"notes.txt","sizeBytes":4}`)))
	if upload.Code != http.StatusAccepted {
		t.Fatalf("upload failed: %d %s", upload.Code, upload.Body.String())
	}
	var uploaded struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(upload.Body).Decode(&uploaded); err != nil || uploaded.Name != "notes.txt" {
		t.Fatalf("unexpected upload response: %#v err=%v", uploaded, err)
	}

	remove := httptest.NewRecorder()
	server.routes().ServeHTTP(remove, httptest.NewRequest(http.MethodPost, "/api/v1/files/delete", strings.NewReader(`{"shareId":"share-files","path":"/Documents","names":["notes.txt"]}`)))
	if remove.Code != http.StatusOK || !strings.Contains(remove.Body.String(), `"deleted":1`) {
		t.Fatalf("delete failed: %d %s", remove.Code, remove.Body.String())
	}

	bin := httptest.NewRecorder()
	server.routes().ServeHTTP(bin, httptest.NewRequest(http.MethodGet, "/api/v1/files/recycle?share=share-files", nil))
	if bin.Code != http.StatusOK || !strings.Contains(bin.Body.String(), "notes.txt") {
		t.Fatalf("recycle listing failed: %d %s", bin.Code, bin.Body.String())
	}
	var recycled []struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(bin.Body).Decode(&recycled); err != nil || len(recycled) != 1 {
		t.Fatalf("unexpected recycle response: %#v err=%v", recycled, err)
	}

	restore := httptest.NewRecorder()
	server.routes().ServeHTTP(restore, httptest.NewRequest(http.MethodPost, "/api/v1/files/recycle/restore", strings.NewReader(`{"id":"`+recycled[0].ID+`"}`)))
	if restore.Code != http.StatusOK {
		t.Fatalf("restore failed: %d %s", restore.Code, restore.Body.String())
	}
	if _, err := os.Stat(filepath.Join(root, "Documents", "notes.txt")); err != nil {
		t.Fatal(err)
	}
}

func TestFileAPITransferReturnsConflictAndCompletesJob(t *testing.T) {
	server := testServer(t)
	t.Setenv("MYNAS_SHARES_FILE", filepath.Join(t.TempDir(), "legacy-shares.json"))
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "source"), 0o770); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "target"), 0o770); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "source", "file.txt"), []byte("copy me"), 0o660); err != nil {
		t.Fatal(err)
	}
	if _, err := server.store.CreateManagedShare(shares.ManagedShare{ID: "share-transfer", Name: "Transfer", Path: root, Enabled: true, Protocols: []shares.Protocol{{Name: "smb"}}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "target", "file.txt"), []byte("existing"), 0o660); err != nil {
		t.Fatal(err)
	}
	conflict := httptest.NewRecorder()
	server.routes().ServeHTTP(conflict, httptest.NewRequest(http.MethodPost, "/api/v1/files/transfer", strings.NewReader(`{"shareId":"share-transfer","sourcePath":"/source","names":["file.txt"],"targetShareId":"share-transfer","targetPath":"/target","op":"copy"}`)))
	if conflict.Code != http.StatusConflict || !strings.Contains(conflict.Body.String(), "file.txt") {
		t.Fatalf("expected transfer conflict: %d %s", conflict.Code, conflict.Body.String())
	}

	transfer := httptest.NewRecorder()
	server.routes().ServeHTTP(transfer, httptest.NewRequest(http.MethodPost, "/api/v1/files/transfer", strings.NewReader(`{"shareId":"share-transfer","sourcePath":"/source","names":["file.txt"],"targetShareId":"share-transfer","targetPath":"/target","op":"copy","conflict":"rename"}`)))
	if transfer.Code != http.StatusAccepted {
		t.Fatalf("transfer failed: %d %s", transfer.Code, transfer.Body.String())
	}
	var job struct {
		JobID string `json:"jobId"`
	}
	if err := json.NewDecoder(transfer.Body).Decode(&job); err != nil || job.JobID == "" {
		t.Fatalf("unexpected transfer response: %#v err=%v", job, err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		value, err := server.store.Job(job.JobID)
		if err == nil && value.State == "successful" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	value, err := server.store.Job(job.JobID)
	if err != nil || value.State != "successful" {
		t.Fatalf("transfer job did not complete: %#v err=%v", value, err)
	}
	if _, err := os.Stat(filepath.Join(root, "target", "file (2).txt")); err != nil {
		t.Fatal(err)
	}
}
