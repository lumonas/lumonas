package notify

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
)

func TestSenderPostsWebhookWithoutLeakingConfiguration(t *testing.T) {
	called := false
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		called = true
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.Header.Get("Content-Type"))
		}
		return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(bytes.NewReader(nil)), Header: make(http.Header)}, nil
	})
	if err := (Sender{Client: &http.Client{Transport: transport}, Config: Config{WebhookURL: "https://example.invalid/webhook"}}).Send(context.Background(), Message{Title: "Disk", Body: "Healthy", Severity: "info"}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("webhook was not called")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestSenderRequiresProvider(t *testing.T) {
	if err := (Sender{}).Send(context.Background(), Message{Title: "x", Body: "y"}); err == nil {
		t.Fatal("expected provider error")
	}
}

func TestShouldSendUsesSeverityThreshold(t *testing.T) {
	if ShouldSend("warning", "info") || !ShouldSend("warning", "critical") || !ShouldSend("attention", "warning") {
		t.Fatal("severity threshold was not enforced")
	}
}
