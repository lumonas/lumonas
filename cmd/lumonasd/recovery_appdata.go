package main

import (
	"context"
	"fmt"
	"os"
	"strconv"

	dockerruntime "github.com/lumonas/lumonas/internal/docker"
	"github.com/lumonas/lumonas/internal/recovery"
)

type dockerAppdataCollection struct {
	Payloads []recovery.AppdataPayload
	Warnings []string
}

func (s *apiServer) collectDockerAppdata(ctx context.Context, stacks []dockerruntime.Stack) dockerAppdataCollection {
	collection := dockerAppdataCollection{Payloads: make([]recovery.AppdataPayload, 0), Warnings: make([]string, 0)}
	limit := configuredAppdataArchiveLimit()
	for _, stack := range stacks {
		if stack.Recovery == nil || len(stack.Recovery.AppdataPaths) == 0 {
			continue
		}
		if stack.Recovery.Strategy == dockerruntime.StrategyNone {
			collection.Warnings = append(collection.Warnings, stack.Name+" uses a recovery strategy that excludes appdata")
			continue
		}
		if stack.Recovery.Strategy != dockerruntime.StrategyStopBackup {
			collection.Warnings = append(collection.Warnings, fmt.Sprintf("%s recovery strategy %q is not executable by the recovery exporter", stack.Name, stack.Recovery.Strategy))
			continue
		}
		sources, err := s.dockerService.AppdataSources(ctx, stack)
		if err != nil {
			collection.Warnings = append(collection.Warnings, fmt.Sprintf("%s appdata source unavailable: %v", stack.Name, err))
			continue
		}
		stopped := false
		if stack.Recovery.Strategy == dockerruntime.StrategyStopBackup {
			if err := s.dockerService.Action(ctx, stack, "stop"); err != nil {
				collection.Warnings = append(collection.Warnings, fmt.Sprintf("%s could not be stopped for a consistent appdata backup: %v", stack.Name, err))
				continue
			}
			stopped = true
		}
		for _, source := range sources {
			archive, archiveErr := recovery.ArchiveAppdata(source.HostPath, limit)
			if archiveErr != nil {
				collection.Warnings = append(collection.Warnings, fmt.Sprintf("%s appdata %s could not be archived: %v", stack.Name, source.ContainerPath, archiveErr))
				continue
			}
			collection.Payloads = append(collection.Payloads, recovery.AppdataPayload{Stack: stack.Name, ContainerPath: source.ContainerPath, HostPath: source.HostPath, Archive: archive})
		}
		if stopped {
			if err := s.dockerService.Action(ctx, stack, "start"); err != nil {
				collection.Warnings = append(collection.Warnings, fmt.Sprintf("%s could not be restarted after appdata backup: %v", stack.Name, err))
			}
		}
	}
	return collection
}

func configuredAppdataArchiveLimit() int64 {
	value := os.Getenv("LUMONAS_APPDATA_ARCHIVE_MAX_BYTES")
	if parsed, err := strconv.ParseInt(value, 10, 64); err == nil && parsed > 0 {
		return parsed
	}
	return recovery.DefaultAppdataArchiveLimit
}
