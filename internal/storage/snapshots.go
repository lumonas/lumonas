package storage

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// SnapshotKind identifies the filesystem that owns a snapshot.
type SnapshotKind string

const (
	SnapshotBtrfs SnapshotKind = "btrfs"
	SnapshotZfs   SnapshotKind = "zfs"
)

// Snapshot is one read-only point-in-time capture of a subvolume or dataset.
type Snapshot struct {
	ID        string       `json:"id"`
	Kind      SnapshotKind `json:"kind"`
	Source    string       `json:"source"`
	Name      string       `json:"name"`
	CreatedAt time.Time    `json:"createdAt"`
	UsedBytes uint64       `json:"usedBytes,omitempty"`
	Readonly  bool         `json:"readonly"`
}

// SnapshotCreateRequest captures one snapshot creation.
type SnapshotCreateRequest struct {
	Kind   SnapshotKind `json:"kind"`
	Source string       `json:"source"`
	Label  string       `json:"label,omitempty"`
}

// SnapshotRunner executes a command and returns its combined output.
type SnapshotRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

var snapshotLabelPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)
var snapshotNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,120}$`)

// SnapshotTimestamp renders the timestamp component of a snapshot name.
func SnapshotTimestamp(now time.Time) string {
	return now.UTC().Format("20060102T150405Z")
}

// SnapshotName combines the optional label with the timestamp.
func SnapshotName(label string, now time.Time) string {
	stamp := SnapshotTimestamp(now)
	if label == "" {
		return stamp
	}
	return label + "-" + stamp
}

// ValidateSnapshotSource checks a subvolume path (btrfs) or dataset (zfs).
func ValidateSnapshotSource(kind SnapshotKind, source string) error {
	switch kind {
	case SnapshotBtrfs:
		if source == "" || source == "/" || !filepath.IsAbs(source) || filepath.Clean(source) != source {
			return errors.New("snapshot source must be a clean absolute subvolume path")
		}
	case SnapshotZfs:
		if !validZfsDataset(source) {
			return errors.New("invalid zfs dataset name")
		}
	default:
		return fmt.Errorf("unsupported snapshot kind %q", kind)
	}
	return nil
}

func validZfsDataset(name string) bool {
	if name == "" || len(name) > 256 || strings.HasPrefix(name, "-") {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
		if !snapshotNamePattern.MatchString(part) {
			return false
		}
	}
	return true
}

// ValidateSnapshotLabel accepts an optional human label used as name prefix.
func ValidateSnapshotLabel(label string) error {
	if label == "" {
		return nil
	}
	if !snapshotLabelPattern.MatchString(label) {
		return errors.New("snapshot label must contain letters, numbers, dots, hyphens, or underscores")
	}
	return nil
}

// DetectBtrfs reports whether path is a btrfs subvolume.
func DetectBtrfs(ctx context.Context, run SnapshotRunner, path string) (bool, error) {
	out, err := run(ctx, "btrfs", "subvolume", "show", path)
	if err != nil {
		if isNotSubvolume(err) {
			return false, nil
		}
		return false, err
	}
	return len(out) > 0, nil
}

func isNotSubvolume(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "not a btrfs") || strings.Contains(message, "not btrfs") || strings.Contains(message, "inval argument")
}

// CreateBtrfsSnapshot takes a read-only subvolume snapshot. Snapshots live
// under <source>.snapshots/ so the parent stays a plain subvolume.
func CreateBtrfsSnapshot(ctx context.Context, run SnapshotRunner, source, name string) error {
	if !snapshotNamePattern.MatchString(name) {
		return errors.New("invalid snapshot name")
	}
	_, err := run(ctx, "btrfs", "subvolume", "snapshot", "-r", source, source+".snapshots/"+name)
	return err
}

// ListBtrfsSnapshots parses `btrfs subvolume list -o -s` output for a source.
func ListBtrfsSnapshots(ctx context.Context, run SnapshotRunner, source string) ([]Snapshot, error) {
	out, err := run(ctx, "btrfs", "subvolume", "list", "-o", "-s", source)
	if err != nil {
		return nil, err
	}
	result := make([]Snapshot, 0)
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		otime, path := -1, -1
		for index, field := range fields {
			switch field {
			case "otime":
				otime = index
			case "path":
				path = index
			}
		}
		if otime < 0 || path < 0 || otime+2 >= len(fields) || path+1 >= len(fields) {
			continue
		}
		created, parseErr := time.Parse("2006-01-02 15:04:05", fields[otime+1]+" "+fields[otime+2])
		if parseErr != nil {
			continue
		}
		name := fields[path+1]
		if index := strings.LastIndexByte(name, '/'); index >= 0 {
			name = name[index+1:]
		}
		result = append(result, Snapshot{
			ID:        "btrfs/" + source + "@" + name,
			Kind:      SnapshotBtrfs,
			Source:    source,
			Name:      name,
			CreatedAt: created.UTC(),
			Readonly:  true,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}

// DeleteBtrfsSnapshot removes a read-only snapshot subvolume.
func DeleteBtrfsSnapshot(ctx context.Context, run SnapshotRunner, source, name string) error {
	if !snapshotNamePattern.MatchString(name) {
		return errors.New("invalid snapshot name")
	}
	_, err := run(ctx, "btrfs", "subvolume", "delete", source+".snapshots/"+name)
	return err
}

// DetectZfs reports whether a dataset is managed by zfs.
func DetectZfs(ctx context.Context, run SnapshotRunner, dataset string) (bool, error) {
	_, err := run(ctx, "zfs", "list", "-H", "-o", "name", dataset)
	if err != nil {
		if zfsDatasetMissing(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func zfsDatasetMissing(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "does not exist") || strings.Contains(message, "no such")
}

// CreateZfsSnapshot creates a zfs snapshot. ZFS snapshots are immutable by
// construction.
func CreateZfsSnapshot(ctx context.Context, run SnapshotRunner, dataset, name string) error {
	if !snapshotNamePattern.MatchString(name) {
		return errors.New("invalid snapshot name")
	}
	_, err := run(ctx, "zfs", "snapshot", dataset+"@"+name)
	return err
}

// ListZfsSnapshots parses `zfs list -H -t snapshot` output for one dataset.
func ListZfsSnapshots(ctx context.Context, run SnapshotRunner, dataset string) ([]Snapshot, error) {
	out, err := run(ctx, "zfs", "list", "-H", "-t", "snapshot", "-o", "name,creation,used", "-r", "-d", "1", dataset)
	if err != nil {
		return nil, err
	}
	result := make([]Snapshot, 0)
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 7 {
			continue
		}
		reference := fields[0]
		at := strings.IndexByte(reference, '@')
		if at <= 0 {
			continue
		}
		source, name := reference[:at], reference[at+1:]
		if source != dataset {
			continue
		}
		// `zfs list -H -o name,creation,used` renders creation as
		// "Mon Jan 2 15:04 2006" across five columns before the size.
		created, parseErr := time.Parse("Mon Jan _2 15:04 2006", strings.Join(fields[1:6], " "))
		if parseErr != nil {
			continue
		}
		result = append(result, Snapshot{
			ID:        "zfs/" + reference,
			Kind:      SnapshotZfs,
			Source:    source,
			Name:      name,
			CreatedAt: created.UTC(),
			UsedBytes: parseZfsSize(fields[6]),
			Readonly:  true,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}

// DeleteZfsSnapshot destroys a zfs snapshot.
func DeleteZfsSnapshot(ctx context.Context, run SnapshotRunner, dataset, name string) error {
	if !snapshotNamePattern.MatchString(name) {
		return errors.New("invalid snapshot name")
	}
	_, err := run(ctx, "zfs", "destroy", dataset+"@"+name)
	return err
}

// parseZfsSize converts zfs human sizes (1.05M, 20K, 0B) to bytes.
func parseZfsSize(value string) uint64 {
	if value == "" || value == "-" {
		return 0
	}
	multipliers := map[string]uint64{"B": 1, "K": 1 << 10, "M": 1 << 20, "G": 1 << 30, "T": 1 << 40, "P": 1 << 50}
	unit := value[len(value)-1:]
	multiplier, known := multipliers[unit]
	if !known {
		return 0
	}
	number, err := strconv.ParseFloat(strings.TrimSuffix(value, unit), 64)
	if err != nil {
		return 0
	}
	return uint64(number * float64(multiplier))
}
