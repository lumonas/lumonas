package collector

import (
	"fmt"
	"sort"
	"strings"

	"github.com/lumonas/lumonas/internal/model"
)

// SMARTTrend computes simple per-sample slopes. The values are deliberately
// not presented as a failure probability: a positive slope is a useful
// signal for investigation, while the current SMART status remains the
// source of truth for severity.
func SMARTTrend(samples []model.SMARTSample) model.SMARTTrend {
	values := append([]model.SMARTSample(nil), samples...)
	sort.Slice(values, func(i, j int) bool { return values[i].CapturedAt.Before(values[j].CapturedAt) })
	trend := model.SMARTTrend{SampleCount: len(values), Status: model.Healthy, Summary: "Not enough history for a trend"}
	if len(values) == 0 {
		return trend
	}
	trend.DiskID = values[0].DiskID
	trend.Since = &values[0].CapturedAt
	latest := values[len(values)-1]
	trend.Status = latest.Summary.Overall
	if len(values) == 1 {
		trend.Summary = "First SMART sample recorded"
		return trend
	}
	first := values[0]
	days := latest.CapturedAt.Sub(first.CapturedAt).Hours() / 24
	if days < 1.0/24.0 {
		days = 1.0 / 24.0
	}
	delta := func(last, initial int64) float64 { return float64(last-initial) / days }
	trend.ReallocatedSlope = delta(latest.Summary.ReallocatedSectors, first.Summary.ReallocatedSectors)
	trend.PendingSlope = delta(latest.Summary.PendingSectors, first.Summary.PendingSectors)
	trend.UncorrectableSlope = delta(latest.Summary.UncorrectableSectors, first.Summary.UncorrectableSectors)
	trend.CRCSlope = delta(latest.Summary.CRCErrors, first.Summary.CRCErrors)
	if latest.TemperatureC != nil && first.TemperatureC != nil {
		trend.TemperatureSlope = (*latest.TemperatureC - *first.TemperatureC) / days
	}

	var signals []string
	if latest.Summary.Overall == model.Critical {
		trend.Status = model.Critical
		signals = append(signals, "SMART self-assessment is failing")
	}
	if latest.Summary.PendingSectors > 0 || latest.Summary.UncorrectableSectors > 0 {
		if trend.Status != model.Critical {
			trend.Status = model.Warning
		}
		signals = append(signals, "media errors need attention")
	}
	if trend.ReallocatedSlope > 0 || trend.PendingSlope > 0 || trend.UncorrectableSlope > 0 || trend.CRCSlope > 0 {
		if trend.Status == model.Healthy {
			trend.Status = model.Attention
		}
		signals = append(signals, "one or more error counters increased")
	}
	if len(signals) == 0 {
		trend.Summary = fmt.Sprintf("Stable across %d samples", len(values))
	} else {
		trend.Summary = strings.Join(signals, "; ")
	}
	return trend
}
