package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Config struct {
	WebhookURL string
	NtfyURL    string
}

type Message struct {
	Title    string `json:"title"`
	Body     string `json:"body"`
	Severity string `json:"severity,omitempty"`
}

type Sender struct {
	Client    *http.Client
	Config    Config
	UserAgent string
}

func (s Sender) Send(ctx context.Context, message Message) error {
	if strings.TrimSpace(message.Title) == "" || strings.TrimSpace(message.Body) == "" {
		return fmt.Errorf("notification title and body are required")
	}
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	var sent bool
	if s.Config.WebhookURL != "" {
		if err := s.postJSON(ctx, client, s.Config.WebhookURL, message); err != nil {
			return fmt.Errorf("webhook: %w", err)
		}
		sent = true
	}
	if s.Config.NtfyURL != "" {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.Config.NtfyURL, strings.NewReader(message.Body))
		if err != nil {
			return fmt.Errorf("ntfy request: %w", err)
		}
		request.Header.Set("Title", message.Title)
		request.Header.Set("Priority", ntfyPriority(message.Severity))
		request.Header.Set("Content-Type", "text/plain; charset=utf-8")
		if err := s.do(client, request); err != nil {
			return fmt.Errorf("ntfy: %w", err)
		}
		sent = true
	}
	if !sent {
		return fmt.Errorf("no notification provider configured")
	}
	return nil
}

func (s Sender) postJSON(ctx context.Context, client *http.Client, endpoint string, message Message) error {
	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	return s.do(client, request)
}

func (s Sender) do(client *http.Client, request *http.Request) error {
	if s.UserAgent != "" {
		request.Header.Set("User-Agent", s.UserAgent)
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("provider returned HTTP %d", response.StatusCode)
	}
	return nil
}

func ntfyPriority(severity string) string {
	switch strings.ToLower(severity) {
	case "critical":
		return "max"
	case "warning":
		return "high"
	case "attention":
		return "default"
	default:
		return "low"
	}
}
