package collector

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lumonas/lumonas/internal/model"
)

type smartJSON struct {
	SmartStatus *struct {
		Passed bool `json:"passed"`
	} `json:"smart_status"`
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

func SMART(run CommandRunner, path string) (model.SmartSummary, error) {
	if strings.TrimSpace(path) == "" {
		return model.SmartSummary{}, fmt.Errorf("disk path is required")
	}
	if run == nil {
		run = SystemRunner
	}
	out, err := run("smartctl", "-aj", path)
	if err != nil {
		return model.SmartSummary{}, fmt.Errorf("smartctl: %w", err)
	}
	var payload smartJSON
	if err := json.Unmarshal(out, &payload); err != nil {
		return model.SmartSummary{}, fmt.Errorf("parse smartctl: %w", err)
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
	return summary, nil
}
