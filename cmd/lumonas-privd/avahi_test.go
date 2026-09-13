package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAvahiBrokerRequiresConfirmationAndValidatedContent(t *testing.T) {
	run := func(string, ...string) ([]byte, error) { return nil, nil }
	if result := applyAvahiConfig(request{Operation: "avahi.config.apply", PlanHash: "avahi"}, run); result.OK || !strings.Contains(result.Error, "confirmed") {
		t.Fatalf("unconfirmed Avahi operation was accepted: %#v", result)
	}
	if result := applyAvahiConfig(request{Operation: "avahi.config.apply", PlanHash: "avahi", Confirmed: true, RequestedState: map[string]any{"content": "not XML"}}, run); result.OK || !strings.Contains(result.Error, "invalid") {
		t.Fatalf("invalid Avahi content was accepted: %#v", result)
	}
	if result := applyAvahiConfig(request{Operation: "avahi.config.apply", PlanHash: "avahi", Confirmed: true, RequestedState: map[string]any{"content": strings.Repeat("x", 64*1024+1)}}, run); result.OK || !strings.Contains(result.Error, "invalid") {
		t.Fatalf("oversized Avahi content was accepted: %#v", result)
	}
}

func TestAvahiBrokerPublishesAndRemovesAnnouncement(t *testing.T) {
	directory := t.TempDir()
	previous := avahiServicePath
	avahiServicePath = filepath.Join(directory, "lumonas-smb.service")
	defer func() { avahiServicePath = previous }()
	commands := make([]string, 0)
	run := func(name string, args ...string) ([]byte, error) {
		commands = append(commands, name+" "+strings.Join(args, " "))
		return nil, nil
	}
	base := request{Operation: "avahi.config.apply", OperationID: "avahi-1", PlanHash: "avahi", Confirmed: true, ExpiresAt: time.Now().UTC().Add(time.Minute)}
	content := "<?xml version=\"1.0\"?>\n<!DOCTYPE service-group SYSTEM \"avahi-service.dtd\">\n<service-group>\n  <name>%h</name>\n</service-group>\n"
	published := applyAvahiConfig(request{Operation: base.Operation, OperationID: base.OperationID, PlanHash: base.PlanHash, Confirmed: true, ExpiresAt: base.ExpiresAt, RequestedState: map[string]any{"content": content}}, run)
	if !published.OK {
		t.Fatalf("publish failed: %#v", published)
	}
	data, err := os.ReadFile(avahiServicePath)
	if err != nil || !strings.Contains(string(data), "<service-group>") {
		t.Fatalf("announcement missing: %v %s", err, data)
	}
	if !strings.Contains(strings.Join(commands, "; "), "systemctl reload avahi-daemon") {
		t.Fatalf("expected daemon reload: %v", commands)
	}
	removed := applyAvahiConfig(request{Operation: base.Operation, OperationID: base.OperationID, PlanHash: base.PlanHash, Confirmed: true, ExpiresAt: base.ExpiresAt, RequestedState: map[string]any{"content": ""}}, run)
	if !removed.OK {
		t.Fatalf("remove failed: %#v", removed)
	}
	if _, err := os.Stat(avahiServicePath); !os.IsNotExist(err) {
		t.Fatalf("announcement was not removed: %v", err)
	}
}
