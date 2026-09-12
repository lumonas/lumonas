package collector

import (
	"os"
	"syscall"

	"github.com/lumonas/lumonas/internal/model"
)

var monitoredFilesystemPaths = []string{"/", "/var/lib/lumonas", "/srv/lumonas", "/srv/disks", "/srv/pools"}

// Filesystems reports bounded usage for the paths that hold appliance state,
// logs, recovery data, and NAS payloads. Missing optional mount points are
// omitted; the root filesystem remains the fallback signal.
func Filesystems() []model.FilesystemUsage {
	return FilesystemsAt(monitoredFilesystemPaths)
}

// FilesystemsAt reports bounded usage for an explicit set of paths. Keeping
// the path selection separate makes the real statfs behavior testable against
// disposable filesystems without changing the appliance's monitored roots.
func FilesystemsAt(paths []string) []model.FilesystemUsage {
	result := make([]model.FilesystemUsage, 0, len(paths))
	seen := make(map[string]bool, len(paths))
	for _, path := range paths {
		if seen[path] {
			continue
		}
		value, err := filesystemUsage(path)
		if err != nil {
			continue
		}
		seen[path] = true
		result = append(result, value)
	}
	return result
}

func filesystemUsage(path string) (model.FilesystemUsage, error) {
	if _, err := os.Stat(path); err != nil {
		return model.FilesystemUsage{}, err
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return model.FilesystemUsage{}, err
	}
	blockSize := uint64(stat.Bsize)
	total := stat.Blocks * blockSize
	free := stat.Bfree * blockSize
	available := stat.Bavail * blockSize
	if total == 0 {
		return model.FilesystemUsage{}, os.ErrInvalid
	}
	used := uint64(0)
	if total >= free {
		used = total - free
	}
	return model.FilesystemUsage{
		Path:           path,
		TotalBytes:     total,
		UsedBytes:      used,
		AvailableBytes: available,
		UsedPercent:    float64(used) * 100 / float64(total),
		State:          classifyFilesystemUsage(used, total),
	}, nil
}

func classifyFilesystemUsage(used, total uint64) string {
	if total == 0 || used > total {
		return "unknown"
	}
	percent := float64(used) * 100 / float64(total)
	switch {
	case percent >= 95:
		return "critical"
	case percent >= 80:
		return "warning"
	default:
		return "healthy"
	}
}
