package notify

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSendWithRetryStopsAfterSuccess(t *testing.T) {
	attempts := 0
	err := SendWithRetry(context.Background(), 3, func(context.Context) error {
		attempts++
		if attempts < 2 {
			return errors.New("temporary")
		}
		return nil
	})
	if err != nil || attempts != 2 {
		t.Fatalf("retry failed: attempts=%d err=%v", attempts, err)
	}
}

func TestSendWithRetryHonorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	err := SendWithRetry(ctx, 3, func(context.Context) error { return errors.New("temporary") })
	if err == nil {
		t.Fatal("cancelled retry returned success")
	}
}

func TestSendChannelRejectsOversizedProviderResponse(t *testing.T) {
	client := &http.Client{Transport: providerRoundTripper(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(strings.Repeat("x", MaxProviderResponseBytes+1))), Request: request}, nil
	})}
	err := SendChannel(context.Background(), client, Channel{ID: "provider-test", Label: "Provider test", Type: "webhook", Target: "https://provider.test/webhook"}, Credentials{}, Message{Title: "test", Body: "payload"})
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected oversized provider response rejection, got %v", err)
	}
}

func TestSendChannelSMTPRequiresSenderCredentials(t *testing.T) {
	err := SendChannel(context.Background(), nil, Channel{
		ID: "smtp-test", Label: "SMTP", Type: "smtp", Target: "smtp://mail.example:587?to=alerts@example.com",
	}, Credentials{Password: "secret"}, Message{Title: "test", Body: "payload"})
	if err == nil || !strings.Contains(err.Error(), "smtp username is required") {
		t.Fatalf("expected SMTP sender validation, got %v", err)
	}
}

type providerRoundTripper func(*http.Request) (*http.Response, error)

func (roundTrip providerRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}
