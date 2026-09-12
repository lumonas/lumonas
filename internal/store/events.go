package store

import "time"

const eventSchemaVersion = 4

func (s *Store) ensureEventSchema() error {
	rows, err := s.db.Query(`PRAGMA table_info(events)`)
	if err != nil {
		return err
	}
	hasSchemaVersion := false
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, dataType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		if name == "schema_version" {
			hasSchemaVersion = true
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if !hasSchemaVersion {
		if _, err := s.db.Exec(`ALTER TABLE events ADD COLUMN schema_version INTEGER NOT NULL DEFAULT 1`); err != nil {
			return err
		}
	}
	_, err = s.db.Exec(`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(?, ?)`, eventSchemaVersion, time.Now().UTC().Format(timeFormat))
	return err
}
