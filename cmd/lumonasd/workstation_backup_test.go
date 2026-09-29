package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/lumonas/lumonas/internal/shares"
	"github.com/lumonas/lumonas/internal/workstationbackup"
)

type apiInProcessTransport struct{ handler http.Handler }

func (transport apiInProcessTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	local := httptest.NewRequest(request.Method, request.URL.String(), request.Body)
	local.Header = request.Header.Clone()
	local.ContentLength = request.ContentLength
	response := httptest.NewRecorder()
	transport.handler.ServeHTTP(response, local)
	result := response.Result()
	result.Request = request
	return result, nil
}

func TestWorkstationClientAgainstLiveDaemonRoutes(t *testing.T) {
	server := testServer(t)
	shareRoot := filepath.Join(t.TempDir(), "private-backups")
	if err := os.Mkdir(shareRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := server.store.CreateManagedShare(shares.ManagedShare{ID: "share-workstation", Name: "Workstation archives", Path: shareRoot, Enabled: true, Protocols: []shares.Protocol{{Name: "smb"}}}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LUMONAS_UPLOAD_SESSION_DIR", filepath.Join(t.TempDir(), "upload-sessions"))
	source := filepath.Join(t.TempDir(), "source")
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatal(err)
	}
	contents := bytes.Repeat([]byte("real daemon route payload\n"), 512)
	if err := os.WriteFile(filepath.Join(source, "notes.txt"), contents, 0o640); err != nil {
		t.Fatal(err)
	}
	key := bytes.Repeat([]byte{0x9d}, 32)
	client, err := workstationbackup.NewClient(workstationbackup.Config{
		ServerURL: "https://lumonas.test",
		Token:     "workstation-test-token",
		ShareID:   "share-workstation",
		Key:       key,
		StateDir:  t.TempDir(),
		HTTP:      &http.Client{Transport: apiInProcessTransport{handler: server.routes()}},
	})
	if err != nil {
		t.Fatal(err)
	}
	filename, err := client.Backup(context.Background(), source, "/")
	if err != nil {
		t.Fatal(err)
	}
	stored, err := os.ReadFile(filepath.Join(shareRoot, filename))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(stored, []byte("real daemon route payload")) {
		t.Fatal("NAS stored workstation data without client encryption")
	}
	archives, err := client.ListArchives(context.Background(), "/")
	if err != nil || len(archives) != 1 || archives[0] != filename {
		t.Fatalf("archive listing failed: %#v err=%v", archives, err)
	}
	output := filepath.Join(t.TempDir(), "restored")
	if err := client.Restore(context.Background(), filename, "/", output, false); err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(filepath.Join(output, "notes.txt"))
	if err != nil || !bytes.Equal(actual, contents) {
		t.Fatalf("live daemon restore differed from the source: err=%v", err)
	}
}
