package storage

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/lumonas/lumonas/internal/model"
)

type RuntimeRunner func(context.Context, string, ...string) ([]byte, error)

type mount struct {
	Target string
	FSType string
	Source string
}

func DiscoverPools(ctx context.Context, disks []model.Disk, runner RuntimeRunner) []model.Pool {
	if runner == nil {
		runner = func(ctx context.Context, command string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, command, args...).Output()
		}
	}
	output, err := runner(ctx, "findmnt", "-rn", "-o", "TARGET,FSTYPE,SOURCE", "-t", "fuse.mergerfs,mergerfs")
	if err != nil {
		return []model.Pool{}
	}
	mounts := parseMounts(string(output))
	result := make([]model.Pool, 0, len(mounts))
	for index, item := range mounts {
		if item.Target == "" || !strings.Contains(item.FSType, "mergerfs") {
			continue
		}
		stat, _ := filesystemUsage(item.Target)
		pool := model.Pool{ID: "pool-" + strconv.Itoa(index+1), Name: filepath.Base(item.Target), Type: "mergerfs", MountPath: item.Target, Status: model.Healthy, SizeBytes: stat.size, UsedBytes: stat.used}
		for _, branch := range strings.Split(item.Source, ":") {
			branch = strings.TrimSpace(branch)
			if branch == "" {
				continue
			}
			member := model.PoolMember{Enabled: true}
			for _, disk := range disks {
				if pathMatchesDisk(branch, disk) {
					member.DiskID = disk.ID
					break
				}
			}
			if member.DiskID != "" {
				pool.Members = append(pool.Members, member)
			}
		}
		result = append(result, pool)
	}
	return result
}

type usage struct{ size, used uint64 }

func filesystemUsage(path string) (usage, bool) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return usage{}, false
	}
	size := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bavail * uint64(stat.Bsize)
	return usage{size: size, used: size - free}, true
}

func pathMatchesDisk(branch string, disk model.Disk) bool {
	for _, value := range []string{disk.ID, disk.CurrentPath, disk.Name, disk.Serial, disk.WWN} {
		if value != "" && strings.Contains(branch, value) {
			return true
		}
	}
	return false
}

func parseMounts(output string) []mount {
	result := make([]mount, 0)
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		result = append(result, mount{Target: fields[0], FSType: fields[1], Source: fields[2]})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Target < result[j].Target })
	return result
}

func DiscoverProtection(ctx context.Context, disks []model.Disk, runner RuntimeRunner, configPath string) model.Protection {
	result := model.Protection{Status: model.Attention, SyncSchedule: "Not configured", ScrubSchedule: "Not configured", LastSyncResult: nil}
	if configPath == "" {
		configPath = "/etc/mynas/snapraid.conf"
	}
	config, err := osReadFile(configPath)
	if err != nil {
		return result
	}
	missingConfiguredDisk := false
	for _, line := range strings.Split(string(config), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		switch fields[0] {
		case "parity":
			if disk := diskForPath(fields[1], disks); disk != nil {
				result.ParityDisks = append(result.ParityDisks, model.DiskRef{DiskID: disk.ID, SizeBytes: disk.SizeBytes})
			} else {
				missingConfiguredDisk = true
			}
		case "data":
			dataPath := fields[1]
			if len(fields) >= 3 {
				dataPath = fields[2]
			}
			if disk := diskForPath(dataPath, disks); disk != nil {
				result.ProtectedDiskIDs = append(result.ProtectedDiskIDs, disk.ID)
			} else {
				missingConfiguredDisk = true
			}
		}
	}
	if missingConfiguredDisk {
		result.Status = model.Critical
	}
	if runner == nil {
		runner = func(ctx context.Context, command string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, command, args...).Output()
		}
	}
	// snapraid status is read-only. A successful invocation proves the configured
	// set is readable, but does not prove parity is current, so status remains
	// attention until a recorded sync result exists.
	_, _ = runner(ctx, "snapraid", "-c", configPath, "status")
	return result
}

var osReadFile = func(path string) ([]byte, error) { return os.ReadFile(path) }

func diskForPath(path string, disks []model.Disk) *model.Disk {
	for index := range disks {
		if pathMatchesDisk(path, disks[index]) {
			return &disks[index]
		}
	}
	return nil
}
