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

func TestShareStorageResourcesOnlyReturnManagedMountRoots(t *testing.T) {
	server := testServer(t)
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/shares/storage-resources", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("storage resource status %d: %s", response.Code, response.Body.String())
	}
	var resources []shareStorageResource
	if err := json.Unmarshal(response.Body.Bytes(), &resources); err != nil {
		t.Fatal(err)
	}
	for _, resource := range resources {
		if resource.ID != resource.Path || (!strings.HasPrefix(resource.Path, "/srv/pools/") && !strings.HasPrefix(resource.Path, "/srv/disks/")) {
			t.Fatalf("unsafe share storage resource returned: %#v", resource)
		}
	}
}

func TestEnsureShareDirectoryIsConfinedToMountedRoot(t *testing.T) {
	root := t.TempDir()
	created, err := ensureShareDirectory(root, "family/photos")
	if err != nil {
		t.Fatal(err)
	}
	if created != filepath.Join(root, "family", "photos") {
		t.Fatalf("unexpected created path %q", created)
	}
	if _, err := os.Stat(created); err != nil {
		t.Fatalf("share directory was not created: %v", err)
	}
	created, err = ensureShareDirectory(root, "family/photos")
	if err != nil || created != "" {
		t.Fatalf("existing share directory should be reused, got path=%q err=%v", created, err)
	}
	if _, err := ensureShareDirectory(root, "../escape"); err == nil {
		t.Fatal("path traversal was accepted")
	}

	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape-link")); err != nil {
		t.Fatal(err)
	}
	if _, err := ensureShareDirectory(root, "escape-link/created"); err == nil {
		t.Fatal("symlink escape was accepted")
	}
	if _, err := os.Stat(filepath.Join(outside, "created")); !os.IsNotExist(err) {
		t.Fatalf("rejected path created data outside storage root: %v", err)
	}
}
