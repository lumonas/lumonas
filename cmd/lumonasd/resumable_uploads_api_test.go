package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/shares"
)

func TestResumableUploadAPIRoundTrip(t *testing.T) {
	server := testServer(t)
	root := t.TempDir()
	shareRoot := filepath.Join(root, "share")
	if err := os.Mkdir(shareRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := server.store.CreateManagedShare(shares.ManagedShare{ID: "upload-share", Name: "Upload", Path: shareRoot, Enabled: true, Protocols: []shares.Protocol{{Name: "smb"}}}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LUMONAS_UPLOAD_SESSION_DIR", filepath.Join(root, "sessions"))

	created := httptest.NewRecorder()
	server.routes().ServeHTTP(created, httptest.NewRequest(http.MethodPost, "/api/v1/files/uploads", strings.NewReader(fmt.Sprintf(`{"shareId":"upload-share","path":"/","name":"large.bin","sizeBytes":%d}`, len("resumable data")))))
	if created.Code != http.StatusCreated {
		t.Fatalf("create upload status %d: %s", created.Code, created.Body.String())
	}
	var session struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &session); err != nil || session.ID == "" {
		t.Fatalf("invalid upload session: %#v err=%v", session, err)
	}
	chunk := httptest.NewRecorder()
	chunkRequest := httptest.NewRequest(http.MethodPut, "/api/v1/files/uploads/"+session.ID, strings.NewReader("resumable data"))
	chunkRequest.Header.Set("Upload-Offset", "0")
	server.routes().ServeHTTP(chunk, chunkRequest)
	if chunk.Code != http.StatusNoContent || chunk.Header().Get("Upload-Offset") != "14" {
		t.Fatalf("write chunk status %d offset=%q: %s", chunk.Code, chunk.Header().Get("Upload-Offset"), chunk.Body.String())
	}
	completed := httptest.NewRecorder()
	server.routes().ServeHTTP(completed, httptest.NewRequest(http.MethodPost, "/api/v1/files/uploads/"+session.ID+"/complete", nil))
	if completed.Code != http.StatusCreated {
		t.Fatalf("complete upload status %d: %s", completed.Code, completed.Body.String())
	}
	var completion struct {
		JobID string `json:"jobId"`
	}
	if err := json.Unmarshal(completed.Body.Bytes(), &completion); err != nil || completion.JobID == "" {
		t.Fatalf("invalid completion response %s: %v", completed.Body.String(), err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		job, err := server.store.Job(completion.JobID)
		if err == nil && (job.State == "successful" || job.State == "failed") {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if data, err := os.ReadFile(filepath.Join(shareRoot, "large.bin")); err != nil || string(data) != "resumable data" {
		t.Fatalf("uploaded file = %q err=%v", data, err)
	}
}
