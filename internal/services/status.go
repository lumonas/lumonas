package services

import (
	"context"
	"os/exec"
	"strings"
)

type Status struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
}

var DefaultNames = []string{"lumonas-privd.service", "lumonasd.service", "lumonas-web.service", "docker.service", "smbd.service", "nfs-server.service", "ssh.service"}

// normalizeState maps raw "systemctl is-active" output onto the state
// vocabulary the web interface understands: running, stopped, degraded.
func normalizeState(raw string) string {
	switch raw {
	case "active", "reloading":
		return "running"
	case "inactive", "deactivating":
		return "stopped"
	default:
		// activating, failed, unknown — and anything unexpected — is
		// reported as degraded so the UI surfaces it for attention.
		return "degraded"
	}
}

func Collect(ctx context.Context, names []string) []Status {
	result := make([]Status, 0, len(names))
	for _, name := range names {
		command := exec.CommandContext(ctx, "systemctl", "is-active", name)
		output, err := command.CombinedOutput()
		raw := strings.TrimSpace(string(output))
		if raw == "" {
			raw = "unknown"
		}
		result = append(result, Status{
			ID:     name,
			Name:   name,
			Active: err == nil && raw == "active",
			State:  normalizeState(raw),
		})
	}
	return result
}
