package store

import "time"

const defaultJobRetention = 1000

func (s *Store) ensureJobSchema() error {
	rows, err := s.db.Query(`PRAGMA table_info(jobs)`)
	if err != nil {
		return err
	}
	hasCorrelationID := false
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, dataType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		if name == "correlation_id" {
			hasCorrelationID = true
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if !hasCorrelationID {
		if _, err := s.db.Exec(`ALTER TABLE jobs ADD COLUMN correlation_id TEXT`); err != nil {
			return err
		}
	}
	_, err = s.db.Exec(`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(5, ?)`, time.Now().UTC().Format(timeFormat))
	return err
}

// PruneJobs keeps active work visible while bounding terminal job history.
// Queued and running jobs are never removed by retention.
func (s *Store) PruneJobs(keep int) error {
	if keep < 100 {
		keep = 100
	}
	_, err := s.db.Exec(`DELETE FROM jobs
WHERE state IN ('completed','failed','canceled')
  AND id NOT IN (
    SELECT id FROM jobs
    WHERE state IN ('completed','failed','canceled')
    ORDER BY created_at DESC
    LIMIT ?
  )`, keep)
	return err
}
