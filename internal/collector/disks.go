package collector

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/runner"
)

type lsblkResponse struct {
	BlockDevices []lsblkDevice `json:"blockdevices"`
}
type lsblkDevice struct {
	Name       string        `json:"name"`
	Path       string        `json:"path"`
	Type       string        `json:"type"`
	Size       uint64        `json:"size"`
	Model      string        `json:"model"`
	Serial     string        `json:"serial"`
	WWN        string        `json:"wwn"`
	Rota       any           `json:"rota"`
	Tran       string        `json:"tran"`
	FSType     string        `json:"fstype"`
	UUID       string        `json:"uuid"`
	PartUUID   string        `json:"partuuid"`
	PTUUID     string        `json:"ptuuid"`
	Mountpoint string        `json:"mountpoint"`
	Children   []lsblkDevice `json:"children,omitempty"`
}

type CommandRunner func(name string, args ...string) ([]byte, error)

var lookupCommand = exec.LookPath

func SystemRunner(name string, args ...string) ([]byte, error) {
	return runner.Output(name, args...)
}

func Disks(run CommandRunner) ([]model.Disk, error) {
	if run == nil {
		run = SystemRunner
	}
	out, err := run("lsblk", "-J", "--tree", "-b", "-o", "NAME,PATH,TYPE,SIZE,MODEL,SERIAL,WWN,ROTA,TRAN,FSTYPE,UUID,PARTUUID,PTUUID,MOUNTPOINT")
	if err != nil {
		if runtime.GOOS != "linux" && errors.Is(err, exec.ErrNotFound) {
			return []model.Disk{}, nil
		}
		return nil, fmt.Errorf("lsblk: %w", err)
	}
	var response lsblkResponse
	if err := json.Unmarshal(out, &response); err != nil {
		return nil, fmt.Errorf("parse lsblk: %w", err)
	}
	result := make([]model.Disk, 0, len(response.BlockDevices))
	for _, d := range response.BlockDevices {
		// Only whole devices are manageable. A loopback attachment is reported
		// by lsblk as type "loop" until it carries a partition table, so it is
		// accepted only once it holds one, which is also when it starts
		// reporting the identity the storage layer plans are keyed on.
		if d.Type != "disk" && !(d.Type == "loop" && strings.TrimSpace(d.PTUUID) != "") {
			continue
		}
		d = enrichFromUdev(run, d)
		d = inheritFilesystemMetadata(d)
		id := StableID(d)
		// A device with no WWN, serial, partition-table GUID, or filesystem UUID
		// has no identity that survives a reboot or a device-letter change, so
		// it cannot be the target of a storage plan. Such placeholder entries do
		// appear on hosts that expose unused network block devices or raw
		// multipath slots. Drop them here so every disk in the inventory is
		// something the storage layer can actually key a plan on.
		if !model.HasStableDiskIdentity(id) {
			continue
		}
		rotational := asBool(d.Rota)
		iface := d.Tran
		if iface == "" {
			iface = "unknown"
		}
		health := model.Healthy
		if d.Size == 0 {
			health = model.Warning
		}
		result = append(result, model.Disk{
			ID: id, Name: d.Name, CurrentPath: d.Path, Model: strings.TrimSpace(d.Model), Serial: strings.TrimSpace(d.Serial), WWN: strings.TrimSpace(d.WWN), GPTDiskGUID: strings.TrimSpace(d.PTUUID), SizeBytes: d.Size,
			Role: "unknown", Rotational: rotational, Interface: iface, Health: health, Filesystem: d.FSType, FilesystemUUID: strings.TrimSpace(d.UUID), PartitionUUID: strings.TrimSpace(d.PartUUID), Mounted: strings.TrimSpace(d.Mountpoint) != "", LastSeen: time.Now().UTC(),
			SMART: model.SmartSummary{Overall: health},
		})
	}
	if _, err := exec.LookPath("smartctl"); err == nil {
		for index := range result {
			if result[index].CurrentPath == "" {
				continue
			}
			if details, smartErr := ReadSMART(run, result[index].CurrentPath); smartErr == nil {
				result[index].SMART = details.Summary
				result[index].Temperature = details.TemperatureC
			}
		}
	}
	return result, nil
}

// inheritFilesystemMetadata promotes the best partition-level filesystem
// metadata to the disk record. lsblk reports mounted filesystems on child
// rows (for example /dev/sda1), while the API models a physical disk as one
// resource. Prefer a mounted child so safety checks cannot miss an active
// mount; otherwise use the first child that exposes filesystem metadata.
func inheritFilesystemMetadata(device lsblkDevice) lsblkDevice {
	var mounted, fallback *lsblkDevice
	var visit func([]lsblkDevice)
	visit = func(children []lsblkDevice) {
		for index := range children {
			child := &children[index]
			if child.Mountpoint != "" {
				copy := *child
				mounted = &copy
			} else if fallback == nil && (child.UUID != "" || child.FSType != "" || child.PartUUID != "") {
				copy := *child
				fallback = &copy
			}
			visit(child.Children)
		}
	}
	visit(device.Children)
	candidate := mounted
	if candidate == nil {
		candidate = fallback
	}
	if candidate == nil {
		return device
	}
	if device.UUID == "" {
		device.UUID = candidate.UUID
	}
	if device.FSType == "" {
		device.FSType = candidate.FSType
	}
	if device.PartUUID == "" {
		device.PartUUID = candidate.PartUUID
	}
	if device.Mountpoint == "" {
		device.Mountpoint = candidate.Mountpoint
	}
	return device
}

// enrichFromUdev fills identity fields that can be absent from lsblk on
// USB/SAS bridges or during early device discovery. It is deliberately
// read-only and best-effort: lsblk remains the primary inventory source, while
// udev properties add stable serial/WWN/GPT/filesystem metadata when present.
func enrichFromUdev(run CommandRunner, device lsblkDevice) lsblkDevice {
	if device.Path == "" {
		return device
	}
	if _, err := lookupCommand("udevadm"); err != nil {
		return device
	}
	output, err := run("udevadm", "info", "--query=property", "--name", device.Path)
	if err != nil {
		return device
	}
	properties := parseUdevProperties(string(output))
	if device.WWN == "" {
		device.WWN = firstProperty(properties, "ID_WWN", "ID_WWN_WITH_EXTENSION")
	}
	if device.Serial == "" {
		device.Serial = firstProperty(properties, "ID_SERIAL_SHORT", "ID_SERIAL")
	}
	if device.Model == "" {
		device.Model = firstProperty(properties, "ID_MODEL")
	}
	if device.UUID == "" {
		device.UUID = firstProperty(properties, "ID_FS_UUID")
	}
	if device.PTUUID == "" {
		device.PTUUID = firstProperty(properties, "ID_PART_TABLE_UUID")
	}
	if device.Tran == "" {
		device.Tran = firstProperty(properties, "ID_BUS")
	}
	return device
}

func parseUdevProperties(output string) map[string]string {
	properties := make(map[string]string)
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) == "" {
			continue
		}
		properties[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return properties
}

func firstProperty(properties map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(properties[key]); value != "" {
			return value
		}
	}
	return ""
}

func StableID(d lsblkDevice) string {
	if value := strings.TrimSpace(d.WWN); value != "" {
		return "wwn:" + value
	}
	if value := strings.TrimSpace(d.Serial); value != "" {
		return "serial:" + value
	}
	if value := strings.TrimSpace(d.PTUUID); value != "" {
		return "gpt:" + value
	}
	if value := strings.TrimSpace(d.UUID); value != "" {
		return "uuid:" + value
	}
	return "path:" + d.Path
}

func asBool(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case float64:
		return v != 0
	case string:
		b, _ := strconv.ParseBool(v)
		return b || v == "1"
	default:
		return false
	}
}
