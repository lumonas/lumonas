package main

import (
	"strings"
	"testing"
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
