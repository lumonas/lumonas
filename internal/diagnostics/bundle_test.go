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
