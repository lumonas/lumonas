package workstationbackup

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type workstationAPIFixture struct {
	t              *testing.T
	data           []byte
	session        uploadSession
	failFirstChunk bool
	chunks         int
}

type workstationRoundTripper struct{ fixture *workstationAPIFixture }

func (transport workstationRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	response := httptest.NewRecorder()
	transport.fixture.ServeHTTP(response, request)
	result := response.Result()
	result.Request = request
	return result, nil
}

func (fixture *workstationAPIFixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer workstation-token" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/files/uploads":
		var body struct {
			ShareID     string `json:"shareId"`
			Path        string `json:"path"`
			Name        string `json:"name"`
			SizeBytes   int64  `json:"sizeBytes"`
			ExpectedSHA string `json:"expectedSha256"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			fixture.t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		fixture.session = uploadSession{ID: "upload-session", ShareID: body.ShareID, Path: body.Path, Name: body.Name, SizeBytes: body.SizeBytes, ExpectedSHA: body.ExpectedSHA, State: "uploading"}
		writeFixtureJSON(w, http.StatusCreated, fixture.session)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/files/uploads/upload-session":
		fixture.session.ReceivedBytes = int64(len(fixture.data))
		writeFixtureJSON(w, http.StatusOK, fixture.session)
	case r.Method == http.MethodPut && r.URL.Path == "/api/v1/files/uploads/upload-session":
		offset, err := strconv.ParseInt(r.Header.Get("Upload-Offset"), 10, 64)
		if err != nil || offset != int64(len(fixture.data)) {
			w.WriteHeader(http.StatusConflict)
			return
		}
		chunk, err := io.ReadAll(r.Body)
		if err != nil {
			fixture.t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		fixture.data = append(fixture.data, chunk...)
		fixture.chunks++
		w.Header().Set("Upload-Offset", strconv.Itoa(len(fixture.data)))
		if fixture.failFirstChunk && fixture.chunks == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/files/uploads/upload-session/complete":
		digest := sha256.Sum256(fixture.data)
		if int64(len(fixture.data)) != fixture.session.SizeBytes || hex.EncodeToString(digest[:]) != fixture.session.ExpectedSHA {
			w.WriteHeader(http.StatusConflict)
			return
		}
		fixture.session.State = "completed"
		fixture.session.FinalName = fixture.session.Name
		writeFixtureJSON(w, http.StatusCreated, map[string]string{"jobId": "job-1", "name": fixture.session.FinalName})
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/files/download":
		if r.URL.Query().Get("share") != fixture.session.ShareID || r.URL.Query().Get("name") != fixture.session.Name {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fixture.data)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/files":
		if r.URL.Query().Get("share") != fixture.session.ShareID {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		writeFixtureJSON(w, http.StatusOK, map[string]any{"shareId": fixture.session.ShareID, "path": r.URL.Query().Get("path"), "entries": []map[string]any{{"name": fixture.session.Name, "type": "file"}, {"name": "notes.txt", "type": "file"}, {"name": "old-folder", "type": "dir"}}})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func writeFixtureJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func TestClientResumesEncryptedBackupAndRestoresFiles(t *testing.T) {
	fixture := &workstationAPIFixture{t: t, failFirstChunk: true}
	source := filepath.Join(t.TempDir(), "source")
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatal(err)
	}
	contents := bytes.Repeat([]byte("workstation backup test payload\n"), 2_000)
	if err := os.WriteFile(filepath.Join(source, "important.txt"), contents, 0o640); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	stateDir := t.TempDir()
	client, err := NewClient(Config{ServerURL: "https://nas.test", Token: "workstation-token", ShareID: "share-backup", Key: key, StateDir: stateDir, HTTP: &http.Client{Transport: workstationRoundTripper{fixture: fixture}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Backup(context.Background(), source, "/"); err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("first upload should fail after persisting its chunk, got %v", err)
	}
	filename, err := client.Backup(context.Background(), source, "/")
	if err != nil {
		t.Fatal(err)
	}
	if filename == "" || fixture.chunks != 1 {
		t.Fatalf("backup did not resume from the server offset: filename=%q chunks=%d", filename, fixture.chunks)
	}
	archives, err := client.ListArchives(context.Background(), "/")
	if err != nil || len(archives) != 1 || archives[0] != filename {
		t.Fatalf("client did not list its encrypted restore point: %#v err=%v", archives, err)
	}
	restored := filepath.Join(t.TempDir(), "restored")
	if err := client.Restore(context.Background(), filename, "/", restored, false); err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(filepath.Join(restored, "important.txt"))
	if err != nil || !bytes.Equal(actual, contents) {
		t.Fatalf("restored file differs from source: err=%v", err)
	}
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("completed upload left pending local state: %v", entries)
	}
}

func TestNewClientRequiresHTTPSOutsideLoopback(t *testing.T) {
	key := make([]byte, 32)
	for _, endpoint := range []string{"http://192.0.2.4", "https://user:secret@example.com", "https://example.com?token=value"} {
		if _, err := NewClient(Config{ServerURL: endpoint, Token: "token", ShareID: "share-backup", Key: key, StateDir: t.TempDir()}); err == nil {
			t.Errorf("unsafe endpoint accepted: %s", endpoint)
		}
	}
	if _, err := NewClient(Config{ServerURL: "https://nas.example", Token: "token", ShareID: "share-backup", Key: key, StateDir: t.TempDir()}); err != nil {
		t.Fatal(fmt.Errorf("valid HTTPS endpoint rejected: %w", err))
	}
}
