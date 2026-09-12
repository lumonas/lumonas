package store

import (
	"fmt"
	"strconv"
	"time"
)

func (s *Store) ensureAuditSchema() error {
	rows, err := s.db.Query(`PRAGMA table_info(audit_log)`)
	if err != nil {
		return err
	}
	columns := map[string]string{
		"correlation_id": "TEXT",
		"operation_id":   "TEXT",
		"plan_hash":      "TEXT",
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
		if _, err := s.db.Exec(fmt.Sprintf("ALTER TABLE audit_log ADD COLUMN %s %s", name, definition)); err != nil {
			return err
		}
	}
	_, err = s.db.Exec(`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(7, ?)`, time.Now().UTC().Format(timeFormat))
	return err
}

func metadataText(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, ok := values[key]
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return text
}

func metadataInt64(values map[string]any, key string) int64 {
	if values == nil {
		return 0
	}
	switch value := values[key].(type) {
	case int:
		return int64(value)
	case int64:
		return value
	case float64:
		return int64(value)
	case string:
		parsed, _ := strconv.ParseInt(value, 10, 64)
		return parsed
	default:
		return 0
	}
}

func fillAuditFields(entry *AuditEntry) {
	if entry.CorrelationID == "" {
		entry.CorrelationID = metadataText(entry.Metadata, "correlationId")
	}
	if entry.OperationID == "" {
		entry.OperationID = metadataText(entry.Metadata, "operationId")
		if entry.OperationID == "" {
			entry.OperationID = metadataText(entry.Metadata, "jobId")
		}
	}
	if entry.PlanHash == "" {
		entry.PlanHash = metadataText(entry.Metadata, "planHash")
	}
	if entry.Generation == 0 {
		entry.Generation = metadataInt64(entry.Metadata, "generation")
	}
}
