package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/lumonas/lumonas/internal/identity"
	"github.com/lumonas/lumonas/internal/shares"
)

func TestShareAccessCheckRejectsSymlinkEscape(t *testing.T) {
	server := testServer(t)
	root := t.TempDir()
	external := t.TempDir()
	if err := os.Symlink(external, filepath.Join(root, "outside")); err != nil {
		t.Fatal(err)
	}
	user, err := server.store.CreatePrincipal(identity.CreateInput{Kind: identity.KindUser, Name: "pathcheck", ManagementRole: identity.RoleNone})
	if err != nil {
		t.Fatal(err)
	}
	share, err := server.store.CreateManagedShare(shares.ManagedShare{ID: "pathcheck-share", Name: "PathCheck", Path: root, Enabled: true, Protocols: []shares.Protocol{{Name: "smb"}}, Access: []shares.AccessRule{{PrincipalID: user.ID, Level: "read"}}})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/shares/"+share.ID+"/access-check?principal="+user.ID+"&path=outside%2Fsecret.txt", nil)
	server.routes().ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected symlink escape to be rejected, got %d: %s", response.Code, response.Body.String())
	}
}

func TestUnixPathAccessUsesOwnerAndParentExecuteBits(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "private")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(directory, "secret.txt")
	if err := os.WriteFile(file, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	stat := info.Sys().(*syscall.Stat_t)
	uid, gid := int64(stat.Uid), int64(stat.Gid)
	principal := identity.Principal{UID: &uid, GID: &gid}
	if access := unixPathAccess(root, file, principal); access != "write" {
		t.Fatalf("expected owner write access, got %q", access)
	}
	if err := os.Chmod(directory, 0o600); err != nil {
		t.Fatal(err)
	}
	if access := unixPathAccess(root, file, principal); access != "none" {
		t.Fatalf("parent without execute permission must block access, got %q", access)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
}
