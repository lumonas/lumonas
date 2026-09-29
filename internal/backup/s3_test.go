package backup

import (
	"context"
	"io"
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

func TestS3ObjectLockHeadersAreSigned(t *testing.T) {
	source := filepath.Join(t.TempDir(), "bundle.mrb")
	if err := os.WriteFile(source, []byte("bundle"), 0o600); err != nil {
		t.Fatal(err)
	}
	headers := make(http.Header)
	headers.Set("x-amz-object-lock-mode", "COMPLIANCE")
	headers.Set("x-amz-object-lock-retain-until-date", "2027-01-01T00:00:00Z")
	request, file, err := signedS3RequestWithHeaders(context.Background(), http.MethodPut, "https://s3.example.test/bucket", Credentials{AccessKey: "access", SecretKey: "secret"}, "recovery/latest.mrb", source, headers)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if request.Header.Get("x-amz-object-lock-mode") != "COMPLIANCE" || !strings.Contains(request.Header.Get("Authorization"), "x-amz-object-lock-mode;x-amz-object-lock-retain-until-date") {
		t.Fatalf("Object Lock headers were not included in the request signature: %s", request.Header.Get("Authorization"))
	}
}

func TestS3ObjectLockCapabilityAndRetentionAreReadBack(t *testing.T) {
	original := s3ObjectLockHTTPClient
	t.Cleanup(func() { s3ObjectLockHTTPClient = original })
	requests := 0
	s3ObjectLockHTTPClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		if !strings.Contains(request.Header.Get("Authorization"), "AWS4-HMAC-SHA256") {
			t.Error("S3 object-lock request was not signed")
		}
		if requests == 1 {
			if request.URL.Path != "/bucket" || !request.URL.Query().Has("object-lock") {
				t.Errorf("unexpected bucket lock query: %s", request.URL)
			}
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`<ObjectLockConfiguration><ObjectLockEnabled>Enabled</ObjectLockEnabled></ObjectLockConfiguration>`)), Request: request}, nil
		}
		if request.URL.Path != "/bucket/configured/recovery/latest.mrb" || !request.URL.Query().Has("retention") {
			t.Errorf("unexpected object retention query: %s", request.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`<GetObjectRetentionOutput><Retention><Mode>COMPLIANCE</Mode><RetainUntilDate>2030-01-01T00:00:00Z</RetainUntilDate></Retention></GetObjectRetentionOutput>`)), Request: request}, nil
	})}
	enabled, err := s3BucketObjectLockEnabled(context.Background(), "https://s3.example.test/bucket/configured", Credentials{AccessKey: "access", SecretKey: "secret"})
	if err != nil || !enabled {
		t.Fatalf("expected bucket Object Lock enabled, enabled=%v err=%v", enabled, err)
	}
	verified, err := s3ObjectRetentionPresent(context.Background(), "https://s3.example.test/bucket/configured", Credentials{AccessKey: "access", SecretKey: "secret"}, "recovery/latest.mrb")
	if err != nil || !verified {
		t.Fatalf("expected per-object retention to verify, verified=%v err=%v", verified, err)
	}
}

func TestS3ListUsesPrefixAndContinuation(t *testing.T) {
	page := 0
	original := s3ListHTTPClient
	t.Cleanup(func() { s3ListHTTPClient = original })
	s3ListHTTPClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if !strings.Contains(request.Header.Get("Authorization"), "AWS4-HMAC-SHA256") {
			t.Errorf("missing S3 signature")
		}
		if request.URL.Path != "/bucket" || request.URL.Query().Get("prefix") != "configured/task/" {
			t.Errorf("unexpected listing URL: %s", request.URL)
		}
		page++
		body := `<ListBucketResult><IsTruncated>false</IsTruncated><Contents><Key>configured/task/sub/b.txt</Key><Size>4</Size><LastModified>2026-09-28T10:01:00Z</LastModified></Contents><Contents><Key>configured/task-neighbor/no.txt</Key><Size>9</Size></Contents></ListBucketResult>`
		if page == 1 {
			if request.URL.Query().Get("continuation-token") != "" {
				t.Errorf("unexpected first continuation token")
			}
			body = `<ListBucketResult><IsTruncated>true</IsTruncated><NextContinuationToken>next page</NextContinuationToken><Contents><Key>configured/task/a.txt</Key><Size>3</Size><LastModified>2026-09-28T10:00:00Z</LastModified></Contents></ListBucketResult>`
		} else if request.URL.Query().Get("continuation-token") != "next page" {
			t.Errorf("continuation token not preserved: %s", request.URL.RawQuery)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})}
	files, err := listS3(context.Background(), "https://s3.example.test/bucket/configured", Credentials{AccessKey: "access", SecretKey: "secret", Region: "us-east-1"}, "task")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0].Path != "a.txt" || files[1].Path != "sub/b.txt" {
		t.Fatalf("unexpected S3 listing: %#v", files)
	}
}

func TestS3ListReturnsRemoteHTTPFailures(t *testing.T) {
	original := s3ListHTTPClient
	t.Cleanup(func() { s3ListHTTPClient = original })
	s3ListHTTPClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusForbidden, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("denied")), Request: request}, nil
	})}
	_, err := listS3(context.Background(), "https://s3.example.test/bucket", Credentials{AccessKey: "access", SecretKey: "secret"}, "")
	if err == nil || !strings.Contains(err.Error(), "HTTP 403") {
		t.Fatalf("expected S3 HTTP error, got %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}
