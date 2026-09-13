package store

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

const systemMetricsSchema = `
CREATE TABLE IF NOT EXISTS system_metric_samples (
  captured_at TEXT PRIMARY KEY,
  data_json TEXT NOT NULL
);`

func (s *Store) ensureSystemMetricsSchema() error {
	if _, err := s.db.Exec(systemMetricsSchema); err != nil {
		return err
	}
	_, err := s.db.Exec(`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(9, ?)`, time.Now().UTC().Format(timeFormat))
	return err
}

// SaveSystemMetricSample stores one observation. The primary key deliberately
// uses the normalized timestamp so retries of one sample are idempotent.
func (s *Store) SaveSystemMetricSample(sample model.SystemMetricSample) error {
	if sample.CapturedAt.IsZero() {
		sample.CapturedAt = time.Now().UTC()
	}
	sample.CapturedAt = sample.CapturedAt.UTC()
	data, err := json.Marshal(sample.Metrics)
	if err != nil {
		return fmt.Errorf("encode system metrics: %w", err)
	}
	if err := s.ensureSystemMetricsSchema(); err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO system_metric_samples(captured_at,data_json) VALUES(?,?) ON CONFLICT(captured_at) DO UPDATE SET data_json=excluded.data_json`, sample.CapturedAt.Format(timeFormat), string(data))
	return err
}

// SystemMetricSamples returns newest observations first, matching the other
// operational history queries and keeping the API response bounded.
func (s *Store) SystemMetricSamples(since time.Time, limit int) ([]model.SystemMetricSample, error) {
	if err := s.ensureSystemMetricsSchema(); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 10080 {
		limit = 1440
	}
	rows, err := s.db.Query(`SELECT captured_at,data_json FROM system_metric_samples WHERE captured_at >= ? ORDER BY captured_at DESC LIMIT ?`, since.UTC().Format(timeFormat), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]model.SystemMetricSample, 0)
	for rows.Next() {
		var captured, data string
		if err := rows.Scan(&captured, &data); err != nil {
			return nil, err
		}
		var sample model.SystemMetricSample
		sample.CapturedAt, err = time.Parse(timeFormat, captured)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(data), &sample.Metrics); err != nil {
			return nil, fmt.Errorf("decode system metrics sample: %w", err)
		}
		result = append(result, sample)
	}
	return result, rows.Err()
}

func (s *Store) PruneSystemMetricSamples(before time.Time) error {
	if err := s.ensureSystemMetricsSchema(); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM system_metric_samples WHERE captured_at < ?`, before.UTC().Format(timeFormat))
	return err
}
