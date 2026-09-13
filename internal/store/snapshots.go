package store

import (
	"database/sql"
	"time"
)

const snapshotsSchema = `
CREATE TABLE IF NOT EXISTS storage_snapshots (
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL,
  source TEXT NOT NULL,
  name TEXT NOT NULL,
  label TEXT,
  origin TEXT NOT NULL DEFAULT 'manual',
  created_at TEXT NOT NULL,
  UNIQUE(kind, source, name)
);
CREATE INDEX IF NOT EXISTS storage_snapshots_source_idx ON storage_snapshots(source, created_at);`

func (s *Store) ensureSnapshotsSchema() error {
	if _, err := s.db.Exec(snapshotsSchema); err != nil {
		return err
	}
	rows, err := s.db.Query(`PRAGMA table_info(storage_snapshots)`)
	if err != nil {
		return err
	}
	hasOrigin := false
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, dataType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		if name == "origin" {
			hasOrigin = true
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if !hasOrigin {
		if _, err := s.db.Exec(`ALTER TABLE storage_snapshots ADD COLUMN origin TEXT NOT NULL DEFAULT 'manual'`); err != nil {
			return err
		}
	}
	_, err = s.db.Exec(`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(10, ?)`, time.Now().UTC().Format(timeFormat))
	return err
}

// StorageSnapshotRecord is one persisted point-in-time snapshot.
type StorageSnapshotRecord struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Source    string    `json:"source"`
	Name      string    `json:"name"`
	Label     string    `json:"label,omitempty"`
	Origin    string    `json:"origin,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// SaveStorageSnapshot persists a snapshot row. Re-saving an identical
// (kind, source, name) triple is a no-op.
func (s *Store) SaveStorageSnapshot(record StorageSnapshotRecord) (StorageSnapshotRecord, error) {
	if err := s.ensureSnapshotsSchema(); err != nil {
		return StorageSnapshotRecord{}, err
	}
	if record.ID == "" {
		record.ID = newStoreID("snap")
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	if record.Origin == "" {
		record.Origin = "manual"
	}
	_, err := s.db.Exec(`INSERT INTO storage_snapshots(id,kind,source,name,label,origin,created_at) VALUES(?,?,?,?,?,?,?)
ON CONFLICT(kind,source,name) DO NOTHING`, record.ID, record.Kind, record.Source, record.Name, nullable(record.Label), record.Origin, record.CreatedAt.UTC().Format(timeFormat))
	if err != nil {
		return StorageSnapshotRecord{}, err
	}
	existing, found, err := s.StorageSnapshotByTriple(record.Kind, record.Source, record.Name)
	if err != nil {
		return StorageSnapshotRecord{}, err
	}
	if found {
		return existing, nil
	}
	return record, nil
}

// StorageSnapshots lists persisted snapshots, newest first, optionally scoped
// to one source path or dataset.
func (s *Store) StorageSnapshots(source string, limit int) ([]StorageSnapshotRecord, error) {
	if err := s.ensureSnapshotsSchema(); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := `SELECT id,kind,source,name,COALESCE(label,''),origin,created_at FROM storage_snapshots`
	args := []any{}
	if source != "" {
		query += ` WHERE source=?`
		args = append(args, source)
	}
	query += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)
	return s.scanStorageSnapshots(query, args...)
}

// StorageSnapshot returns one persisted snapshot by id.
func (s *Store) StorageSnapshot(id string) (StorageSnapshotRecord, bool, error) {
	records, err := s.scanStorageSnapshots(`SELECT id,kind,source,name,COALESCE(label,''),origin,created_at FROM storage_snapshots WHERE id=?`, id)
	if err != nil {
		return StorageSnapshotRecord{}, false, err
	}
	if len(records) == 0 {
		return StorageSnapshotRecord{}, false, nil
	}
	return records[0], true, nil
}

// StorageSnapshotByTriple resolves a snapshot by its natural key.
func (s *Store) StorageSnapshotByTriple(kind, source, name string) (StorageSnapshotRecord, bool, error) {
	records, err := s.scanStorageSnapshots(`SELECT id,kind,source,name,COALESCE(label,''),origin,created_at FROM storage_snapshots WHERE kind=? AND source=? AND name=?`, kind, source, name)
	if err != nil {
		return StorageSnapshotRecord{}, false, err
	}
	if len(records) == 0 {
		return StorageSnapshotRecord{}, false, nil
	}
	return records[0], true, nil
}

// DeleteStorageSnapshot removes a persisted snapshot row once the privileged
// deletion has succeeded.
func (s *Store) DeleteStorageSnapshot(id string) error {
	if err := s.ensureSnapshotsSchema(); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM storage_snapshots WHERE id=?`, id)
	return err
}

func (s *Store) scanStorageSnapshots(query string, args ...any) ([]StorageSnapshotRecord, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]StorageSnapshotRecord, 0)
	for rows.Next() {
		var record StorageSnapshotRecord
		var created string
		if err := rows.Scan(&record.ID, &record.Kind, &record.Source, &record.Name, &record.Label, &record.Origin, &created); err != nil {
			return nil, err
		}
		record.CreatedAt, _ = parseTime(created)
		result = append(result, record)
	}
	return result, rows.Err()
}
