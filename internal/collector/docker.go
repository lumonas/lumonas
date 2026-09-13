package collector

import (
	"context"

	dockerruntime "github.com/lumonas/lumonas/internal/docker"
	"github.com/lumonas/lumonas/internal/model"
)

func DockerSummary() model.DockerSummary {
	return DockerSummaryFromService(context.Background(), dockerruntime.New("", nil))
}

// DockerSummaryFromService keeps legacy collectors on the same typed Engine
// API path as the daemon. Compose stack discovery remains owned by the Docker
// runtime service, so this summary intentionally reports only container
// activity and availability.
func DockerSummaryFromService(ctx context.Context, service dockerruntime.Service) model.DockerSummary {
	if !service.Available(ctx) {
		return model.DockerSummary{}
	}
	containers, err := service.Containers(ctx)
	if err != nil {
		return model.DockerSummary{}
	}
	running := 0
	for _, container := range containers {
		if container.State == "running" {
			running++
		}
	}
	return model.DockerSummary{Available: true, AppsRunning: running}
}
