package storage

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type ScrubStatus struct {
	Running   bool    `json:"running"`
	Progress  float64 `json:"progress"`
	Errors    bool    `json:"errors"`
	Cancelled bool    `json:"cancelled"`
	Message   string  `json:"message"`
}

var scrubProgressPattern = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)% done`)
var scrubErrorCountPattern = regexp.MustCompile(`(?:csum|verify|read|write|flush|corruption|generation|errors?)\s*[=:]\s*([0-9]+)`)
var zpoolErrorCountPattern = regexp.MustCompile(`with\s+([0-9]+)\s+errors?`)

func ValidateScrubSource(kind SnapshotKind, source string) error {
	switch kind {
	case SnapshotBtrfs:
		if source == "" || !filepath.IsAbs(source) || filepath.Clean(source) != source || !(strings.HasPrefix(source, "/srv/pools/") || strings.HasPrefix(source, "/srv/disks/")) {
			return errors.New("Btrfs scrub source must be a clean path inside a managed pool or disk mount")
		}
	case SnapshotZfs:
		if !validZfsDataset(source) {
			return errors.New("ZFS scrub source must be a valid pool or dataset name")
		}
	default:
		return fmt.Errorf("unsupported scrub filesystem %q", kind)
	}
	return nil
}

func ParseBtrfsScrubStatus(output string) (ScrubStatus, error) {
	status := ScrubStatus{Message: strings.TrimSpace(output)}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Status:") {
			value := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, "Status:")))
			switch value {
			case "running":
				status.Running = true
			case "finished":
				status.Running = false
			case "aborted":
				status.Running, status.Cancelled = false, true
			default:
				return ScrubStatus{}, fmt.Errorf("unknown Btrfs scrub status %q", value)
			}
		}
	}
	if !strings.Contains(output, "Status:") {
		return ScrubStatus{}, errors.New("Btrfs scrub status did not report a status")
	}
	if !status.Running {
		lower := strings.ToLower(output)
		if status.Cancelled {
			status.Progress, status.Message = 100, "cancelled"
		} else if strings.Contains(lower, "error summary: no errors") {
			status.Progress, status.Message = 100, "finished without errors"
		} else if strings.Contains(lower, "error summary:") {
			status.Progress = 100
			counts := scrubErrorCountPattern.FindAllStringSubmatch(lower, -1)
			for _, match := range counts {
				if match[1] != "0" {
					status.Errors = true
				}
			}
			if len(counts) == 0 {
				status.Errors = true
			}
			if status.Errors {
				status.Message = "finished; scrub error summary reported findings"
			} else {
				status.Message = "finished without errors"
			}
		} else {
			status.Progress, status.Message = 100, "finished"
		}
	}
	return status, nil
}

func ParseZpoolScrubStatus(output string) (ScrubStatus, error) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "scan:") {
			continue
		}
		lower := strings.ToLower(line)
		status := ScrubStatus{Message: line}
		if strings.Contains(lower, "scrub in progress") {
			status.Running = true
			if match := scrubProgressPattern.FindStringSubmatch(output); len(match) == 2 {
				progress, err := strconv.ParseFloat(match[1], 64)
				if err != nil || progress < 0 || progress > 100 {
					return ScrubStatus{}, errors.New("ZFS scrub returned invalid progress")
				}
				status.Progress = progress
			}
			return status, nil
		}
		if strings.Contains(lower, "scrub repaired") || strings.Contains(lower, "scrub canceled") || strings.Contains(lower, "none requested") {
			status.Progress = 100
			if strings.Contains(lower, "scrub canceled") {
				status.Cancelled = true
			}
			if match := zpoolErrorCountPattern.FindStringSubmatch(lower); len(match) == 2 && match[1] != "0" {
				status.Errors = true
				status.Message += " (errors found)"
			}
			return status, nil
		}
		return ScrubStatus{}, errors.New("ZFS scan status does not describe a scrub")
	}
	return ScrubStatus{}, errors.New("ZFS status did not include a scan summary")
}
