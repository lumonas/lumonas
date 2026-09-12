package store

import (
	"encoding/json"
	"time"

	"github.com/lumonas/lumonas/internal/storage"
)

const mountSchema = `
CREATE TABLE IF NOT EXISTS storage_mounts (
  mount_path TEXT PRIMARY KEY,
  config_json TEXT NOT NULL,
  updated_at TEXT NOT NULL
);`

func (s *Store) ensureMountSchema() error {
	_, err := s.db.Exec(mountSchema)
	return err
}

// MountEntries returns every persisted declarative mount.
func (s *Store) MountEntries() ([]storage.MountEntry, error) {
	if err := s.ensureMountSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT config_json FROM storage_mounts ORDER BY mount_path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]storage.MountEntry, 0)
	for rows.Next() {
		var encoded string
		var entry storage.MountEntry
		if err := rows.Scan(&encoded); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(encoded), &entry); err != nil {
			return nil, err
		}
		result = append(result, entry)
	}
	return result, rows.Err()
}

// SaveMountEntries atomically replaces the declarative mount state.
func (s *Store) SaveMountEntries(entries []storage.MountEntry) error {
	if err := s.ensureMountSchema(); err != nil {
		return err
	}
	normalized, err := storage.NormalizeMountEntries(entries)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM storage_mounts`); err != nil {
		_ = tx.Rollback()
		return err
	}
	now := time.Now().UTC().Format(timeFormat)
	for _, entry := range normalized {
		encoded, err := json.Marshal(entry)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		if _, err := tx.Exec(`INSERT INTO storage_mounts(mount_path,config_json,updated_at) VALUES(?,?,?) ON CONFLICT(mount_path) DO UPDATE SET config_json=excluded.config_json,updated_at=excluded.updated_at`, entry.MountPath, string(encoded), now); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// MountEntriesChanged reports whether the desired state differs from the
// persisted state, so unit regeneration can be skipped when unchanged.
func (s *Store) MountEntriesChanged(entries []storage.MountEntry) (bool, error) {
	existing, err := s.MountEntries()
	if err != nil {
		return true, err
	}
	normalized, err := storage.NormalizeMountEntries(entries)
	if err != nil {
		return true, err
	}
	if len(existing) != len(normalized) {
		return true, nil
	}
	existingEncoded := make([]string, 0, len(existing))
	for _, entry := range existing {
		encoded, err := json.Marshal(entry)
		if err != nil {
			return true, err
		}
		existingEncoded = append(existingEncoded, string(encoded))
	}
	for _, entry := range normalized {
		encoded, err := json.Marshal(entry)
		if err != nil {
			return true, err
		}
		found := false
		for _, candidate := range existingEncoded {
			if candidate == string(encoded) {
				found = true
				break
			}
		}
		if !found {
			return true, nil
		}
	}
	return false, nil
}
