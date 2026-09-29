package monitoring

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/storage"
)

type AlertRule struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Condition       string     `json:"condition"`
	Severity        string     `json:"severity"`
	Routes          []string   `json:"routes"`
	Enabled         bool       `json:"enabled"`
	LastTriggeredAt *time.Time `json:"lastTriggeredAt,omitempty"`
}

func (r AlertRule) Validate() error {
	if strings.TrimSpace(r.ID) == "" || strings.TrimSpace(r.Name) == "" || strings.TrimSpace(r.Condition) == "" {
		return errors.New("alert rule id, name, and condition are required")
	}
	switch strings.ToLower(strings.TrimSpace(r.Severity)) {
	case "info", "attention", "warning", "critical":
	default:
		return fmt.Errorf("unsupported alert rule severity %q", r.Severity)
	}
	if len(r.Routes) == 0 {
		return errors.New("alert rule requires at least one route")
	}
	seen := make(map[string]bool, len(r.Routes))
	for _, route := range r.Routes {
		route = strings.TrimSpace(route)
		if route == "" || strings.ContainsAny(route, "\x00\r\n") {
			return errors.New("alert rule contains an invalid route")
		}
		if seen[route] {
			return fmt.Errorf("alert rule contains duplicate route %q", route)
		}
		seen[route] = true
	}
	return nil
}

type NotificationChannel struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Label      string `json:"label"`
	Target     string `json:"target,omitempty"`
	Configured bool   `json:"configured"`
	Enabled    bool   `json:"enabled"`
}

type Schedule struct {
	ID                     string     `json:"id"`
	Name                   string     `json:"name"`
	JobType                string     `json:"jobType"`
	Kind                   string     `json:"kind"`
	TimeOfDay              string     `json:"timeOfDay"`
	Weekday                string     `json:"weekday,omitempty"`
	Enabled                bool       `json:"enabled"`
	LastStartedAt          *time.Time `json:"lastStartedAt,omitempty"`
	NextDueAt              *time.Time `json:"nextDueAt,omitempty"`
	Schedule               string     `json:"schedule"`
	Next                   string     `json:"next"`
	SnapshotKind           string     `json:"snapshotKind,omitempty"`
	SnapshotSource         string     `json:"snapshotSource,omitempty"`
	SnapshotLabel          string     `json:"snapshotLabel,omitempty"`
	SnapshotKeep           int        `json:"snapshotKeep,omitempty"`
	SnapshotLockDays       int        `json:"snapshotLockDays,omitempty"`
	FilesystemKind         string     `json:"filesystemKind,omitempty"`
	FilesystemSource       string     `json:"filesystemSource,omitempty"`
	RelocationShareID      string     `json:"relocationShareId,omitempty"`
	RelocationResourceID   string     `json:"relocationResourceId,omitempty"`
	RelocationRelativePath string     `json:"relocationRelativePath,omitempty"`
}

const (
	ScheduleDaily  = "daily"
	ScheduleWeekly = "weekly"
	ScheduleEvent  = "event"
)

func (s Schedule) Validate() error {
	if strings.TrimSpace(s.ID) == "" || strings.TrimSpace(s.Name) == "" {
		return errors.New("schedule id and name are required")
	}
	switch s.Kind {
	case ScheduleEvent:
		if strings.TrimSpace(s.JobType) == "" {
			return errors.New("schedule job type is required")
		}
		return nil
	case ScheduleDaily, ScheduleWeekly:
	default:
		return fmt.Errorf("unsupported schedule kind %q", s.Kind)
	}
	switch s.JobType {
	case "smart.short", "smart.extended", "snapraid.sync", "snapraid.scrub", "backup.run":
	case "filesystem.scrub":
		if err := storage.ValidateScrubSource(storage.SnapshotKind(s.FilesystemKind), s.FilesystemSource); err != nil {
			return err
		}
	case "snapshot.create":
		if strings.TrimSpace(s.SnapshotKind) == "" || strings.TrimSpace(s.SnapshotSource) == "" {
			return errors.New("snapshot schedule kind and source are required")
		}
		if s.SnapshotKind != "btrfs" && s.SnapshotKind != "zfs" {
			return fmt.Errorf("unsupported snapshot kind %q", s.SnapshotKind)
		}
		if s.SnapshotKeep < 1 || s.SnapshotKeep > 365 {
			return errors.New("snapshot retention must keep between 1 and 365 snapshots")
		}
		if s.SnapshotLockDays < 0 || s.SnapshotLockDays > 3650 {
			return errors.New("snapshot retention lock must be between 0 and 3650 days")
		}
	case "share.relocate":
		if strings.TrimSpace(s.RelocationShareID) == "" || !filepath.IsAbs(s.RelocationResourceID) || filepath.Clean(s.RelocationResourceID) != s.RelocationResourceID {
			return errors.New("share relocation schedule requires a share and managed storage location")
		}
		if s.RelocationRelativePath == "" || filepath.IsAbs(s.RelocationRelativePath) || filepath.Clean(s.RelocationRelativePath) != s.RelocationRelativePath || s.RelocationRelativePath == "." || s.RelocationRelativePath == ".." || strings.HasPrefix(s.RelocationRelativePath, ".."+string(filepath.Separator)) || strings.ContainsAny(s.RelocationRelativePath, "\\\x00\r\n") {
			return errors.New("share relocation schedule path must stay inside the selected storage location")
		}
	default:
		return fmt.Errorf("unsupported schedule job type %q", s.JobType)
	}
	if _, _, err := ParseTimeOfDay(s.TimeOfDay); err != nil {
		return err
	}
	if s.Kind == ScheduleWeekly {
		if _, err := ParseWeekday(s.Weekday); err != nil {
			return err
		}
	}
	return nil
}

// Humanize refreshes the derived human-readable cadence string.
func (s *Schedule) Humanize() {
	switch s.Kind {
	case ScheduleDaily:
		s.Schedule = "Daily at " + s.TimeOfDay
	case ScheduleWeekly:
		if weekday, err := ParseWeekday(s.Weekday); err == nil {
			s.Schedule = pluralWeekday(weekday) + " at " + s.TimeOfDay
		}
	default:
		s.Schedule = "After every change"
	}
}

// DescribeNext renders the next-run description relative to now.
func (s Schedule) DescribeNext(now time.Time) string {
	if s.Kind == ScheduleEvent {
		return "on change"
	}
	if !s.Enabled {
		return "paused"
	}
	if s.NextDueAt == nil {
		return "pending"
	}
	until := s.NextDueAt.Sub(now)
	if until <= 0 {
		return "due now"
	}
	days := int(until.Hours()) / 24
	hours := int(until.Hours()) % 24
	minutes := int(until.Minutes()) % 60
	switch {
	case days >= 1:
		return fmt.Sprintf("in %dd %dh", days, hours)
	case hours >= 1:
		return fmt.Sprintf("in %dh %dm", hours, minutes)
	default:
		return fmt.Sprintf("in %dm", minutes)
	}
}

// MarkStarted records that the schedule fired at now and advances the next due time.
func (s *Schedule) MarkStarted(now time.Time) {
	last := now
	s.LastStartedAt = &last
	next := NextOccurrence(*s, now)
	s.NextDueAt = &next
}

// NextOccurrence returns the next wall-clock occurrence strictly after now
// in now's location. Calendar-day arithmetic keeps the wall-clock time stable
// across daylight-saving transitions. Zero time for event schedules.
func NextOccurrence(schedule Schedule, now time.Time) time.Time {
	if schedule.Kind != ScheduleDaily && schedule.Kind != ScheduleWeekly {
		return time.Time{}
	}
	hour, minute, err := ParseTimeOfDay(schedule.TimeOfDay)
	if err != nil {
		return time.Time{}
	}
	candidate := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	if schedule.Kind == ScheduleWeekly {
		weekday, err := ParseWeekday(schedule.Weekday)
		if err != nil {
			return time.Time{}
		}
		delta := (int(weekday) - int(candidate.Weekday()) + 7) % 7
		candidate = candidate.AddDate(0, 0, delta)
	}
	if !candidate.After(now) {
		if schedule.Kind == ScheduleWeekly {
			candidate = candidate.AddDate(0, 0, 7)
		} else {
			candidate = candidate.AddDate(0, 0, 1)
		}
	}
	return candidate
}

// ParseTimeOfDay parses an HH:MM 24-hour clock string.
func ParseTimeOfDay(value string) (int, int, error) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("time of day %q must use HH:MM format", value)
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return 0, 0, fmt.Errorf("time of day %q has an invalid hour", value)
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("time of day %q has an invalid minute", value)
	}
	return hour, minute, nil
}

// ParseWeekday parses a lowercase weekday name.
func ParseWeekday(value string) (time.Weekday, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "sunday":
		return time.Sunday, nil
	case "monday":
		return time.Monday, nil
	case "tuesday":
		return time.Tuesday, nil
	case "wednesday":
		return time.Wednesday, nil
	case "thursday":
		return time.Thursday, nil
	case "friday":
		return time.Friday, nil
	case "saturday":
		return time.Saturday, nil
	default:
		return time.Sunday, fmt.Errorf("unsupported weekday %q", value)
	}
}

func pluralWeekday(weekday time.Weekday) string {
	names := map[time.Weekday]string{
		time.Sunday: "Sundays", time.Monday: "Mondays", time.Tuesday: "Tuesdays",
		time.Wednesday: "Wednesdays", time.Thursday: "Thursdays", time.Friday: "Fridays",
		time.Saturday: "Saturdays",
	}
	return names[weekday]
}

func DefaultAlertRules() []AlertRule {
	return []AlertRule{
		{ID: "rule-temp", Name: "Disk temperature high", Condition: "temperature > 45°C for 5 minutes", Severity: "warning", Routes: []string{"web"}, Enabled: true},
		{ID: "rule-smart", Name: "SMART attribute changed", Condition: "pending/reallocated sectors increase", Severity: "warning", Routes: []string{"web"}, Enabled: true},
		{ID: "rule-pool", Name: "Pool degraded or disk missing", Condition: "pool member offline", Severity: "critical", Routes: []string{"web"}, Enabled: true},
		{ID: "rule-sync", Name: "SnapRAID sync stale", Condition: "no successful sync in 48h", Severity: "attention", Routes: []string{"web"}, Enabled: true},
		{ID: "rule-backup", Name: "Backup job failed", Condition: "backup job state = failed", Severity: "warning", Routes: []string{"web"}, Enabled: true},
		{ID: "rule-container", Name: "Container unhealthy", Condition: "health check failing for 2 minutes", Severity: "warning", Routes: []string{"web"}, Enabled: true},
		{ID: "rule-filesystem", Name: "Filesystem nearly full", Condition: "filesystem usage above 80%", Severity: "warning", Routes: []string{"web"}, Enabled: true},
		{ID: "rule-ransomware", Name: "Unusual SMB file changes", Condition: "80 or more delete, directory-remove, or rename operations on one share in 5 minutes", Severity: "critical", Routes: []string{"web"}, Enabled: true},
		{ID: "rule-login", Name: "New admin sign-in", Condition: "login from unseen device", Severity: "info", Routes: []string{"web"}, Enabled: false},
	}
}

func DefaultSchedules() []Schedule {
	return []Schedule{
		{ID: "sched-sync", Name: "SnapRAID sync", JobType: "snapraid.sync", Kind: ScheduleDaily, TimeOfDay: "02:00", Enabled: true},
		{ID: "sched-scrub", Name: "SnapRAID scrub", JobType: "snapraid.scrub", Kind: ScheduleWeekly, Weekday: "sunday", TimeOfDay: "03:00", Enabled: true},
		{ID: "sched-smart", Name: "SMART short tests", JobType: "smart.short", Kind: ScheduleWeekly, Weekday: "saturday", TimeOfDay: "04:00", Enabled: true},
		{ID: "sched-backup", Name: "App backups", JobType: "backup.run", Kind: ScheduleDaily, TimeOfDay: "03:30", Enabled: true},
		{ID: "sched-config", Name: "Config snapshot", JobType: "config.snapshot", Kind: ScheduleEvent, Enabled: true},
		{ID: "sched-snapshot", Name: "Filesystem snapshots", JobType: "snapshot.create", Kind: ScheduleDaily, TimeOfDay: "01:30", Enabled: false, SnapshotKind: "btrfs", SnapshotSource: "/srv/pools/media", SnapshotLabel: "scheduled", SnapshotKeep: 7},
	}
}
