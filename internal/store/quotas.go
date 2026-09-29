package store

import (
	"encoding/json"
	"time"

	"github.com/lumonas/lumonas/internal/quotas"
)

// ensureQuotaSchema creates the quota tables. It is called from Open so the
// packaged lumonas-migrate upgrade path provisions them, and remains safe to
// call from the accessors below.
func (s *Store) ensureQuotaSchema() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS storage_quotas(id TEXT PRIMARY KEY,target_type TEXT NOT NULL,target_id TEXT NOT NULL,limit_bytes INTEGER NOT NULL,warning_percent INTEGER NOT NULL,UNIQUE(target_type,target_id))`); err != nil {
		return err
	}
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS storage_quota_status(id TEXT PRIMARY KEY,status_json TEXT NOT NULL,updated_at TEXT NOT NULL)`)
	return err
}

func (s *Store) Quotas() ([]quotas.Policy, error) {
	if err := s.ensureQuotaSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT id,target_type,target_id,limit_bytes,warning_percent FROM storage_quotas ORDER BY target_type,target_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]quotas.Policy, 0)
	for rows.Next() {
		var p quotas.Policy
		if err := rows.Scan(&p.ID, &p.TargetType, &p.TargetID, &p.LimitBytes, &p.WarningPercent); err != nil {
			return nil, err
		}
		values = append(values, p)
	}
	return values, rows.Err()
}

func (s *Store) ReplaceQuotas(values []quotas.Policy) error {
	if err := s.ensureQuotaSchema(); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM storage_quotas`); err != nil {
		return err
	}
	for _, p := range values {
		if err := p.Validate(); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO storage_quotas(id,target_type,target_id,limit_bytes,warning_percent) VALUES(?,?,?,?,?)`, p.ID, p.TargetType, p.TargetID, p.LimitBytes, p.WarningPercent); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SaveQuotaStatus(status quotas.Status) error {
	if err := s.ensureQuotaSchema(); err != nil {
		return err
	}
	raw, err := json.Marshal(status)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO storage_quota_status(id,status_json,updated_at) VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET status_json=excluded.status_json,updated_at=excluded.updated_at`, status.Policy.ID, string(raw), time.Now().UTC().Format(timeFormat))
	return err
}
