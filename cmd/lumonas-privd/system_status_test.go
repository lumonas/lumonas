package main

import (
	"errors"
	"strings"
	"testing"
)

func TestDebianUpdatesStatusCountsPendingAndSecurity(t *testing.T) {
	run := fakeCommand{
		"dist-upgrade": strings.Join([]string{
			"Reading package lists...",
			"Calculating upgrade...",
			"Inst libc6 [2.36] (2.37 Debian:13.0/stable [amd64])",
			"Inst openssh-server [9.2] (9.4 Debian:13.0/security [amd64])",
			"Inst linux-image-amd64 [6.1] (6.2 Debian-Security:13/security [amd64])",
			"Conf libc6 (2.37 Debian:13.0/stable [amd64])",
		}, "\n"),
	}
	response := debianUpdatesStatus(run.run)
	if !response.OK {
		t.Fatalf("updates.debian.status failed: %s", response.Error)
	}
	data := response.Data.(map[string]any)
	if data["pendingCount"].(int) != 3 {
		t.Fatalf("pendingCount = %v, want 3", data["pendingCount"])
	}
	if data["securityCount"].(int) != 2 {
		t.Fatalf("securityCount = %v, want 2", data["securityCount"])
	}
}

func TestDebianUpdatesStatusFailsWithoutApt(t *testing.T) {
	failing := func(name string, args ...string) ([]byte, error) {
		return nil, errors.New("apt-get: not found")
	}
	if response := debianUpdatesStatus(failing); response.OK {
		t.Fatal("apt failure should not report success")
	}
}

func TestDebianUpdatesStatusEmptyCacheMeansZero(t *testing.T) {
	response := debianUpdatesStatus(fakeCommand{"dist-upgrade": "Reading package lists...\nCalculating upgrade...\n"}.run)
	if !response.OK {
		t.Fatalf("empty upgrade set should succeed: %s", response.Error)
	}
	data := response.Data.(map[string]any)
	if data["pendingCount"].(int) != 0 || data["securityCount"].(int) != 0 {
		t.Fatalf("expected zero pending, got %#v", data)
	}
}

func TestDockerLogUsageRanksContainers(t *testing.T) {
	run := fakeCommand{
		"du -sb": strings.Join([]string{
			"104857600 /var/lib/docker/containers/ccc/ccc-json.log",
			"2097152 /var/lib/docker/containers/aaa/aaa-json.log",
			"52428800 /var/lib/docker/containers/bbb/bbb-json.log",
			"garbage line without size",
		}, "\n"),
	}
	response := dockerLogUsage(run.run)
	if !response.OK {
		t.Fatalf("docker.logs.usage failed: %s", response.Error)
	}
	data := response.Data.(map[string]any)
	consumers := data["topConsumers"].([]logConsumer)
	if len(consumers) != 3 {
		t.Fatalf("expected 3 consumers, got %d", len(consumers))
	}
	if consumers[0].Name != "ccc" || consumers[0].SizeBytes != 104857600 {
		t.Fatalf("largest consumer wrong: %#v", consumers[0])
	}
	if consumers[2].Name != "aaa" {
		t.Fatalf("ranking wrong: %#v", consumers)
	}
	if data["totalBytes"].(int64) != 104857600+52428800+2097152 {
		t.Fatalf("totalBytes wrong: %v", data["totalBytes"])
	}
}

func TestDockerLogUsageEmptyScanReturnsEmptyList(t *testing.T) {
	run := fakeCommand{"du -sb": ""}
	response := dockerLogUsage(run.run)
	if !response.OK {
		t.Fatalf("empty scan should still succeed: %s", response.Error)
	}
	consumers := response.Data.(map[string]any)["topConsumers"].([]logConsumer)
	if len(consumers) != 0 {
		t.Fatalf("expected empty consumers, got %#v", consumers)
	}
}
