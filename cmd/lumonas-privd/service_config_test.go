package main

import (
	"strings"
	"testing"
)

func TestManagedServiceConfigTargetAllowListsNativeDestinations(t *testing.T) {
	tests := []struct {
		service string
		source  string
		target  string
	}{
		{"nfs-server.service", "/var/lib/lumonas/generated/exports", "/etc/exports.d/lumonas.exports"},
		{"ssh.service", "/var/lib/lumonas/generated/sshd-sftp.conf", "/etc/ssh/sshd_config.d/90-lumonas-sftp.conf"},
	}
	for _, test := range tests {
		got, err := managedServiceConfigTarget(test.service, test.source)
		if err != nil || got != test.target {
			t.Fatalf("managedServiceConfigTarget(%q, %q) = %q, %v", test.service, test.source, got, err)
		}
	}
	for _, test := range [][2]string{
		{"nfs-server.service", "/etc/passwd"},
		{"ssh.service", "/var/lib/lumonas/generated/exports"},
		{"vsftpd.service", "/var/lib/lumonas/generated/ftp.conf"},
	} {
		if _, err := managedServiceConfigTarget(test[0], test[1]); err == nil {
			t.Fatalf("expected %s/%s to be rejected", test[0], test[1])
		}
	}
}

func TestApplyServiceConfigInstallsAndRestartsAllowListedService(t *testing.T) {
	var commands []string
	run := func(name string, args ...string) ([]byte, error) {
		commands = append(commands, strings.Join(append([]string{name}, args...), " "))
		return nil, nil
	}
	result := applyServiceConfig(request{
		Operation: "service.config.apply",
		Confirmed: true,
		RequestedState: map[string]any{
			"service":    "nfs-server.service",
			"sourcePath": "/var/lib/lumonas/generated/exports",
		},
	}, run)
	if !result.OK {
		t.Fatalf("service config apply failed: %#v", result)
	}
	joined := strings.Join(commands, "; ")
	if !strings.Contains(joined, "install -D -m 0640 /var/lib/lumonas/generated/exports /etc/exports.d/lumonas.exports") || !strings.Contains(joined, "systemctl reload-or-restart nfs-server.service") {
		t.Fatalf("unexpected activation commands: %s", joined)
	}
}

func TestServiceReloadUsesReloadOrRestart(t *testing.T) {
	var command string
	result := execute(request{
		Operation:      "service.reload",
		PlanHash:       "service-reload",
		Confirmed:      true,
		RequestedState: map[string]any{"service": "smbd.service"},
	}, nil, func(name string, args ...string) ([]byte, error) {
		command = strings.Join(append([]string{name}, args...), " ")
		return nil, nil
	})
	if !result.OK || command != "systemctl reload-or-restart smbd.service" {
		t.Fatalf("unexpected service reload result: %#v command=%q", result, command)
	}
}
