package main

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/privileged"
	"github.com/lumonas/lumonas/internal/storage"
)

// desiredMountEntries derives the declarative mount state from live
// discovery: every mounted LumoNAS disk branch and every mounted mergerfs
// pool becomes a systemd mount unit that must survive reboots.
func (s *apiServer) desiredMountEntries() ([]storage.MountEntry, error) {
	disks, err := s.diskFunc()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pools := storage.DiscoverPools(ctx, disks, nil)
	entries := make([]storage.MountEntry, 0, len(pools)+len(disks))
	for _, pool := range pools {
		branches := make([]string, 0, len(pool.Members))
		for _, member := range pool.Members {
			if member.BranchPath != "" {
				branches = append(branches, member.BranchPath)
			}
		}
		if len(branches) == 0 {
			continue
		}
		entries = append(entries, storage.MountEntry{Kind: "pool", TargetID: pool.Name, MountPath: pool.MountPath, FSType: "fuse.mergerfs", Source: strings.Join(branches, ":"), Options: storage.PoolMountOptions, Enabled: true})
	}
	for _, disk := range disks {
		if !disk.Mounted || disk.Filesystem != "ext4" && disk.Filesystem != "xfs" || disk.FilesystemUUID == "" {
			continue
		}
		entries = append(entries, storage.MountEntry{Kind: "disk", TargetID: disk.ID, MountPath: storage.DiskBranchPath(disk.ID), FSType: disk.Filesystem, Source: "UUID=" + disk.FilesystemUUID, Options: storage.DiskMountOptions, Enabled: true})
	}
	return entries, nil
}

// persistMountState reconciles declarative mount persistence: it snapshots
// the live mount state into the store and asks the privileged storage worker
// to regenerate systemd mount units when the desired state changed.
// Persistence is best-effort; failures never fail the triggering operation.
func (s *apiServer) persistMountState(reason string) {
	entries, err := s.desiredMountEntries()
	if err != nil {
		s.warnMountPersistence(reason, err)
		return
	}
	if len(entries) == 0 {
		if existing, listErr := s.store.MountEntries(); listErr == nil && len(existing) == 0 {
			return
		}
	}
	changed, err := s.store.MountEntriesChanged(entries)
	if err != nil {
		changed = true
	}
	if !changed {
		return
	}
	if err := s.store.SaveMountEntries(entries); err != nil {
		s.warnMountPersistence(reason, err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.applyMountPersistence(ctx, entries); err != nil {
		s.warnMountPersistence(reason, err)
		return
	}
	s.publish("storage.mounts.persisted", "info", nil, map[string]any{"reason": reason, "entries": len(entries)})
}

func (s *apiServer) warnMountPersistence(reason string, err error) {
	if s.log != nil {
		s.log.Warn("mount persistence could not be updated", "reason", reason, "error", err)
	}
}

func (s *apiServer) applyMountPersistence(ctx context.Context, entries []storage.MountEntry) error {
	disks, err := s.diskFunc()
	if err != nil {
		return fmt.Errorf("disk identity discovery unavailable: %w", err)
	}
	expected, err := expectedMountDisks(disks, entries)
	if err != nil {
		return err
	}
	encoded := make([]any, 0, len(entries))
	for _, entry := range entries {
		encoded = append(encoded, map[string]any{"kind": entry.Kind, "targetId": entry.TargetID, "mountPath": entry.MountPath, "fstype": entry.FSType, "source": entry.Source, "options": entry.Options, "enabled": entry.Enabled})
	}
	request := privileged.Request{Operation: "storage.mountpersist.apply", OperationID: newID("mountpersist"), PlanHash: "mountpersist-" + fmt.Sprint(s.currentGeneration()), ExpectedDisks: expected, RequestedState: map[string]any{"entries": encoded}, ExpiresAt: time.Now().UTC().Add(5 * time.Minute), Confirmed: true}
	result, err := (privileged.Client{Socket: envOr("LUMONAS_PRIVD_SOCKET", "/run/lumonas/privd.sock")}).Execute(ctx, request)
	if err != nil {
		return err
	}
	if !result.OK {
		return fmt.Errorf("%s", result.Error)
	}
	return nil
}

func expectedMountDisks(disks []model.Disk, entries []storage.MountEntry) ([]privileged.ExpectedDisk, error) {
	byID := make(map[string]model.Disk, len(disks))
	ids := make(map[string]bool)
	for _, disk := range disks {
		byID[disk.ID] = disk
	}
	for _, entry := range entries {
		if entry.Kind == "disk" {
			ids[entry.TargetID] = true
			continue
		}
		for _, branch := range strings.Split(entry.Source, ":") {
			found := false
			for _, disk := range disks {
				if storage.DiskBranchPath(disk.ID) == branch {
					ids[disk.ID] = true
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("mount branch %q has no discovered stable disk identity", branch)
			}
		}
	}
	result := make([]privileged.ExpectedDisk, 0, len(ids))
	for id := range ids {
		disk, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("mount disk identity is no longer present: %s", id)
		}
		result = append(result, privileged.ExpectedDisk{ID: disk.ID, WWN: disk.WWN, Serial: disk.Serial, Model: disk.Model, SizeBytes: disk.SizeBytes, FilesystemUUID: disk.FilesystemUUID})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func (s *apiServer) storageMounts(w http.ResponseWriter) {
	entries, err := s.store.MountEntries()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}
