package quotas

import (
	"errors"
	"fmt"
)

type Policy struct {
	ID             string `json:"id"`
	TargetType     string `json:"targetType"` // share, user, group
	TargetID       string `json:"targetId"`
	LimitBytes     int64  `json:"limitBytes"`
	WarningPercent int    `json:"warningPercent"`
}

type Status struct {
	Policy     Policy  `json:"policy"`
	UsedBytes  int64   `json:"usedBytes"`
	Percent    float64 `json:"percent"`
	State      string  `json:"state"` // healthy, warning, over
	MeasuredAt string  `json:"measuredAt"`
}

func (p Policy) Validate() error {
	if p.ID == "" || p.TargetID == "" {
		return errors.New("quota target is required")
	}
	if p.TargetType != "share" && p.TargetType != "user" && p.TargetType != "group" {
		return fmt.Errorf("unsupported quota target type %q", p.TargetType)
	}
	if p.LimitBytes < 1<<20 {
		return errors.New("quota limit must be at least 1 MiB")
	}
	if p.WarningPercent < 50 || p.WarningPercent > 99 {
		return errors.New("warning threshold must be between 50 and 99 percent")
	}
	return nil
}
