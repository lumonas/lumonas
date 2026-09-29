package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

type WorkloadRecoveryObjective struct {
	WorkloadID string    `json:"workloadId"`
	RPOHours   int       `json:"rpoHours"`
	RTOMinutes int       `json:"rtoMinutes"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

func (s *Store) WorkloadRecoveryObjectives() ([]WorkloadRecoveryObjective, error) {
	rows, err := s.db.Query(`SELECT key,value FROM meta WHERE key LIKE 'recovery_workload_objective:%' ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]WorkloadRecoveryObjective, 0)
	for rows.Next() {
		var key, raw string
		if err := rows.Scan(&key, &raw); err != nil {
			return nil, err
		}
		var value WorkloadRecoveryObjective
		if json.Unmarshal([]byte(raw), &value) == nil {
			value.WorkloadID = strings.TrimPrefix(key, "recovery_workload_objective:")
			values = append(values, value)
		}
	}
	return values, rows.Err()
}

func (s *Store) SetWorkloadRecoveryObjective(value WorkloadRecoveryObjective) error {
	value.WorkloadID = strings.TrimSpace(value.WorkloadID)
	if value.WorkloadID == "" || len(value.WorkloadID) > 256 || value.RPOHours < 1 || value.RPOHours > 8760 || value.RTOMinutes < 1 || value.RTOMinutes > 10080 {
		return fmt.Errorf("workload id is required; RPO must be 1-8760 hours and RTO 1-10080 minutes")
	}
	value.UpdatedAt = time.Now().UTC()
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, "recovery_workload_objective:"+value.WorkloadID, string(raw))
	return err
}

const restoreDrillSchema = `
CREATE TABLE IF NOT EXISTS restore_drills (
  id TEXT PRIMARY KEY,
  trigger_name TEXT NOT NULL,
  state TEXT NOT NULL,
  bundle_path TEXT,
  generation INTEGER NOT NULL DEFAULT 0,
  started_at TEXT NOT NULL,
  finished_at TEXT,
  verified INTEGER NOT NULL DEFAULT 0,
  database_valid INTEGER NOT NULL DEFAULT 0,
  compose_valid INTEGER NOT NULL DEFAULT 0,
  secrets_restored INTEGER NOT NULL DEFAULT 0,
  database_restored INTEGER NOT NULL DEFAULT 0,
  appdata_json TEXT NOT NULL DEFAULT '[]',
  shares_json TEXT NOT NULL DEFAULT '[]',
  services_json TEXT NOT NULL DEFAULT '[]',
  services_healthy INTEGER NOT NULL DEFAULT 0,
  applied_files INTEGER NOT NULL DEFAULT 0,
  warnings_json TEXT NOT NULL DEFAULT '[]',
  error TEXT
);
CREATE INDEX IF NOT EXISTS restore_drills_started_idx ON restore_drills(started_at DESC);
CREATE TABLE IF NOT EXISTS restore_drill_schedule (
  id INTEGER PRIMARY KEY CHECK(id=1), enabled INTEGER NOT NULL,
  interval_seconds INTEGER NOT NULL, rpo_hours INTEGER NOT NULL DEFAULT 24,
  rto_minutes INTEGER NOT NULL DEFAULT 60, last_started_at TEXT, next_due_at TEXT,
  updated_at TEXT NOT NULL
);`

func (s *Store) ensureRestoreDrillSchema() error {
	_, err := s.db.Exec(restoreDrillSchema)
	if err != nil {
		return err
	}
	for _, column := range []struct{ name, definition string }{{"database_restored", "INTEGER NOT NULL DEFAULT 0"}, {"appdata_json", "TEXT NOT NULL DEFAULT '[]'"}, {"shares_json", "TEXT NOT NULL DEFAULT '[]'"}, {"services_json", "TEXT NOT NULL DEFAULT '[]'"}, {"services_healthy", "INTEGER NOT NULL DEFAULT 0"}} {
		rows, queryErr := s.db.Query(`PRAGMA table_info(restore_drills)`)
		if queryErr != nil {
			return queryErr
		}
		found := false
		for rows.Next() {
			var cid int
			var name, kind string
			var notnull, pk int
			var defaultValue any
			if scanErr := rows.Scan(&cid, &name, &kind, &notnull, &defaultValue, &pk); scanErr != nil {
				rows.Close()
				return scanErr
			}
			if name == column.name {
				found = true
			}
		}
		rowsErr := rows.Err()
		rows.Close()
		if rowsErr != nil {
			return rowsErr
		}
		if !found {
			if _, err := s.db.Exec(`ALTER TABLE restore_drills ADD COLUMN ` + column.name + ` ` + column.definition); err != nil {
				return err
			}
		}
	}
	for _, column := range []struct{ name, definition string }{{"rpo_hours", "INTEGER NOT NULL DEFAULT 24"}, {"rto_minutes", "INTEGER NOT NULL DEFAULT 60"}} {
		rows, queryErr := s.db.Query(`PRAGMA table_info(restore_drill_schedule)`)
		if queryErr != nil {
			return queryErr
		}
		found := false
		for rows.Next() {
			var cid int
			var name, kind string
			var notnull, pk int
			var defaultValue any
			if scanErr := rows.Scan(&cid, &name, &kind, &notnull, &defaultValue, &pk); scanErr != nil {
				rows.Close()
				return scanErr
			}
			if name == column.name {
				found = true
			}
		}
		rowsErr := rows.Err()
		rows.Close()
		if rowsErr != nil {
			return rowsErr
		}
		if !found {
			if _, err := s.db.Exec(`ALTER TABLE restore_drill_schedule ADD COLUMN ` + column.name + ` ` + column.definition); err != nil {
				return err
			}
		}
	}
	_, err = s.db.Exec(`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(15, ?)`, time.Now().UTC().Format(timeFormat))
	return err
}

func (s *Store) SaveRestoreDrill(value model.RestoreDrill) error {
	if err := s.ensureRestoreDrillSchema(); err != nil {
		return err
	}
	warnings, err := json.Marshal(value.Warnings)
	if err != nil {
		return err
	}
	appdata, err := json.Marshal(value.AppdataRestored)
	if err != nil {
		return err
	}
	shares, err := json.Marshal(value.SharesRestored)
	if err != nil {
		return err
	}
	services, err := json.Marshal(value.ServicesRehearsed)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO restore_drills(id,trigger_name,state,bundle_path,generation,started_at,finished_at,verified,database_valid,compose_valid,secrets_restored,database_restored,appdata_json,shares_json,services_json,services_healthy,applied_files,warnings_json,error) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET state=excluded.state,bundle_path=excluded.bundle_path,generation=excluded.generation,finished_at=excluded.finished_at,verified=excluded.verified,database_valid=excluded.database_valid,compose_valid=excluded.compose_valid,secrets_restored=excluded.secrets_restored,database_restored=excluded.database_restored,appdata_json=excluded.appdata_json,shares_json=excluded.shares_json,services_json=excluded.services_json,services_healthy=excluded.services_healthy,applied_files=excluded.applied_files,warnings_json=excluded.warnings_json,error=excluded.error`, value.ID, value.Trigger, value.State, nullable(value.BundlePath), value.Generation, value.StartedAt.UTC().Format(timeFormat), timeValue(value.FinishedAt), value.Verified, value.DatabaseValid, value.ComposeValid, value.SecretsRestored, value.DatabaseRestored, string(appdata), string(shares), string(services), value.ServicesHealthy, value.AppliedFiles, string(warnings), nullable(value.Error))
	return err
}

func (s *Store) RestoreDrills(limit int) ([]model.RestoreDrill, error) {
	if err := s.ensureRestoreDrillSchema(); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 500 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT id,trigger_name,state,COALESCE(bundle_path,''),generation,started_at,COALESCE(finished_at,''),verified,database_valid,compose_valid,secrets_restored,database_restored,appdata_json,shares_json,services_json,services_healthy,applied_files,warnings_json,COALESCE(error,'') FROM restore_drills ORDER BY started_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]model.RestoreDrill, 0)
	for rows.Next() {
		var value model.RestoreDrill
		var started, finished, appdata, shares, services, warnings string
		if err := rows.Scan(&value.ID, &value.Trigger, &value.State, &value.BundlePath, &value.Generation, &started, &finished, &value.Verified, &value.DatabaseValid, &value.ComposeValid, &value.SecretsRestored, &value.DatabaseRestored, &appdata, &shares, &services, &value.ServicesHealthy, &value.AppliedFiles, &warnings, &value.Error); err != nil {
			return nil, err
		}
		if value.StartedAt, err = time.Parse(timeFormat, started); err != nil {
			return nil, err
		}
		if finished != "" {
			parsed, parseErr := time.Parse(timeFormat, finished)
			if parseErr != nil {
				return nil, parseErr
			}
			value.FinishedAt = &parsed
		}
		if err := json.Unmarshal([]byte(warnings), &value.Warnings); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(appdata), &value.AppdataRestored); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(shares), &value.SharesRestored); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(services), &value.ServicesRehearsed); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *Store) RestoreDrillSchedule() (model.RestoreDrillSchedule, error) {
	if err := s.ensureRestoreDrillSchema(); err != nil {
		return model.RestoreDrillSchedule{}, err
	}
	var value model.RestoreDrillSchedule
	var last, next, updated sql.NullString
	err := s.db.QueryRow(`SELECT enabled,interval_seconds,rpo_hours,rto_minutes,last_started_at,next_due_at,updated_at FROM restore_drill_schedule WHERE id=1`).Scan(&value.Enabled, &value.IntervalSeconds, &value.RPOHours, &value.RTOMinutes, &last, &next, &updated)
	if err == sql.ErrNoRows {
		value = model.RestoreDrillSchedule{Enabled: true, IntervalSeconds: 7 * 24 * 60 * 60, RPOHours: 24, RTOMinutes: 60, UpdatedAt: time.Now().UTC()}
		return s.SaveRestoreDrillSchedule(value)
	}
	if err != nil {
		return value, err
	}
	for raw, target := range map[*sql.NullString]**time.Time{&last: &value.LastStartedAt, &next: &value.NextDueAt} {
		if raw.Valid {
			parsed, parseErr := time.Parse(timeFormat, raw.String)
			if parseErr != nil {
				return value, parseErr
			}
			*target = &parsed
		}
	}
	value.UpdatedAt, err = time.Parse(timeFormat, updated.String)
	return value, err
}

func (s *Store) SaveRestoreDrillSchedule(value model.RestoreDrillSchedule) (model.RestoreDrillSchedule, error) {
	if value.IntervalSeconds < 3600 || value.IntervalSeconds > 365*24*60*60 {
		value.IntervalSeconds = 7 * 24 * 60 * 60
	}
	if value.RPOHours <= 0 {
		value.RPOHours = 24
	}
	if value.RTOMinutes <= 0 {
		value.RTOMinutes = 60
	}
	if err := s.ensureRestoreDrillSchema(); err != nil {
		return value, err
	}
	now := time.Now().UTC()
	value.UpdatedAt = now
	_, err := s.db.Exec(`INSERT INTO restore_drill_schedule(id,enabled,interval_seconds,rpo_hours,rto_minutes,last_started_at,next_due_at,updated_at) VALUES(1,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET enabled=excluded.enabled,interval_seconds=excluded.interval_seconds,rpo_hours=excluded.rpo_hours,rto_minutes=excluded.rto_minutes,last_started_at=excluded.last_started_at,next_due_at=excluded.next_due_at,updated_at=excluded.updated_at`, value.Enabled, value.IntervalSeconds, value.RPOHours, value.RTOMinutes, timeValue(value.LastStartedAt), timeValue(value.NextDueAt), now.Format(timeFormat))
	return value, err
}
