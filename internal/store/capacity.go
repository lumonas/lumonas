package store

import (
	"fmt"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

type CapacityThreshold struct {
	ResourceID string    `json:"resourceId"`
	Percent    int       `json:"thresholdPercent"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

func (s *Store) CapacityThresholds() ([]CapacityThreshold, error) {
	rows, err := s.db.Query(`SELECT key,value FROM meta WHERE key GLOB 'capacity_threshold:*' ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]CapacityThreshold, 0)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		var percent int
		if _, err := fmt.Sscanf(value, "%d", &percent); err != nil || percent < 50 || percent > 99 {
			continue
		}
		resource := strings.TrimPrefix(key, "capacity_threshold:")
		var updated string
		_ = s.db.QueryRow(`SELECT value FROM meta WHERE key=?`, "capacity_threshold_updated:"+resource).Scan(&updated)
		updatedAt, _ := time.Parse(time.RFC3339Nano, updated)
		result = append(result, CapacityThreshold{ResourceID: resource, Percent: percent, UpdatedAt: updatedAt})
	}
	return result, rows.Err()
}

func (s *Store) SetCapacityThreshold(resourceID string, percent int) error {
	if strings.TrimSpace(resourceID) == "" || len(resourceID) > 512 || percent < 50 || percent > 99 {
		return fmt.Errorf("resource id is required and threshold must be between 50 and 99")
	}
	_, err := s.db.Exec(`INSERT INTO meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, "capacity_threshold:"+resourceID, fmt.Sprint(percent))
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, "capacity_threshold_updated:"+resourceID, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

const capacitySchema = `
CREATE TABLE IF NOT EXISTS capacity_snapshots (
  resource_id TEXT NOT NULL,
  captured_at TEXT NOT NULL,
  captured_day TEXT NOT NULL,
  total_bytes INTEGER NOT NULL,
  used_bytes INTEGER NOT NULL,
  PRIMARY KEY(resource_id, captured_day)
);`

func (s *Store) ensureCapacitySchema() error {
	_, err := s.db.Exec(capacitySchema)
	return err
}

func (s *Store) SaveCapacitySnapshot(snapshot model.CapacitySnapshot) error {
	if snapshot.ResourceID == "" || snapshot.TotalBytes == 0 || snapshot.UsedBytes > snapshot.TotalBytes {
		return fmt.Errorf("invalid capacity snapshot")
	}
	if snapshot.CapturedAt.IsZero() {
		snapshot.CapturedAt = time.Now().UTC()
	}
	if err := s.ensureCapacitySchema(); err != nil {
		return err
	}
	_, err := s.db.Exec(`INSERT INTO capacity_snapshots(resource_id,captured_at,captured_day,total_bytes,used_bytes) VALUES(?,?,?,?,?) ON CONFLICT(resource_id,captured_day) DO UPDATE SET captured_at=excluded.captured_at,total_bytes=excluded.total_bytes,used_bytes=excluded.used_bytes`, snapshot.ResourceID, snapshot.CapturedAt.UTC().Format(timeFormat), snapshot.CapturedAt.UTC().Format("2006-01-02"), snapshot.TotalBytes, snapshot.UsedBytes)
	return err
}

func (s *Store) CapacitySnapshots(resourceID string, since time.Time, limit int) ([]model.CapacitySnapshot, error) {
	if err := s.ensureCapacitySchema(); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 366 {
		limit = 180
	}
	query := `SELECT resource_id,captured_at,total_bytes,used_bytes FROM capacity_snapshots WHERE captured_at >= ?`
	args := []any{since.UTC().Format(timeFormat)}
	if resourceID != "" {
		query += ` AND resource_id = ?`
		args = append(args, resourceID)
	}
	query += ` ORDER BY captured_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]model.CapacitySnapshot, 0)
	for rows.Next() {
		var snapshot model.CapacitySnapshot
		var captured string
		if err := rows.Scan(&snapshot.ResourceID, &captured, &snapshot.TotalBytes, &snapshot.UsedBytes); err != nil {
			return nil, err
		}
		parsed, err := time.Parse(timeFormat, captured)
		if err != nil {
			return nil, err
		}
		snapshot.CapturedAt = parsed
		result = append(result, snapshot)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Store) CapacityResources(since time.Time) ([]string, error) {
	if err := s.ensureCapacitySchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT DISTINCT resource_id FROM capacity_snapshots WHERE captured_at >= ? ORDER BY resource_id`, since.UTC().Format(timeFormat))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var resourceID string
		if err := rows.Scan(&resourceID); err != nil {
			return nil, err
		}
		result = append(result, resourceID)
	}
	return result, rows.Err()
}

func (s *Store) PruneCapacitySnapshots(before time.Time) error {
	if err := s.ensureCapacitySchema(); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM capacity_snapshots WHERE captured_at < ?`, before.UTC().Format(timeFormat))
	return err
}
