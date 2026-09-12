package monitoring

import "time"

type AlertRule struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Condition       string     `json:"condition"`
	Severity        string     `json:"severity"`
	Routes          []string   `json:"routes"`
	Enabled         bool       `json:"enabled"`
	LastTriggeredAt *time.Time `json:"lastTriggeredAt,omitempty"`
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
	ID       string `json:"id"`
	Name     string `json:"name"`
	Schedule string `json:"schedule"`
	Next     string `json:"next"`
	Enabled  bool   `json:"enabled"`
}

func DefaultAlertRules() []AlertRule {
	return []AlertRule{
		{ID: "rule-temp", Name: "Disk temperature high", Condition: "temperature > 45°C for 5 minutes", Severity: "warning", Routes: []string{"web"}, Enabled: true},
		{ID: "rule-smart", Name: "SMART attribute changed", Condition: "pending/reallocated sectors increase", Severity: "warning", Routes: []string{"web"}, Enabled: true},
		{ID: "rule-pool", Name: "Pool degraded or disk missing", Condition: "pool member offline", Severity: "critical", Routes: []string{"web"}, Enabled: true},
		{ID: "rule-sync", Name: "SnapRAID sync stale", Condition: "no successful sync in 48h", Severity: "attention", Routes: []string{"web"}, Enabled: true},
		{ID: "rule-backup", Name: "Backup job failed", Condition: "backup job state = failed", Severity: "warning", Routes: []string{"web"}, Enabled: true},
		{ID: "rule-container", Name: "Container unhealthy", Condition: "health check failing for 2 minutes", Severity: "warning", Routes: []string{"web"}, Enabled: true},
		{ID: "rule-login", Name: "New admin sign-in", Condition: "login from unseen device", Severity: "info", Routes: []string{"web"}, Enabled: false},
	}
}

func DefaultSchedules() []Schedule {
	return []Schedule{
		{ID: "sched-sync", Name: "SnapRAID sync", Schedule: "Daily at 02:00", Next: "configured", Enabled: true},
		{ID: "sched-scrub", Name: "SnapRAID scrub", Schedule: "Sundays at 03:00", Next: "configured", Enabled: true},
		{ID: "sched-smart", Name: "SMART short tests", Schedule: "Saturdays at 04:00", Next: "configured", Enabled: true},
		{ID: "sched-backup", Name: "App backups", Schedule: "Daily at 03:30", Next: "configured", Enabled: true},
		{ID: "sched-config", Name: "Config snapshot", Schedule: "After every change", Next: "on change", Enabled: true},
	}
}
