package store

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

const smartHistorySchema = `
CREATE TABLE IF NOT EXISTS smart_samples (
  disk_id TEXT NOT NULL,
  captured_at TEXT NOT NULL,
  data_json TEXT NOT NULL,
  PRIMARY KEY (disk_id, captured_at)
);
CREATE INDEX IF NOT EXISTS smart_samples_disk_time_idx ON smart_samples(disk_id, captured_at DESC);`

func (s *Store) ensureSMARTHistorySchema() error {
	_, err := s.db.Exec(smartHistorySchema)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(13, ?)`, time.Now().UTC().Format(timeFormat))
	return err
}

func (s *Store) SaveSMARTSample(sample model.SMARTSample) error {
	if sample.DiskID == "" {
		return fmt.Errorf("SMART disk id is required")
	}
	if sample.CapturedAt.IsZero() {
		sample.CapturedAt = time.Now().UTC()
	}
	sample.CapturedAt = sample.CapturedAt.UTC()
	data, err := json.Marshal(struct {
		Summary      model.SmartSummary `json:"summary"`
		TemperatureC *float64           `json:"temperatureC,omitempty"`
	}{sample.Summary, sample.TemperatureC})
	if err != nil {
		return err
	}
	if err := s.ensureSMARTHistorySchema(); err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO smart_samples(disk_id,captured_at,data_json) VALUES(?,?,?) ON CONFLICT(disk_id,captured_at) DO UPDATE SET data_json=excluded.data_json`, sample.DiskID, sample.CapturedAt.Format(timeFormat), string(data))
	return err
}

func (s *Store) SMARTSamples(diskID string, since time.Time, limit int) ([]model.SMARTSample, error) {
	if err := s.ensureSMARTHistorySchema(); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 10000 {
		limit = 1000
	}
	rows, err := s.db.Query(`SELECT disk_id,captured_at,data_json FROM smart_samples WHERE disk_id=? AND captured_at>=? ORDER BY captured_at DESC LIMIT ?`, diskID, since.UTC().Format(timeFormat), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]model.SMARTSample, 0)
	for rows.Next() {
		var sample model.SMARTSample
		var captured, data string
		if err := rows.Scan(&sample.DiskID, &captured, &data); err != nil {
			return nil, err
		}
		if sample.CapturedAt, err = time.Parse(timeFormat, captured); err != nil {
			return nil, err
		}
		var payload struct {
			Summary      model.SmartSummary `json:"summary"`
			TemperatureC *float64           `json:"temperatureC,omitempty"`
		}
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			return nil, fmt.Errorf("decode SMART sample: %w", err)
		}
		sample.Summary, sample.TemperatureC = payload.Summary, payload.TemperatureC
		result = append(result, sample)
	}
	return result, rows.Err()
}

func (s *Store) PruneSMARTSamples(before time.Time) error {
	if err := s.ensureSMARTHistorySchema(); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM smart_samples WHERE captured_at < ?`, before.UTC().Format(timeFormat))
	return err
}
