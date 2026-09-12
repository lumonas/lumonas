package main

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
		if bytes.Contains(data, []byte("MYNAS_ADMIN_PASSWORD")) {
			t.Fatalf("support bundle leaked a secret key in %s", file.Name)
		}
	}
}
