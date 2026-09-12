package updates

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/testhttp"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b     string
		expected int
	}{
		{"1.2.3", "1.2.3", 0},
		{"1.2.4", "1.2.3", 1},
		{"1.10.0", "1.9.9", 1},
		{"1.2", "1.2.0", 0},
		{"v2.0.0", "1.9.9", 1},
		{"0.9", "1.0", -1},
		{"1.2.3-rc1", "1.2.3", 1},
	}
	for _, testCase := range cases {
		if got := CompareVersions(testCase.a, testCase.b); got != testCase.expected {
			t.Fatalf("CompareVersions(%q, %q) = %d, want %d", testCase.a, testCase.b, got, testCase.expected)
		}
	}
}

func signedDocument(t *testing.T, version string) (FeedDocument, ed25519.PublicKey) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{
		FormatVersion: ManifestFormatVersion,
		Version:       version,
		PackageSHA256: "6a4b1c9e51d2c7a6f0b3e5d8c2a19f7e4b6d3a5c8e1f2b4d7a9c6e3f5b8d2a1c",
		PackageSize:   1024,
		PublishedAt:   time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC),
		Notes:         "Test release",
	}
	canonical, err := CanonicalManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	signature := ed25519.Sign(private, canonical)
	return FeedDocument{Manifest: manifest, Signature: base64.StdEncoding.EncodeToString(signature)}, public
}

func TestFetchAndVerifyFeedDocument(t *testing.T) {
	document, public := signedDocument(t, "1.4.0")
	server := testhttp.NewServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(document)
	}))
	fetched, err := FetchFeed(context.Background(), server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := VerifyFeedDocument(public, fetched)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Version != "1.4.0" {
		t.Fatalf("unexpected manifest version %q", manifest.Version)
	}
}

func TestFetchFeedRejectsHTTPError(t *testing.T) {
	server := testhttp.NewServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	if _, err := FetchFeed(context.Background(), server.Client(), server.URL); err == nil {
		t.Fatal("expected fetch failure for non-200 feed")
	}
}

func TestVerifyFeedDocumentRejectsTampering(t *testing.T) {
	document, public := signedDocument(t, "1.4.0")
	document.Manifest.Version = "9.9.9"
	if _, err := VerifyFeedDocument(public, document); err == nil {
		t.Fatal("expected tampered manifest to fail verification")
	}
	document, public = signedDocument(t, "1.4.0")
	document.Signature = base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
	if _, err := VerifyFeedDocument(public, document); err == nil {
		t.Fatal("expected wrong signature to fail verification")
	}
}

func TestVerifyFeedDocumentRequiresKey(t *testing.T) {
	document, _ := signedDocument(t, "1.4.0")
	if _, err := VerifyFeedDocument(nil, document); err == nil {
		t.Fatal("expected missing key to fail verification")
	}
}
