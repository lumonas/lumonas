package auth

import "testing"

func TestPasswordHashIsSlowAndVerifiable(t *testing.T) {
	hash, err := HashPassword("a-long-development-password")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword("a-long-development-password", hash) {
		t.Fatal("password should verify")
	}
	if VerifyPassword("wrong-password", hash) {
		t.Fatal("wrong password verified")
	}
}

func TestPasswordMinimumLength(t *testing.T) {
	if _, err := HashPassword("short"); err == nil {
		t.Fatal("short password should be rejected")
	}
}
