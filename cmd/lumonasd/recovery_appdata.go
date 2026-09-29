package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	dockerruntime "github.com/lumonas/lumonas/internal/docker"
	"github.com/lumonas/lumonas/internal/recovery"
)

type dockerAppdataCollection struct {
	Payloads      []recovery.AppdataPayload
	DatabaseDumps []recovery.DatabaseDumpPayload
	Warnings      []string
}

type shareDataCollection struct {
	Payloads []recovery.SharePayload
	Warnings []string
}

func (s *apiServer) collectManagedShareData() (shareDataCollection, error) {
	collection := shareDataCollection{Payloads: make([]recovery.SharePayload, 0), Warnings: make([]string, 0)}
	managed, err := s.store.ListManagedShares()
	if err != nil {
		return collection, err
	}
	limit := configuredShareArchiveLimit()
	for _, share := range managed {
		if !share.Enabled {
			continue
		}
		archive, archiveErr := recovery.ArchiveAppdata(share.Path, limit)
		if archiveErr != nil {
			return collection, fmt.Errorf("share %s could not be archived; backup was not published: %w", share.ID, archiveErr)
		}
		collection.Payloads = append(collection.Payloads, recovery.SharePayload{ID: share.ID, Name: share.Name, Path: share.Path, Archive: archive})
	}
	return collection, nil
}

func configuredShareArchiveLimit() int64 {
	value := os.Getenv("LUMONAS_SHARE_ARCHIVE_MAX_BYTES")
	if parsed, err := strconv.ParseInt(value, 10, 64); err == nil && parsed > 0 {
		return parsed
	}
	return recovery.DefaultShareArchiveLimit
}

func (s *apiServer) collectDockerAppdata(ctx context.Context, stacks []dockerruntime.Stack) dockerAppdataCollection {
	collection := dockerAppdataCollection{Payloads: make([]recovery.AppdataPayload, 0), DatabaseDumps: make([]recovery.DatabaseDumpPayload, 0), Warnings: make([]string, 0)}
	limit := configuredAppdataArchiveLimit()
	for _, stack := range stacks {
		if stack.Recovery == nil || (len(stack.Recovery.AppdataPaths) == 0 && stack.Recovery.DBDump == nil && stack.Recovery.PreBackupHook == nil && stack.Recovery.PostBackupHook == nil) {
			continue
		}
		if stack.Recovery.Strategy == dockerruntime.StrategyNone {
			collection.Warnings = append(collection.Warnings, stack.Name+" uses a recovery strategy that excludes app data and database dumps")
			continue
		}
		if stack.Recovery.Strategy != dockerruntime.StrategyStopBackup && stack.Recovery.Strategy != dockerruntime.StrategyCustom {
			collection.Warnings = append(collection.Warnings, fmt.Sprintf("%s recovery strategy %q is not executable by the recovery exporter", stack.Name, stack.Recovery.Strategy))
			continue
		}
		if err := dockerruntime.ValidateRecoveryContract(stack.Recovery); err != nil {
			collection.Warnings = append(collection.Warnings, fmt.Sprintf("%s recovery contract is invalid: %v", stack.Name, err))
			continue
		}
		if stack.Recovery.PreBackupHook != nil {
			if _, err := s.dockerService.RunRecoveryHook(ctx, stack.Name, stack.Recovery.PreBackupHook, 1<<20); err != nil {
				collection.Warnings = append(collection.Warnings, fmt.Sprintf("%s pre-backup hook failed: %v", stack.Name, err))
				continue
			}
		}
		if stack.Recovery.DBDump != nil {
			dump, err := s.dockerService.RunRecoveryHook(ctx, stack.Name, stack.Recovery.DBDump, limit)
			if err != nil {
				collection.Warnings = append(collection.Warnings, fmt.Sprintf("%s database dump failed: %v", stack.Name, err))
			} else if err := validateCollectedDatabaseDump(stack.Name, stack.Recovery.DBDump.Container, dump, limit); err != nil {
				collection.Warnings = append(collection.Warnings, fmt.Sprintf("%s database dump was rejected: %v", stack.Name, err))
			} else {
				collection.DatabaseDumps = append(collection.DatabaseDumps, recovery.DatabaseDumpPayload{Stack: stack.Name, Container: stack.Recovery.DBDump.Container, Dump: dump})
			}
		}
		sources := make([]dockerruntime.AppdataSource, 0)
		if len(stack.Recovery.AppdataPaths) > 0 {
			var err error
			sources, err = s.dockerService.AppdataSources(ctx, stack)
			if err != nil {
				collection.Warnings = append(collection.Warnings, fmt.Sprintf("%s appdata source unavailable: %v", stack.Name, err))
			}
		}
		stopped := false
		if len(sources) > 0 && stack.Recovery.Strategy == dockerruntime.StrategyStopBackup {
			if err := s.dockerService.Action(ctx, stack, "stop"); err != nil {
				collection.Warnings = append(collection.Warnings, fmt.Sprintf("%s could not be stopped for a consistent appdata backup: %v", stack.Name, err))
			} else {
				stopped = true
			}
		}
		if !stopped && len(sources) > 0 && stack.Recovery.Strategy == dockerruntime.StrategyStopBackup {
			collection.Warnings = append(collection.Warnings, fmt.Sprintf("%s appdata archive skipped because the stack could not be stopped", stack.Name))
		} else {
			for _, source := range sources {
				archive, archiveErr := recovery.ArchiveAppdata(source.HostPath, limit)
				if archiveErr != nil {
					collection.Warnings = append(collection.Warnings, fmt.Sprintf("%s appdata %s could not be archived: %v", stack.Name, source.ContainerPath, archiveErr))
					continue
				}
				collection.Payloads = append(collection.Payloads, recovery.AppdataPayload{Stack: stack.Name, ContainerPath: source.ContainerPath, HostPath: source.HostPath, Archive: archive})
			}
		}
		if stopped {
			if err := s.dockerService.Action(ctx, stack, "start"); err != nil {
				collection.Warnings = append(collection.Warnings, fmt.Sprintf("%s could not be restarted after appdata backup: %v", stack.Name, err))
			}
		}
		if stack.Recovery.PostBackupHook != nil {
			if _, err := s.dockerService.RunRecoveryHook(ctx, stack.Name, stack.Recovery.PostBackupHook, 1<<20); err != nil {
				collection.Warnings = append(collection.Warnings, fmt.Sprintf("%s post-backup hook failed: %v", stack.Name, err))
			}
		}
	}
	return collection
}

func validateCollectedDatabaseDump(stack, container string, dump []byte, maxBytes int64) error {
	if strings.TrimSpace(stack) == "" || strings.TrimSpace(container) == "" || len(dump) == 0 {
		return fmt.Errorf("dump output is empty or its workload identity is missing")
	}
	if maxBytes <= 0 {
		maxBytes = recovery.DefaultAppdataArchiveLimit
	}
	if int64(len(dump)) > maxBytes {
		return fmt.Errorf("dump is larger than the configured recovery limit")
	}
	return nil
}

func configuredAppdataArchiveLimit() int64 {
	value := os.Getenv("LUMONAS_APPDATA_ARCHIVE_MAX_BYTES")
	if parsed, err := strconv.ParseInt(value, 10, 64); err == nil && parsed > 0 {
		return parsed
	}
	return recovery.DefaultAppdataArchiveLimit
}
