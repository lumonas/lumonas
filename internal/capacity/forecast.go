package capacity

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

type Forecast struct {
	ResourceID          string     `json:"resourceId"`
	TotalBytes          uint64     `json:"totalBytes"`
	UsedBytes           uint64     `json:"usedBytes"`
	SampleCount         int        `json:"sampleCount"`
	WindowDays          float64    `json:"windowDays"`
	GrowthBytesPerDay   float64    `json:"growthBytesPerDay"`
	DaysToNinetyPercent *float64   `json:"daysToNinetyPercent,omitempty"`
	DaysToFull          *float64   `json:"daysToFull,omitempty"`
	EstimatedFullAt     *time.Time `json:"estimatedFullAt,omitempty"`
	SampleAgeHours      float64    `json:"sampleAgeHours"`
	Stale               bool       `json:"stale"`
	Confidence          string     `json:"confidence"`
	Available           bool       `json:"available"`
	Message             string     `json:"message,omitempty"`
}

// Build returns a deliberately conservative linear forecast. It does not
// report a prediction until there are at least three samples spanning a day,
// which avoids turning a restart or a transient filesystem reading into an
// operational alert.
func Build(resourceID string, snapshots []model.CapacitySnapshot, now time.Time) Forecast {
	forecast := Forecast{ResourceID: resourceID, SampleCount: len(snapshots)}
	if len(snapshots) == 0 {
		forecast.Message = "not enough capacity history"
		return forecast
	}
	samples := append([]model.CapacitySnapshot(nil), snapshots...)
	sort.Slice(samples, func(i, j int) bool { return samples[i].CapturedAt.Before(samples[j].CapturedAt) })
	latest := samples[len(samples)-1]
	forecast.TotalBytes, forecast.UsedBytes = latest.TotalBytes, latest.UsedBytes
	forecast.SampleAgeHours = math.Max(0, now.Sub(latest.CapturedAt).Hours())
	forecast.Stale = forecast.SampleAgeHours > 24
	if len(samples) < 3 {
		forecast.Message = "not enough capacity history"
		return forecast
	}
	span := samples[len(samples)-1].CapturedAt.Sub(samples[0].CapturedAt)
	forecast.WindowDays = span.Hours() / 24
	switch {
	case forecast.WindowDays >= 30 && len(samples) >= 8:
		forecast.Confidence = "high"
	case forecast.WindowDays >= 7:
		forecast.Confidence = "medium"
	default:
		forecast.Confidence = "low"
	}
	if forecast.Stale {
		forecast.Confidence = "low"
	}
	if span < 24*time.Hour || latest.TotalBytes == 0 {
		forecast.Message = "capacity history must span at least one day"
		return forecast
	}
	var sumX, sumY, sumXX, sumXY float64
	start := samples[0].CapturedAt
	for _, sample := range samples {
		x := sample.CapturedAt.Sub(start).Hours() / 24
		y := float64(sample.UsedBytes)
		sumX += x
		sumY += y
		sumXX += x * x
		sumXY += x * y
	}
	denominator := float64(len(samples))*sumXX - sumX*sumX
	if denominator <= 0 {
		forecast.Message = "capacity history has no usable time range"
		return forecast
	}
	slope := (float64(len(samples))*sumXY - sumX*sumY) / denominator
	forecast.GrowthBytesPerDay = slope
	if slope <= 0 {
		forecast.Message = "capacity is not currently growing"
		return forecast
	}
	target := float64(latest.TotalBytes) * 0.9
	remaining := target - float64(latest.UsedBytes)
	days := 0.0
	if remaining > 0 {
		days = remaining / slope
	}
	forecast.DaysToNinetyPercent = &days
	fullDays := math.Max(0, (float64(latest.TotalBytes)-float64(latest.UsedBytes))/slope)
	forecast.DaysToFull = &fullDays
	if fullDays <= 365*1000 {
		fullAt := now.AddDate(0, 0, int(math.Ceil(fullDays)))
		forecast.EstimatedFullAt = &fullAt
	}
	forecast.Available = true
	forecast.Message = fmt.Sprintf("at the recent growth rate, %s may reach 90%% in %.0f days", resourceID, days)
	return forecast
}

func (f Forecast) GrowthGiBPerDay() float64 { return f.GrowthBytesPerDay / math.Pow(1024, 3) }
