package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type DestinationType string

const (
	DestinationLocal DestinationType = "local"
	DestinationSFTP  DestinationType = "sftp"
	DestinationS3    DestinationType = "s3"
)

type RetentionPolicy struct {
	Generations int `json:"generations"`
	Daily       int `json:"daily"`
	Monthly     int `json:"monthly"`
}

func DefaultRetention() RetentionPolicy {
	return RetentionPolicy{Generations: 20, Daily: 30, Monthly: 12}
}

type Destination struct {
	ID                    string          `json:"id"`
	Name                  string          `json:"name"`
	Type                  DestinationType `json:"type"`
	Target                string          `json:"target"`
	Enabled               bool            `json:"enabled"`
	Retention             RetentionPolicy `json:"retention"`
	CredentialsConfigured bool            `json:"credentialsConfigured"`
	CreatedAt             time.Time       `json:"createdAt"`
	UpdatedAt             time.Time       `json:"updatedAt"`
}

type Credentials struct {
	Username       string `json:"username,omitempty"`
	Password       string `json:"password,omitempty"`
	PrivateKeyPath string `json:"privateKeyPath,omitempty"`
	AccessKey      string `json:"accessKey,omitempty"`
	SecretKey      string `json:"secretKey,omitempty"`
	SessionToken   string `json:"sessionToken,omitempty"`
	Region         string `json:"region,omitempty"`
}

type Run struct {
	ID         string     `json:"id"`
	Actor      string     `json:"actor,omitempty"`
	Trigger    string     `json:"trigger"`
	Generation int64      `json:"generation"`
	State      string     `json:"state"`
	BundlePath string     `json:"bundlePath,omitempty"`
	Checksum   string     `json:"checksum,omitempty"`
	Bytes      int64      `json:"bytes,omitempty"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	Error      string     `json:"error,omitempty"`
}

type Copy struct {
	ID            string     `json:"id"`
	RunID         string     `json:"runId"`
	DestinationID string     `json:"destinationId"`
	Object        string     `json:"object"`
	Checksum      string     `json:"checksum"`
	Bytes         int64      `json:"bytes"`
	State         string     `json:"state"`
	Verified      bool       `json:"verified"`
	CreatedAt     time.Time  `json:"createdAt"`
	FinishedAt    *time.Time `json:"finishedAt,omitempty"`
	Error         string     `json:"error,omitempty"`
}

type Schedule struct {
	ID              string     `json:"id"`
	Enabled         bool       `json:"enabled"`
	IntervalSeconds int64      `json:"intervalSeconds"`
	LastStartedAt   *time.Time `json:"lastStartedAt,omitempty"`
	NextDueAt       *time.Time `json:"nextDueAt,omitempty"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

type Verification struct {
	ID            string     `json:"id"`
	RunID         string     `json:"runId"`
	DestinationID string     `json:"destinationId"`
	State         string     `json:"state"`
	VerifiedAt    *time.Time `json:"verifiedAt,omitempty"`
	Error         string     `json:"error,omitempty"`
}

type DestinationHealth struct {
	DestinationID string `json:"destinationId"`
	State         string `json:"state"`
	Verified      int    `json:"verified"`
	LastError     string `json:"lastError,omitempty"`
}

type Readiness struct {
	Configured           bool                `json:"configured"`
	RecoveryKey          bool                `json:"recoveryKey"`
	LatestVerified       bool                `json:"latestVerified"`
	LatestGeneration     int64               `json:"latestGeneration"`
	DestinationCount     int                 `json:"destinationCount"`
	HealthyCopies        int                 `json:"healthyCopies"`
	LastVerification     *time.Time          `json:"lastVerification,omitempty"`
	DockerAppdataCovered bool                `json:"dockerAppdataCovered"`
	DestinationHealth    []DestinationHealth `json:"destinationHealth,omitempty"`
	Warnings             []string            `json:"warnings,omitempty"`
	CheckedAt            time.Time           `json:"checkedAt"`
}

func DefaultSchedule() Schedule {
	return Schedule{ID: "default", Enabled: true, IntervalSeconds: 24 * 60 * 60}
}

func (s Schedule) Validate() error {
	if strings.TrimSpace(s.ID) == "" {
		return errors.New("backup schedule id is required")
	}
	if s.IntervalSeconds < 60*60 || s.IntervalSeconds > 365*24*60*60 {
		return errors.New("backup schedule interval must be between one hour and one year")
	}
	return nil
}

func (d Destination) Validate() error {
	if strings.TrimSpace(d.ID) == "" || strings.TrimSpace(d.Name) == "" {
		return errors.New("backup destination id and name are required")
	}
	if strings.ContainsAny(d.ID+d.Name+d.Target, "\x00\r\n") {
		return errors.New("backup destination contains unsupported control characters")
	}
	minimum := DefaultRetention()
	if d.Retention.Generations < minimum.Generations || d.Retention.Daily < minimum.Daily || d.Retention.Monthly < minimum.Monthly {
		return fmt.Errorf("backup retention must keep at least %d generations, %d daily copies, and %d monthly copies", minimum.Generations, minimum.Daily, minimum.Monthly)
	}
	switch d.Type {
	case DestinationLocal:
		if !filepath.IsAbs(d.Target) || filepath.Clean(d.Target) != d.Target {
			return errors.New("local backup target must be a clean absolute path")
		}
	case DestinationSFTP, DestinationS3:
		parsed, err := url.Parse(d.Target)
		if err != nil || parsed.Scheme != string(d.Type) && !(d.Type == DestinationS3 && (parsed.Scheme == "http" || parsed.Scheme == "https")) || parsed.Host == "" || parsed.Path == "" {
			return fmt.Errorf("%s backup target must be a valid URL", d.Type)
		}
	default:
		return fmt.Errorf("unsupported backup destination type %q", d.Type)
	}
	return nil
}

func SHA256File(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	hash := sha256.New()
	bytes, err := file.Seek(0, 2)
	if err != nil {
		return "", 0, err
	}
	if _, err := file.Seek(0, 0); err != nil {
		return "", 0, err
	}
	if _, err := file.WriteTo(hash); err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), bytes, nil
}

func ObjectName(generation int64, created time.Time) string {
	return fmt.Sprintf("recovery/generation-%d-%s.mrb", generation, created.UTC().Format("20060102T150405Z"))
}

// RetainedRuns returns the verified runs that satisfy generation, daily, and
// monthly retention. Runs are expected to be newest-first or are sorted here.
func RetainedRuns(runs []Run, policy RetentionPolicy) []Run {
	if policy.Generations < 1 {
		policy.Generations = 1
	}
	if policy.Daily < 1 {
		policy.Daily = 1
	}
	if policy.Monthly < 1 {
		policy.Monthly = 1
	}
	values := append([]Run(nil), runs...)
	sort.SliceStable(values, func(i, j int) bool { return values[i].StartedAt.After(values[j].StartedAt) })
	result := make([]Run, 0)
	seenGeneration, seenDay, seenMonth := map[int64]bool{}, map[string]bool{}, map[string]bool{}
	add := func(run Run, day, month string) {
		result = append(result, run)
		seenGeneration[run.Generation] = true
		seenDay[day] = true
		seenMonth[month] = true
	}
	for _, run := range values {
		if run.State != "verified" {
			continue
		}
		day, month := run.StartedAt.UTC().Format("2006-01-02"), run.StartedAt.UTC().Format("2006-01")
		if !seenGeneration[run.Generation] && countGeneration(result) < policy.Generations {
			add(run, day, month)
			continue
		}
		if !seenDay[day] && len(seenDay) < policy.Daily {
			add(run, day, month)
			continue
		}
		if !seenMonth[month] && len(seenMonth) < policy.Monthly {
			add(run, day, month)
		}
	}
	return result
}

func countGeneration(runs []Run) int {
	seen := map[int64]bool{}
	for _, run := range runs {
		seen[run.Generation] = true
	}
	return len(seen)
}
