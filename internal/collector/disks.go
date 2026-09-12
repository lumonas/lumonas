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
)

type lsblkResponse struct {
	BlockDevices []lsblkDevice `json:"blockdevices"`
}
type lsblkDevice struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Type       string `json:"type"`
	Size       uint64 `json:"size"`
	Model      string `json:"model"`
	Serial     string `json:"serial"`
	WWN        string `json:"wwn"`
	Rota       any    `json:"rota"`
	Tran       string `json:"tran"`
	FSType     string `json:"fstype"`
	UUID       string `json:"uuid"`
	PartUUID   string `json:"partuuid"`
	Mountpoint string `json:"mountpoint"`
}

type CommandRunner func(name string, args ...string) ([]byte, error)

func SystemRunner(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

func Disks(run CommandRunner) ([]model.Disk, error) {
	if run == nil {
		run = SystemRunner
	}
	out, err := run("lsblk", "-J", "-b", "-o", "NAME,PATH,TYPE,SIZE,MODEL,SERIAL,WWN,ROTA,TRAN,FSTYPE,UUID,PARTUUID,MOUNTPOINT")
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
		if d.Type != "disk" {
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
		id := StableID(d)
		result = append(result, model.Disk{
			ID: id, Name: d.Name, CurrentPath: d.Path, Model: strings.TrimSpace(d.Model), Serial: strings.TrimSpace(d.Serial), WWN: strings.TrimSpace(d.WWN), SizeBytes: d.Size,
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

func StableID(d lsblkDevice) string {
	if value := strings.TrimSpace(d.WWN); value != "" {
		return "wwn:" + value
	}
	if value := strings.TrimSpace(d.Serial); value != "" {
		return "serial:" + value
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
