package collector

import (
	"encoding/json"
	"os/exec"
	"strings"

	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/runner"
)

type dockerContainer struct {
	ID    string `json:"ID"`
	Names string `json:"Names"`
	Image string `json:"Image"`
	State string `json:"State"`
	Ports string `json:"Ports"`
}

func DockerSummary() model.DockerSummary {
	if _, err := exec.LookPath("docker"); err != nil {
		return model.DockerSummary{}
	}
	out, err := runner.Output("docker", "ps", "--format", "{{json .}}", "-a")
	if err != nil {
		return model.DockerSummary{}
	}
	count, running := 0, 0
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		var c dockerContainer
		if json.Unmarshal([]byte(line), &c) == nil {
			count++
			if strings.HasPrefix(c.State, "Up") {
				running++
			}
		}
	}
	return model.DockerSummary{Stacks: 0, AppsRunning: running, UpdatesAvailable: 0}
}
