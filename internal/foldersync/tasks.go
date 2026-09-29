package foldersync

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"
)

type Endpoint struct {
	Kind          string `json:"kind"` // share, mount, destination
	ShareID       string `json:"shareId,omitempty"`
	MountPath     string `json:"mountPath,omitempty"`
	DestinationID string `json:"destinationId,omitempty"`
	Path          string `json:"path,omitempty"`
	Prefix        string `json:"prefix,omitempty"`
}

type Task struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Direction      string     `json:"direction,omitempty"` // one-way, two-way
	Source         Endpoint   `json:"source"`
	Destination    Endpoint   `json:"destination"`
	Mode           string     `json:"mode"` // copy, mirror
	DeepCheck      bool       `json:"deepCheck"`
	IgnorePatterns []string   `json:"ignorePatterns,omitempty"`
	MirrorApproved bool       `json:"mirrorApproved"`
	ScheduleKind   string     `json:"scheduleKind,omitempty"` // daily, weekly, manual
	TimeOfDay      string     `json:"timeOfDay,omitempty"`
	Weekday        string     `json:"weekday,omitempty"`
	OnUSBAttach    bool       `json:"onUsbAttach,omitempty"`
	Enabled        bool       `json:"enabled"`
	NextDueAt      *time.Time `json:"nextDueAt,omitempty"`
	LastRunAt      *time.Time `json:"lastRunAt,omitempty"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

type Run struct {
	ID         string     `json:"id"`
	TaskID     string     `json:"taskId"`
	State      string     `json:"state"`
	Trigger    string     `json:"trigger"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	Files      int        `json:"files"`
	Bytes      int64      `json:"bytes"`
	Deleted    int        `json:"deleted"`
	Conflicts  int        `json:"conflicts,omitempty"`
	Plan       Plan       `json:"plan"`
	Error      string     `json:"error,omitempty"`
}

func (t Task) Validate() error {
	if strings.TrimSpace(t.Name) == "" || len(t.Name) > 80 || strings.ContainsAny(t.Name, "\x00\r\n") {
		return errors.New("task name is required and must be under 80 characters")
	}
	if t.ID == "" {
		return errors.New("task id is required")
	}
	if err := validateEndpoint(t.Source); err != nil {
		return fmt.Errorf("source: %w", err)
	}
	if err := validateEndpoint(t.Destination); err != nil {
		return fmt.Errorf("destination: %w", err)
	}
	if t.Source.Kind == "destination" && t.Destination.Kind == "destination" {
		return errors.New("at least one endpoint must be local")
	}
	if t.Direction != "" && t.Direction != "one-way" && t.Direction != "two-way" {
		return errors.New("direction must be one-way or two-way")
	}
	if len(t.IgnorePatterns) > 128 {
		return errors.New("at most 128 ignore patterns are allowed")
	}
	for _, pattern := range t.IgnorePatterns {
		if pattern == "" || len(pattern) > 256 || strings.ContainsAny(pattern, "\\\x00\r\n") || filepath.IsAbs(pattern) || path.Clean(pattern) != pattern || pattern == "." {
			return errors.New("ignore patterns must be clean relative globs under the managed root")
		}
		for _, part := range strings.Split(pattern, "/") {
			if part == ".." {
				return errors.New("ignore patterns cannot traverse outside the managed root")
			}
		}
		if _, err := path.Match(pattern, "probe"); err != nil {
			return fmt.Errorf("invalid ignore pattern %q: %w", pattern, err)
		}
	}
	if t.Direction == "two-way" {
		if t.Source.Kind == "destination" || t.Destination.Kind == "destination" {
			return errors.New("two-way sync currently requires two local managed endpoints")
		}
		if t.Mode != "copy" {
			return errors.New("two-way sync preserves deletions and does not support mirror mode")
		}
		if t.OnUSBAttach {
			return errors.New("USB attach triggers are not supported for two-way sync")
		}
	}
	if t.OnUSBAttach && (t.Source.Kind != "share" || t.Destination.Kind != "mount") {
		return errors.New("USB attach runs require a managed share source and a mounted-volume destination")
	}
	if t.Mode != "copy" && t.Mode != "mirror" {
		return errors.New("mode must be copy or mirror")
	}
	if t.Mode == "mirror" && !t.MirrorApproved {
		return errors.New("mirror mode requires explicit approval")
	}
	if t.ScheduleKind != "" && t.ScheduleKind != "manual" && t.ScheduleKind != "daily" && t.ScheduleKind != "weekly" {
		return errors.New("schedule must be manual, daily, or weekly")
	}
	if t.ScheduleKind == "daily" || t.ScheduleKind == "weekly" {
		if _, err := time.Parse("15:04", t.TimeOfDay); err != nil {
			return errors.New("schedule time must use HH:MM")
		}
	}
	if t.ScheduleKind == "weekly" {
		switch strings.ToLower(t.Weekday) {
		case "monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday":
		default:
			return errors.New("weekly schedule requires a valid weekday")
		}
	}
	return nil
}

func validateEndpoint(e Endpoint) error {
	switch e.Kind {
	case "share":
		if e.ShareID == "" {
			return errors.New("share id is required")
		}
	case "mount":
		if !filepath.IsAbs(e.MountPath) || filepath.Clean(e.MountPath) != e.MountPath || !(strings.HasPrefix(e.MountPath, "/srv/disks/") || strings.HasPrefix(e.MountPath, "/srv/pools/")) {
			return errors.New("mount path must be a managed disk or pool mount")
		}
	case "destination":
		if e.DestinationID == "" {
			return errors.New("backup destination is required")
		}
		if e.Prefix != "" && (filepath.IsAbs(e.Prefix) || strings.Contains(e.Prefix, "\\") || strings.Contains(e.Prefix, "..")) {
			return errors.New("remote prefix is unsafe")
		}
	default:
		return errors.New("endpoint must be a share, managed mount, or backup destination")
	}
	if e.Path != "" && (filepath.IsAbs(e.Path) || filepath.Clean(e.Path) != e.Path || strings.HasPrefix(e.Path, "..") || strings.ContainsAny(e.Path, "\\\x00\r\n")) {
		return errors.New("endpoint path must remain inside its managed root")
	}
	return nil
}
