package store

import (
	"bytes"
	"testing"
)

func TestPasskeyCredentialCRUD(t *testing.T) {
	store := testStore(t)
	credential := PasskeyCredential{
		ID:        []byte{1, 2, 3, 4},
		PublicKey: []byte{9, 8, 7},
		UserID:    "user-1",
		Name:      "YubiKey",
		SignCount: 5,
	}
	if err := store.SavePasskeyCredential(credential); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.PasskeyCredential(credential.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name != "YubiKey" || loaded.UserID != "user-1" || loaded.SignCount != 5 {
		t.Fatalf("unexpected credential: %#v", loaded)
	}
	if !bytes.Equal(loaded.PublicKey, credential.PublicKey) {
		t.Fatal("public key did not round-trip")
	}

	list, err := store.PasskeyCredentials("user-1")
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %#v err=%v", list, err)
	}
	if _, err := store.PasskeyCredentials("user-2"); err != nil || len(list) == 0 {
		// Other users simply have no credentials.
		other, err := store.PasskeyCredentials("user-2")
		if err != nil || len(other) != 0 {
			t.Fatalf("expected empty list for other user, got %#v err=%v", other, err)
		}
	}

	// Sign count advances monotonically; stale counters are rejected.
	if err := store.UpdatePasskeySignCount(credential.ID, 6); err != nil {
		t.Fatal(err)
	}
	loaded, _ = store.PasskeyCredential(credential.ID)
	if loaded.SignCount != 6 {
		t.Fatalf("sign count = %d, want 6", loaded.SignCount)
	}
	if err := store.UpdatePasskeySignCount(credential.ID, 4); err != nil {
		t.Fatal(err)
	}
	loaded, _ = store.PasskeyCredential(credential.ID)
	if loaded.SignCount != 6 {
		t.Fatalf("stale sign count overwrote value: %d", loaded.SignCount)
	}

	if err := store.DeletePasskeyCredential(credential.ID, "user-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PasskeyCredential(credential.ID); err != ErrPasskeyNotFound {
		t.Fatalf("expected ErrPasskeyNotFound, got %v", err)
	}
	if err := store.DeletePasskeyCredential(credential.ID, "user-1"); err != ErrPasskeyNotFound {
		t.Fatalf("double delete should report not found, got %v", err)
	}
}

func TestPasskeyCredentialRequiresIdentity(t *testing.T) {
	store := testStore(t)
	if err := store.SavePasskeyCredential(PasskeyCredential{UserID: "user-1"}); err == nil {
		t.Fatal("credential without id/public key was accepted")
	}
}

func testStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}
