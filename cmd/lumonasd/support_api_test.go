package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lumonas/lumonas/internal/model"
)

func TestSupportBundleReportsUnavailableCollections(t *testing.T) {
	server := testServer(t)
	server.diskFunc = func() ([]model.Disk, error) {
		return nil, errors.New("disk collector unavailable")
	}
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics/support-bundle", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("support bundle failed during partial collection: %d %s", response.Code, response.Body.String())
	}
	reader, err := zip.NewReader(bytes.NewReader(response.Body.Bytes()), int64(response.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range reader.File {
		if file.Name != "server.json" {
			continue
		}
		handle, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		var payload struct {
			CollectionErrors map[string]string `json:"collectionErrors"`
		}
		err = json.NewDecoder(handle).Decode(&payload)
		_ = handle.Close()
		if err != nil {
			t.Fatal(err)
		}
		if payload.CollectionErrors["disks"] != "unavailable" {
			t.Fatalf("support bundle did not record disk collection failure: %#v", payload.CollectionErrors)
		}
		return
	}
	t.Fatal("support bundle did not contain server.json")
}

func TestSupportBundleIsAnAuthenticatedDownloadableArchive(t *testing.T) {
	server := testServer(t)
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics/support-bundle", nil))
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("unexpected support response: %d %q", response.Code, response.Header().Get("Content-Type"))
	}
	reader, err := zip.NewReader(bytes.NewReader(response.Body.Bytes()), int64(response.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if len(reader.File) < 4 {
		t.Fatalf("expected support entries, got %d", len(reader.File))
	}
	for _, file := range reader.File {
		handle, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(handle)
		_ = handle.Close()
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(data, []byte("LUMONAS_ADMIN_PASSWORD")) {
			t.Fatalf("support bundle leaked a secret key in %s", file.Name)
		}
	}
}
