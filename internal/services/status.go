package services

import (
	"context"
	"os/exec"
	"strings"
)

type Status struct {
	Name   string `json:"name"`
	Active bool   `json:"active"`
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
}

var DefaultNames = []string{"mynas-privd.service", "mynasd.service", "mynas-web.service", "docker.service", "smbd.service", "nfs-server.service", "ssh.service"}

func Collect(ctx context.Context, names []string) []Status {
	result := make([]Status, 0, len(names))
	for _, name := range names {
		command := exec.CommandContext(ctx, "systemctl", "is-active", name)
		output, err := command.CombinedOutput()
		state := strings.TrimSpace(string(output))
		if state == "" {
			state = "unknown"
		}
		result = append(result, Status{Name: name, Active: err == nil && state == "active", State: state})
	}
	return result
}
