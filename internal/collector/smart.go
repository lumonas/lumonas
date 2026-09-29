package collector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/lumonas/lumonas/internal/model"
)

type smartJSON struct {
	SmartStatus *struct {
		Passed bool `json:"passed"`
	} `json:"smart_status"`
	Temperature *struct {
		Current float64 `json:"current"`
	} `json:"temperature"`
	PowerOnTime *struct {
		Hours int64 `json:"hours"`
	} `json:"power_on_time"`
	ATAAttributes *struct {
		Table []struct {
			Name string `json:"name"`
			Raw  struct {
				Value int64 `json:"value"`
			} `json:"raw"`
		} `json:"table"`
	} `json:"ata_smart_attributes"`
}

type SMARTDetails struct {
	Summary      model.SmartSummary
	TemperatureC *float64
}

func ReadSMART(run CommandRunner, path string) (SMARTDetails, error) {
	if strings.TrimSpace(path) == "" {
		return SMARTDetails{}, fmt.Errorf("disk path is required")
	}
	if run == nil {
		run = SystemRunner
	}
	out, err := run("smartctl", "-aj", path)
	if err != nil {
		return SMARTDetails{}, fmt.Errorf("smartctl: %w", err)
	}
	return decodeSMART(out)
}

func ReadSMARTContext(ctx context.Context, run CommandRunner, path string) (SMARTDetails, error) {
	if strings.TrimSpace(path) == "" {
		return SMARTDetails{}, fmt.Errorf("disk path is required")
	}
	var out []byte
	var err error
	if run != nil {
		out, err = run("smartctl", "-aj", path)
	} else {
		out, err = exec.CommandContext(ctx, "smartctl", "-aj", path).Output()
	}
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return SMARTDetails{}, ctx.Err()
		}
		return SMARTDetails{}, fmt.Errorf("smartctl: %w", err)
	}
	return decodeSMART(out)
}

func decodeSMART(out []byte) (SMARTDetails, error) {
	var payload smartJSON
	if err := json.Unmarshal(out, &payload); err != nil {
		return SMARTDetails{}, fmt.Errorf("parse smartctl: %w", err)
	}
	summary := model.SmartSummary{Overall: model.Healthy}
	if payload.SmartStatus != nil && !payload.SmartStatus.Passed {
		summary.Overall = model.Critical
	}
	if payload.PowerOnTime != nil {
		summary.PowerOnHours = payload.PowerOnTime.Hours
	}
	if payload.ATAAttributes != nil {
		for _, attr := range payload.ATAAttributes.Table {
			switch strings.ToLower(attr.Name) {
			case "reallocated_sector_count":
				summary.ReallocatedSectors = attr.Raw.Value
			case "current_pending_sector":
				summary.PendingSectors = attr.Raw.Value
			case "offline_uncorrectable":
				summary.UncorrectableSectors = attr.Raw.Value
			case "udma_crc_error_count":
				summary.CRCErrors = attr.Raw.Value
			}
		}
	}
	if summary.PendingSectors > 0 || summary.UncorrectableSectors > 0 {
		summary.Overall = model.Warning
	}
	var temperature *float64
	if payload.Temperature != nil {
		temperature = &payload.Temperature.Current
	}
	return SMARTDetails{Summary: summary, TemperatureC: temperature}, nil
}

func SMART(run CommandRunner, path string) (model.SmartSummary, error) {
	details, err := ReadSMART(run, path)
	return details.Summary, err
}

func SMARTContext(ctx context.Context, run CommandRunner, path string) (model.SmartSummary, error) {
	details, err := ReadSMARTContext(ctx, run, path)
	return details.Summary, err
}
