package main

import (
	"strings"
	"testing"
	"time"
)

func TestApplySnapraidConfigValidatesBeforeAtomicActivation(t *testing.T) {
	path := "/tmp/snapraid.conf"
	var command string
	result := applySnapraidConfig(request{Operation: "snapraid.config.apply", PlanHash: "hash", RequestedState: map[string]any{"configPath": path, "parityDiskId": "serial:parity", "dataDiskIds": []string{"serial:data"}}, ExpiresAt: time.Now().UTC().Add(time.Minute), Confirmed: true}, nil, func(name string, args ...string) ([]byte, error) {
		command = name + " " + strings.Join(args, " ")
		return nil, nil
	})
	if result.OK || !strings.Contains(result.Error, "not allow-listed") {
		t.Fatalf("expected test path rejection: %#v", result)
	}
	if command != "" {
		t.Fatalf("rejected path must not execute SnapRAID: %s", command)
	}
}

func TestApplySnapraidConfigRejectsUnconfirmedAndUnsafeContent(t *testing.T) {
	request := request{Operation: "snapraid.config.apply", PlanHash: "hash", RequestedState: map[string]any{"configPath": "/etc/lumonas/snapraid.conf", "parityDiskId": "serial:parity", "dataDiskIds": []string{}}, ExpiresAt: time.Now().UTC().Add(time.Minute)}
	if result := applySnapraidConfig(request, nil, func(string, ...string) ([]byte, error) { return nil, nil }); result.OK || !strings.Contains(result.Error, "confirmed") {
		t.Fatalf("expected confirmation rejection: %#v", result)
	}
	request.Confirmed = true
	if result := applySnapraidConfig(request, nil, func(string, ...string) ([]byte, error) { return nil, nil }); result.OK || !strings.Contains(result.Error, "at least one") {
		t.Fatalf("expected identity rejection: %#v", result)
	}
}
