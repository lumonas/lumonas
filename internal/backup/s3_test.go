package backup

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestS3UploadUsesAuthenticatedPut(t *testing.T) {
	source := filepath.Join(t.TempDir(), "bundle.mrb")
	if err := os.WriteFile(source, []byte("bundle"), 0o600); err != nil {
		t.Fatal(err)
	}
	request, file, err := signedS3Request(context.Background(), http.MethodPut, "https://s3.example.test/bucket", Credentials{AccessKey: "access", SecretKey: "secret", Region: "us-east-1"}, "recovery/latest.mrb", source)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if request.Method != http.MethodPut || !strings.Contains(request.Header.Get("Authorization"), "AWS4-HMAC-SHA256") {
		t.Fatalf("unexpected S3 request: %s %s", request.Method, request.Header.Get("Authorization"))
	}
	if request.URL.Path != "/bucket/recovery/latest.mrb" {
		t.Fatalf("unexpected S3 object path %q", request.URL.Path)
	}
}
