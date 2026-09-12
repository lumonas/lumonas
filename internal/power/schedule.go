package power

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type Schedule struct {
	Enabled bool   `json:"enabled"`
	Action  string `json:"action"`
	Time    string `json:"time"`
	Days    string `json:"days"`
}

func (s Schedule) Validate() error {
	if s.Action != "shutdown" && s.Action != "reboot" {
		return errors.New("scheduled power action must be shutdown or reboot")
	}
	if len(s.Time) != 5 || s.Time[2] != ':' || s.Time[0] < '0' || s.Time[0] > '9' || s.Time[1] < '0' || s.Time[1] > '9' || s.Time[3] < '0' || s.Time[3] > '9' || s.Time[4] < '0' || s.Time[4] > '9' {
		return errors.New("scheduled power time must use HH:MM")
	}
	if _, err := time.Parse("15:04", s.Time); err != nil {
		return errors.New("scheduled power time must use HH:MM")
	}
	if _, err := scheduleDays(s.Days); err != nil {
		return err
	}
	return nil
}

func (s Schedule) Due(now time.Time) bool {
	if !s.Enabled || s.Validate() != nil {
		return false
	}
	clock, _ := time.Parse("15:04", s.Time)
	local := now.In(time.Local)
	return local.Hour() == clock.Hour() && local.Minute() == clock.Minute() && dayAllowed(s.Days, local.Weekday())
}

func scheduleDays(value string) (map[time.Weekday]bool, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" || value == "daily" {
		return map[time.Weekday]bool{
			time.Sunday: true, time.Monday: true, time.Tuesday: true,
			time.Wednesday: true, time.Thursday: true, time.Friday: true, time.Saturday: true,
		}, nil
	}
	if value == "weekdays" {
		return map[time.Weekday]bool{time.Monday: true, time.Tuesday: true, time.Wednesday: true, time.Thursday: true, time.Friday: true}, nil
	}
	if value == "weekends" {
		return map[time.Weekday]bool{time.Saturday: true, time.Sunday: true}, nil
	}
	result := make(map[time.Weekday]bool)
	for _, item := range strings.Split(value, ",") {
		weekday, ok := parseWeekday(strings.TrimSpace(item))
		if !ok {
			return nil, fmt.Errorf("scheduled power day %q is invalid", item)
		}
		result[weekday] = true
	}
	if len(result) == 0 {
		return nil, errors.New("scheduled power days are required")
	}
	return result, nil
}

func dayAllowed(value string, weekday time.Weekday) bool {
	days, err := scheduleDays(value)
	return err == nil && days[weekday]
}

func parseWeekday(value string) (time.Weekday, bool) {
	switch value {
	case "sun", "sunday":
		return time.Sunday, true
	case "mon", "monday":
		return time.Monday, true
	case "tue", "tuesday":
		return time.Tuesday, true
	case "wed", "wednesday":
		return time.Wednesday, true
	case "thu", "thursday":
		return time.Thursday, true
	case "fri", "friday":
		return time.Friday, true
	case "sat", "saturday":
		return time.Saturday, true
	default:
		return time.Sunday, false
	}
}
