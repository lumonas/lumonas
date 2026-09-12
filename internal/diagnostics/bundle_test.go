package diagnostics

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestRedactionRemovesSecretCanaries(t *testing.T) {
	value := map[string]any{"password": "canary-password", "nested": map[string]any{"api_token": "canary-token"}, "message": "Bearer canary-bearer"}
	data, err := MarshalJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, canary := range []string{"canary-password", "canary-token", "canary-bearer"} {
		if strings.Contains(text, canary) {
			t.Fatalf("secret canary leaked: %s", canary)
		}
	}
}

func TestBundleRejectsUnsafeNamesAndRedactsText(t *testing.T) {
	if _, err := CreateBundle(map[string][]byte{"../secret.txt": []byte("x")}); err == nil {
		t.Fatal("expected unsafe entry rejection")
	}
	bundle, err := CreateBundle(map[string][]byte{"logs/runtime.log": []byte("password=canary")})
	if err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(bundle), int64(len(bundle)))
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(func() io.Reader { file, _ := reader.File[0].Open(); return file }())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "canary") {
		t.Fatal("text canary leaked into support bundle")
	}
}

func TestRedactionCoversRecoveryAndPrivateKeyMaterial(t *testing.T) {
	privateKey := "-----BEGIN PRIVATE KEY-----\nprivate-canary\n-----END PRIVATE KEY-----"
	data, err := MarshalJSON(map[string]any{
		"recoveryKey": "recovery-canary",
		"privateKey":  privateKey,
		"message":     "private_key=" + privateKey,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "canary") || strings.Contains(string(data), "PRIVATE KEY") {
		t.Fatalf("structured redaction leaked key material: %s", data)
	}

	bundle, err := CreateBundle(map[string][]byte{
		"recovery.key":    []byte("raw-recovery-canary"),
		"tls/private.key": []byte(privateKey),
	})
	if err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(bundle), int64(len(bundle)))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range reader.File {
		handle, openErr := file.Open()
		if openErr != nil {
			t.Fatal(openErr)
		}
		content, readErr := io.ReadAll(handle)
		_ = handle.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if strings.Contains(string(content), "canary") || strings.Contains(string(content), "PRIVATE KEY") {
			t.Fatalf("raw sensitive material leaked from %s: %q", file.Name, content)
		}
	}
}
