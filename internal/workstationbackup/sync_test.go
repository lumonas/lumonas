package workstationbackup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

type syncFixture struct {
	t            *testing.T
	files        map[string]syncRemoteFile
	dirs         map[string]bool
	sessions     map[string]syncFixtureSession
	uploads      int
	replacements int
}

type syncRemoteFile struct {
	data     []byte
	modified time.Time
}
type syncFixtureSession struct {
	path, name, expected string
	size                 int64
	replace              bool
	expectedSize         int64
	expectedModified     time.Time
	data                 []byte
}

type syncRoundTripper struct{ fixture *syncFixture }

func (transport syncRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	response := httptest.NewRecorder()
	transport.fixture.ServeHTTP(response, request)
	result := response.Result()
	result.Request = request
	return result, nil
}

func (fixture *syncFixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer sync-token" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/files":
		if r.URL.Query().Get("share") != "sync-share" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		current := filepath.ToSlash(filepath.Clean("/" + r.URL.Query().Get("path")))
		current = strings.TrimPrefix(current, "/")
		if current == "" {
			current = "."
		}
		entries := []remoteEntry{}
		for directory := range fixture.dirs {
			if directory == current || path.Dir(directory) != current {
				continue
			}
			entries = append(entries, remoteEntry{Name: path.Base(directory), Type: "dir"})
		}
		for name, file := range fixture.files {
			if path.Dir(name) != current {
				continue
			}
			entries = append(entries, remoteEntry{Name: path.Base(name), Type: "file", SizeBytes: int64(len(file.data)), ModifiedAt: file.modified})
		}
		writeFixtureJSON(w, http.StatusOK, map[string]any{"entries": entries})
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/files/mkdir":
		var body struct {
			ShareID string `json:"shareId"`
			Path    string `json:"path"`
			Name    string `json:"name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.ShareID != "sync-share" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		parent := strings.TrimPrefix(filepath.ToSlash(filepath.Clean("/"+body.Path)), "/")
		if parent == "" {
			parent = "."
		}
		fixture.dirs[path.Join(parent, body.Name)] = true
		writeFixtureJSON(w, http.StatusCreated, map[string]bool{"ok": true})
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/files/properties":
		name := r.URL.Query().Get("name")
		parent := strings.TrimPrefix(filepath.ToSlash(filepath.Clean("/"+r.URL.Query().Get("path"))), "/")
		file, ok := fixture.files[path.Join(parent, name)]
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeFixtureJSON(w, http.StatusOK, map[string]any{"name": name, "type": "file", "sizeBytes": len(file.data), "modifiedAt": file.modified})
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/files/uploads":
		var body struct {
			ShareID          string    `json:"shareId"`
			Path             string    `json:"path"`
			Name             string    `json:"name"`
			Size             int64     `json:"sizeBytes"`
			SHA              string    `json:"expectedSha256"`
			Replace          bool      `json:"replaceExisting"`
			ExpectedSize     int64     `json:"expectedSizeBytes"`
			ExpectedModified time.Time `json:"expectedModifiedAt"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		id := "sync-upload"
		fixture.sessions[id] = syncFixtureSession{path: path.Join(body.Path, body.Name), name: body.Name, expected: body.SHA, size: body.Size, replace: body.Replace, expectedSize: body.ExpectedSize, expectedModified: body.ExpectedModified}
		writeFixtureJSON(w, http.StatusCreated, map[string]any{"id": id, "shareId": body.ShareID, "path": body.Path, "name": body.Name, "sizeBytes": body.Size, "receivedBytes": 0, "state": "uploading"})
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/files/uploads/sync-upload":
		session := fixture.sessions["sync-upload"]
		writeFixtureJSON(w, http.StatusOK, map[string]any{"id": "sync-upload", "shareId": "sync-share", "path": path.Dir(session.path), "name": session.name, "sizeBytes": session.size, "receivedBytes": len(session.data), "expectedSha256": session.expected, "state": "uploading"})
	case r.Method == http.MethodPut && r.URL.Path == "/api/v1/files/uploads/sync-upload":
		session := fixture.sessions["sync-upload"]
		offset, _ := strconv.Atoi(r.Header.Get("Upload-Offset"))
		if offset != len(session.data) {
			w.WriteHeader(http.StatusConflict)
			return
		}
		chunk, _ := io.ReadAll(r.Body)
		session.data = append(session.data, chunk...)
		fixture.sessions["sync-upload"] = session
		w.Header().Set("Upload-Offset", strconv.Itoa(len(session.data)))
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/files/uploads/sync-upload/complete":
		session := fixture.sessions["sync-upload"]
		digest := sha256.Sum256(session.data)
		if int64(len(session.data)) != session.size || hex.EncodeToString(digest[:]) != session.expected {
			w.WriteHeader(http.StatusConflict)
			return
		}
		if existing, ok := fixture.files[session.path]; ok {
			if !session.replace || int64(len(existing.data)) != session.expectedSize || !existing.modified.Equal(session.expectedModified) {
				w.WriteHeader(http.StatusConflict)
				return
			}
			fixture.replacements++
		} else if session.replace {
			w.WriteHeader(http.StatusConflict)
			return
		}
		fixture.files[session.path] = syncRemoteFile{data: append([]byte(nil), session.data...), modified: time.Now().UTC()}
		fixture.uploads++
		writeFixtureJSON(w, http.StatusCreated, map[string]string{"jobId": "job-sync", "name": session.name})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func newSyncClient(t *testing.T, fixture *syncFixture) *Client {
	t.Helper()
	client, err := NewClient(Config{ServerURL: "http://127.0.0.1:8080", Token: "sync-token", ShareID: "sync-share", StateDir: t.TempDir(), HTTP: &http.Client{Transport: syncRoundTripper{fixture: fixture}}})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestSyncPushAddsUpdatesAndPreservesRemoteOnlyFiles(t *testing.T) {
	fixture := &syncFixture{t: t, files: map[string]syncRemoteFile{".keep": {data: []byte("remote only"), modified: time.Now().UTC()}}, dirs: map[string]bool{".": true}, sessions: map[string]syncFixtureSession{}}
	client := newSyncClient(t, fixture)
	source := t.TempDir()
	if err := os.MkdirAll(filepath.Join(source, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "nested", "first.txt"), []byte("first contents"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "keep.txt"), []byte("local keep"), 0600); err != nil {
		t.Fatal(err)
	}
	report, err := client.SyncPush(context.Background(), SyncOptions{Source: source, RemotePath: "workstation"})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Added) != 2 || fixture.uploads != 2 || string(fixture.files["workstation/nested/first.txt"].data) != "first contents" || len(report.Conflicts) != 0 {
		t.Fatalf("initial push did not create expected files: report=%#v files=%#v", report, fixture.files)
	}
	if _, exists := fixture.files[".keep"]; !exists {
		t.Fatal("sync unexpectedly deleted a remote-only file")
	}
	second, err := client.SyncPush(context.Background(), SyncOptions{Source: source, RemotePath: "workstation"})
	if err != nil || len(second.Unchanged) != 2 || fixture.uploads != 2 {
		t.Fatalf("second run should skip unchanged files: %#v %v uploads=%d", second, err, fixture.uploads)
	}
	if err := os.WriteFile(filepath.Join(source, "nested", "first.txt"), []byte("updated contents"), 0600); err != nil {
		t.Fatal(err)
	}
	third, err := client.SyncPush(context.Background(), SyncOptions{Source: source, RemotePath: "workstation"})
	if err != nil || len(third.Updated) != 1 || fixture.replacements != 1 || string(fixture.files["workstation/nested/first.txt"].data) != "updated contents" {
		t.Fatalf("changed local file was not safely updated: %#v %v replacements=%d", third, err, fixture.replacements)
	}
}

func TestSyncPushDryRunAndConcurrentRemoteChangesBecomeConflicts(t *testing.T) {
	fixture := &syncFixture{t: t, files: map[string]syncRemoteFile{}, dirs: map[string]bool{".": true}, sessions: map[string]syncFixtureSession{}}
	client := newSyncClient(t, fixture)
	source := t.TempDir()
	file := filepath.Join(source, "notes.txt")
	if err := os.WriteFile(file, []byte("local version"), 0600); err != nil {
		t.Fatal(err)
	}
	dryRun, err := client.SyncPush(context.Background(), SyncOptions{Source: source, RemotePath: "workstation", DryRun: true})
	if err != nil || len(dryRun.Added) != 1 || fixture.uploads != 0 || fixture.dirs["workstation"] {
		t.Fatalf("dry run performed a mutation: %#v %v", dryRun, err)
	}
	if _, err := client.SyncPush(context.Background(), SyncOptions{Source: source, RemotePath: "workstation"}); err != nil {
		t.Fatal(err)
	}
	remote := fixture.files["workstation/notes.txt"]
	remote.data = []byte("edited remotely")
	remote.modified = remote.modified.Add(time.Second)
	fixture.files["workstation/notes.txt"] = remote
	report, err := client.SyncPush(context.Background(), SyncOptions{Source: source, RemotePath: "workstation"})
	if err != nil || len(report.Conflicts) != 1 || fixture.replacements != 0 {
		t.Fatalf("remote edit was overwritten without consent: %#v %v", report, err)
	}
	accepted, err := client.SyncPush(context.Background(), SyncOptions{Source: source, RemotePath: "workstation", Overwrite: true})
	if err != nil || len(accepted.Updated) != 1 || fixture.replacements != 1 || !bytes.Equal(fixture.files["workstation/notes.txt"].data, []byte("local version")) {
		t.Fatalf("explicit overwrite did not replace the reviewed version: %#v %v", accepted, err)
	}
}

func TestSyncPushSkipsSymlinksAndRejectsTraversal(t *testing.T) {
	fixture := &syncFixture{t: t, files: map[string]syncRemoteFile{}, dirs: map[string]bool{".": true}, sessions: map[string]syncFixtureSession{}}
	client := newSyncClient(t, fixture)
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "ok.txt"), []byte("safe"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(source, "ok.txt"), filepath.Join(source, "link.txt")); err == nil {
		report, err := client.SyncPush(context.Background(), SyncOptions{Source: source})
		if err != nil || report.SkippedSymlinks != 1 || len(report.Added) != 1 {
			t.Fatalf("symlink policy was not followed: %#v %v", report, err)
		}
	}
	if _, err := client.SyncPush(context.Background(), SyncOptions{Source: source, RemotePath: "../outside"}); err == nil {
		t.Fatal("remote traversal path was accepted")
	}
}
