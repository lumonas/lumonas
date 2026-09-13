package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/smtp"
	"net/url"
	"strings"
	"time"
)

const MaxProviderResponseBytes = 64 << 10

func SendChannel(ctx context.Context, client *http.Client, channel Channel, credentials Credentials, message Message) error {
	if err := channel.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(message.Title) == "" || strings.TrimSpace(message.Body) == "" {
		return errors.New("notification title and body are required")
	}
	switch channel.Type {
	case "webhook", "slack", "discord", "gotify":
		payload := map[string]string{"title": message.Title, "message": message.Body, "body": message.Body}
		if channel.Type == "slack" {
			payload = map[string]string{"text": message.Title + "\n" + message.Body}
		}
		if channel.Type == "discord" {
			payload = map[string]string{"content": "**" + message.Title + "**\n" + message.Body}
		}
		if channel.Type == "gotify" && credentials.Token != "" {
			channel.Target = strings.TrimRight(channel.Target, "/") + "/message?token=" + url.QueryEscape(credentials.Token)
		}
		return postChannelJSON(ctx, client, channel.Target, payload)
	case "ntfy":
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, channel.Target, strings.NewReader(message.Body))
		if err != nil {
			return err
		}
		request.Header.Set("Title", message.Title)
		request.Header.Set("Priority", ntfyPriority(message.Severity))
		if credentials.Token != "" {
			request.Header.Set("Authorization", "Bearer "+credentials.Token)
		}
		return doChannelRequest(client, request)
	case "telegram":
		if credentials.Token == "" {
			return errors.New("telegram token is required")
		}
		endpoint := "https://api.telegram.org/bot" + url.PathEscape(credentials.Token) + "/sendMessage"
		return postChannelJSON(ctx, client, endpoint, map[string]string{"chat_id": channel.Target, "text": message.Title + "\n" + message.Body})
	case "smtp":
		return sendSMTP(channel, credentials, message)
	default:
		return fmt.Errorf("unsupported notification channel type %q", channel.Type)
	}
}

func SendWithRetry(ctx context.Context, attempts int, send func(context.Context) error) error {
	if attempts < 1 {
		attempts = 1
	}
	var last error
	for attempt := 0; attempt < attempts; attempt++ {
		if err := send(ctx); err == nil {
			return nil
		} else {
			last = err
		}
		if attempt+1 < attempts {
			timer := time.NewTimer(time.Duration(1<<attempt) * 100 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	return last
}

func postChannelJSON(ctx context.Context, client *http.Client, target string, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	return doChannelRequest(client, request)
}

func doChannelRequest(client *http.Client, request *http.Request) error {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, MaxProviderResponseBytes+1))
	if err != nil {
		return fmt.Errorf("read notification provider response: %w", err)
	}
	if len(body) > MaxProviderResponseBytes {
		return fmt.Errorf("notification provider response exceeds %d bytes", MaxProviderResponseBytes)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("notification provider returned HTTP %d", response.StatusCode)
	}
	return nil
}

func sendSMTP(channel Channel, credentials Credentials, message Message) error {
	parsed, err := url.Parse(channel.Target)
	if err != nil || parsed.Host == "" {
		return errors.New("smtp target must include a host")
	}
	to := parsed.Query().Get("to")
	if to == "" {
		return errors.New("smtp target must include a to address")
	}
	from := credentials.Username
	if from == "" {
		return errors.New("smtp username is required as sender")
	}
	auth := smtp.PlainAuth("", credentials.Username, credentials.Password, parsed.Hostname())
	body := "From: " + from + "\r\nTo: " + to + "\r\nSubject: " + message.Title + "\r\n\r\n" + message.Body
	return smtp.SendMail(parsed.Host, auth, from, []string{to}, []byte(body))
}
