package store

import "time"

const eventSchemaVersion = 6

func (s *Store) ensureEventSchema() error {
	rows, err := s.db.Query(`PRAGMA table_info(events)`)
	if err != nil {
		return err
	}
	columns := map[string]string{
		"schema_version": "INTEGER NOT NULL DEFAULT 1",
		"correlation_id": "TEXT",
		"operation_id":   "TEXT",
		"plan_hash":      "TEXT",
		"actor":          "TEXT",
		"generation":     "INTEGER NOT NULL DEFAULT 0",
	}
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, dataType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		delete(columns, name)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for name, definition := range columns {
		if _, err := s.db.Exec(`ALTER TABLE events ADD COLUMN ` + name + ` ` + definition); err != nil {
			return err
		}
	}
	_, err = s.db.Exec(`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(?, ?)`, eventSchemaVersion, time.Now().UTC().Format(timeFormat))
	return err
}
