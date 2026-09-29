package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lumonas/lumonas/internal/backup"
)

const backupSchema = `
CREATE TABLE IF NOT EXISTS backup_destinations (
  id TEXT PRIMARY KEY, name TEXT UNIQUE NOT NULL, type TEXT NOT NULL,
  target TEXT NOT NULL, enabled INTEGER NOT NULL, retention_json TEXT NOT NULL,
  credentials_ciphertext BLOB, created_at TEXT NOT NULL, updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS backup_runs (
  id TEXT PRIMARY KEY, actor TEXT, trigger_name TEXT NOT NULL, generation INTEGER NOT NULL,
  state TEXT NOT NULL, bundle_path TEXT, checksum TEXT, bytes INTEGER NOT NULL DEFAULT 0,
  started_at TEXT NOT NULL, finished_at TEXT, error TEXT
);
CREATE TABLE IF NOT EXISTS backup_copies (
  id TEXT PRIMARY KEY, run_id TEXT NOT NULL REFERENCES backup_runs(id) ON DELETE CASCADE,
  destination_id TEXT NOT NULL REFERENCES backup_destinations(id) ON DELETE CASCADE,
  object_name TEXT NOT NULL, checksum TEXT NOT NULL, bytes INTEGER NOT NULL DEFAULT 0,
  state TEXT NOT NULL, verified INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL,
  finished_at TEXT, error TEXT
);
CREATE INDEX IF NOT EXISTS backup_copies_destination_idx ON backup_copies(destination_id, created_at);
CREATE TABLE IF NOT EXISTS backup_schedule (
  id TEXT PRIMARY KEY, enabled INTEGER NOT NULL, interval_seconds INTEGER NOT NULL,
  on_usb_attach INTEGER NOT NULL DEFAULT 0, last_started_at TEXT, next_due_at TEXT, updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS backup_verifications (
  id TEXT PRIMARY KEY, run_id TEXT NOT NULL REFERENCES backup_runs(id) ON DELETE CASCADE,
  destination_id TEXT NOT NULL REFERENCES backup_destinations(id) ON DELETE CASCADE,
  state TEXT NOT NULL, verified_at TEXT, error TEXT
);
CREATE INDEX IF NOT EXISTS backup_verifications_destination_idx ON backup_verifications(destination_id, verified_at);`

func (s *Store) ensureBackupSchema() error {
	if _, err := s.db.Exec(backupSchema); err != nil {
		return err
	}
	rows, err := s.db.Query(`PRAGMA table_info(backup_runs)`)
	if err != nil {
		return err
	}
	hasActor := false
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, dataType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		if name == "actor" {
			hasActor = true
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if !hasActor {
		if _, err := s.db.Exec(`ALTER TABLE backup_runs ADD COLUMN actor TEXT`); err != nil {
			return err
		}
	}
	rows, err = s.db.Query(`PRAGMA table_info(backup_schedule)`)
	if err != nil {
		return err
	}
	hasUSBTrigger := false
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, dataType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		if name == "on_usb_attach" {
			hasUSBTrigger = true
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if !hasUSBTrigger {
		if _, err := s.db.Exec(`ALTER TABLE backup_schedule ADD COLUMN on_usb_attach INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	_, err = s.db.Exec(`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(3, ?), (8, ?), (16, ?)`, time.Now().UTC().Format(timeFormat), time.Now().UTC().Format(timeFormat), time.Now().UTC().Format(timeFormat))
	return err
}

func (s *Store) SaveBackupDestination(value backup.Destination, credentials backup.Credentials, key []byte) (backup.Destination, error) {
	if err := value.Validate(); err != nil {
		return backup.Destination{}, err
	}
	if value.Type == backup.DestinationRclone {
		if err := backup.ValidateRcloneConfiguration(value.Target, credentials.RcloneConfig); err != nil {
			return backup.Destination{}, err
		}
	}
	if err := s.ensureBackupSchema(); err != nil {
		return backup.Destination{}, err
	}
	ciphertext, err := backup.EncryptCredentials(credentials, key)
	if err != nil {
		return backup.Destination{}, err
	}
	encoded, err := json.Marshal(value.Retention)
	if err != nil {
		return backup.Destination{}, err
	}
	now := time.Now().UTC()
	_, err = s.db.Exec(`INSERT INTO backup_destinations(id,name,type,target,enabled,retention_json,credentials_ciphertext,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,type=excluded.type,target=excluded.target,enabled=excluded.enabled,retention_json=excluded.retention_json,credentials_ciphertext=excluded.credentials_ciphertext,updated_at=excluded.updated_at`, value.ID, value.Name, value.Type, value.Target, value.Enabled, string(encoded), ciphertext, now.Format(timeFormat), now.Format(timeFormat))
	if err != nil {
		return backup.Destination{}, err
	}
	value.CredentialsConfigured = len(ciphertext) > 0
	value.CreatedAt, value.UpdatedAt = now, now
	return value, nil
}

func (s *Store) ListBackupDestinations() ([]backup.Destination, error) {
	if err := s.ensureBackupSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT id,name,type,target,enabled,retention_json,credentials_ciphertext,created_at,updated_at FROM backup_destinations ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]backup.Destination, 0)
	for rows.Next() {
		value, err := scanBackupDestination(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *Store) BackupDestination(id string, key []byte) (backup.Destination, backup.Credentials, error) {
	if err := s.ensureBackupSchema(); err != nil {
		return backup.Destination{}, backup.Credentials{}, err
	}
	row := s.db.QueryRow(`SELECT id,name,type,target,enabled,retention_json,credentials_ciphertext,created_at,updated_at FROM backup_destinations WHERE id=?`, id)
	value, ciphertext, err := scanBackupDestinationWithCiphertext(row)
	if err != nil {
		return backup.Destination{}, backup.Credentials{}, err
	}
	if len(ciphertext) == 0 {
		return value, backup.Credentials{}, nil
	}
	credentials, err := backup.DecryptCredentials(ciphertext, key)
	return value, credentials, err
}

func (s *Store) DeleteBackupDestination(id string) error {
	if err := s.ensureBackupSchema(); err != nil {
		return err
	}
	result, err := s.db.Exec(`DELETE FROM backup_destinations WHERE id=?`, id)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) SaveBackupSchedule(value backup.Schedule) (backup.Schedule, error) {
	if err := value.Validate(); err != nil {
		return backup.Schedule{}, err
	}
	if err := s.ensureBackupSchema(); err != nil {
		return backup.Schedule{}, err
	}
	now := time.Now().UTC()
	_, err := s.db.Exec(`INSERT INTO backup_schedule(id,enabled,interval_seconds,on_usb_attach,last_started_at,next_due_at,updated_at) VALUES(?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET enabled=excluded.enabled,interval_seconds=excluded.interval_seconds,on_usb_attach=excluded.on_usb_attach,last_started_at=excluded.last_started_at,next_due_at=excluded.next_due_at,updated_at=excluded.updated_at`, value.ID, value.Enabled, value.IntervalSeconds, value.OnUSBAttach, timeValue(value.LastStartedAt), timeValue(value.NextDueAt), now.Format(timeFormat))
	if err != nil {
		return backup.Schedule{}, err
	}
	value.UpdatedAt = now
	return value, nil
}

// ApplyBackupPolicyTemplate updates the backup cadence and destination
// retention counts atomically. Destination-specific provider-lock settings
// are intentionally preserved by changing only generation/day/month counts.
func (s *Store) ApplyBackupPolicyTemplate(templateID string) (backup.Schedule, error) {
	template, ok := backup.PolicyTemplateByID(templateID)
	if !ok {
		return backup.Schedule{}, fmt.Errorf("unknown backup policy template %q", templateID)
	}
	if err := s.ensureBackupSchema(); err != nil {
		return backup.Schedule{}, err
	}
	schedule, err := s.BackupSchedule()
	if err != nil {
		return backup.Schedule{}, err
	}
	schedule.IntervalSeconds = template.IntervalSeconds
	if schedule.ID == "" {
		schedule.ID = "default"
	}
	if err := schedule.Validate(); err != nil {
		return backup.Schedule{}, err
	}
	rows, err := s.db.Query(`SELECT id,retention_json FROM backup_destinations`)
	if err != nil {
		return backup.Schedule{}, err
	}
	type retentionUpdate struct {
		id    string
		value backup.RetentionPolicy
	}
	updates := make([]retentionUpdate, 0)
	for rows.Next() {
		var id, encoded string
		if err := rows.Scan(&id, &encoded); err != nil {
			rows.Close()
			return backup.Schedule{}, err
		}
		var retention backup.RetentionPolicy
		if err := json.Unmarshal([]byte(encoded), &retention); err != nil {
			rows.Close()
			return backup.Schedule{}, fmt.Errorf("decode retention for destination %s: %w", id, err)
		}
		retention.Generations, retention.Daily, retention.Monthly = template.Generations, template.Daily, template.Monthly
		updates = append(updates, retentionUpdate{id: id, value: retention})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return backup.Schedule{}, err
	}
	if err := rows.Close(); err != nil {
		return backup.Schedule{}, err
	}
	now := time.Now().UTC()
	schedule.UpdatedAt = now
	nextDue := now.Add(time.Duration(schedule.IntervalSeconds) * time.Second)
	schedule.NextDueAt = &nextDue
	tx, err := s.db.Begin()
	if err != nil {
		return backup.Schedule{}, err
	}
	defer func() { _ = tx.Rollback() }()
	for _, update := range updates {
		encoded, err := json.Marshal(update.value)
		if err != nil {
			return backup.Schedule{}, err
		}
		if _, err := tx.Exec(`UPDATE backup_destinations SET retention_json=?,updated_at=? WHERE id=?`, string(encoded), now.Format(timeFormat), update.id); err != nil {
			return backup.Schedule{}, err
		}
	}
	if _, err := tx.Exec(`INSERT INTO backup_schedule(id,enabled,interval_seconds,on_usb_attach,last_started_at,next_due_at,updated_at) VALUES(?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET enabled=excluded.enabled,interval_seconds=excluded.interval_seconds,on_usb_attach=excluded.on_usb_attach,last_started_at=excluded.last_started_at,next_due_at=excluded.next_due_at,updated_at=excluded.updated_at`, schedule.ID, schedule.Enabled, schedule.IntervalSeconds, schedule.OnUSBAttach, timeValue(schedule.LastStartedAt), timeValue(schedule.NextDueAt), now.Format(timeFormat)); err != nil {
		return backup.Schedule{}, err
	}
	if err := tx.Commit(); err != nil {
		return backup.Schedule{}, err
	}
	return schedule, nil
}

func (s *Store) BackupSchedule() (backup.Schedule, error) {
	if err := s.ensureBackupSchema(); err != nil {
		return backup.Schedule{}, err
	}
	row := s.db.QueryRow(`SELECT id,enabled,interval_seconds,on_usb_attach,last_started_at,next_due_at,updated_at FROM backup_schedule ORDER BY id LIMIT 1`)
	var value backup.Schedule
	var lastStarted, nextDue sql.NullString
	var updated string
	if err := row.Scan(&value.ID, &value.Enabled, &value.IntervalSeconds, &value.OnUSBAttach, &lastStarted, &nextDue, &updated); err != nil {
		if err == sql.ErrNoRows {
			return backup.DefaultSchedule(), nil
		}
		return backup.Schedule{}, err
	}
	var err error
	if lastStarted.Valid {
		parsed, parseErr := time.Parse(timeFormat, lastStarted.String)
		if parseErr != nil {
			return backup.Schedule{}, parseErr
		}
		value.LastStartedAt = &parsed
	}
	if nextDue.Valid {
		parsed, parseErr := time.Parse(timeFormat, nextDue.String)
		if parseErr != nil {
			return backup.Schedule{}, parseErr
		}
		value.NextDueAt = &parsed
	}
	value.UpdatedAt, err = time.Parse(timeFormat, updated)
	if err != nil {
		return backup.Schedule{}, err
	}
	return value, nil
}

func (s *Store) SaveBackupRun(value backup.Run) error {
	if err := s.ensureBackupSchema(); err != nil {
		return err
	}
	_, err := s.db.Exec(`INSERT INTO backup_runs(id,actor,trigger_name,generation,state,bundle_path,checksum,bytes,started_at,finished_at,error) VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET actor=excluded.actor,state=excluded.state,bundle_path=excluded.bundle_path,checksum=excluded.checksum,bytes=excluded.bytes,finished_at=excluded.finished_at,error=excluded.error`, value.ID, nullable(value.Actor), value.Trigger, value.Generation, value.State, nullable(value.BundlePath), nullable(value.Checksum), value.Bytes, value.StartedAt.Format(timeFormat), timeValue(value.FinishedAt), nullable(value.Error))
	return err
}

// FailInterruptedBackupRuns closes backup work that cannot safely resume after
// the daemon process disappeared. A new process must never report an old
// upload as active or silently continue with an incomplete bundle.
func (s *Store) FailInterruptedBackupRuns(reason string) (int, error) {
	if reason == "" {
		reason = "daemon restarted before backup completed"
	}
	if err := s.ensureBackupSchema(); err != nil {
		return 0, err
	}
	now := time.Now().UTC().Format(timeFormat)
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`UPDATE backup_copies SET state='failed',verified=0,finished_at=?,error=? WHERE state='running' AND run_id IN (SELECT id FROM backup_runs WHERE state IN ('queued','running'))`, now, reason); err != nil {
		return 0, err
	}
	result, err := tx.Exec(`UPDATE backup_runs SET state='failed',finished_at=?,error=? WHERE state IN ('queued','running')`, now, reason)
	if err != nil {
		return 0, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return int(count), nil
}

func (s *Store) SaveBackupCopy(value backup.Copy) error {
	if err := s.ensureBackupSchema(); err != nil {
		return err
	}
	_, err := s.db.Exec(`INSERT INTO backup_copies(id,run_id,destination_id,object_name,checksum,bytes,state,verified,created_at,finished_at,error) VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET state=excluded.state,verified=excluded.verified,finished_at=excluded.finished_at,error=excluded.error`, value.ID, value.RunID, value.DestinationID, value.Object, value.Checksum, value.Bytes, value.State, value.Verified, value.CreatedAt.Format(timeFormat), timeValue(value.FinishedAt), nullable(value.Error))
	return err
}

func (s *Store) BackupRuns(limit int) ([]backup.Run, error) {
	if err := s.ensureBackupSchema(); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 500 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT id,COALESCE(actor,''),trigger_name,generation,state,COALESCE(bundle_path,''),COALESCE(checksum,''),bytes,started_at,finished_at,COALESCE(error,'') FROM backup_runs ORDER BY started_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]backup.Run, 0)
	for rows.Next() {
		var value backup.Run
		var started string
		var finished, bundlePath, checksum, runError sql.NullString
		if err := rows.Scan(&value.ID, &value.Actor, &value.Trigger, &value.Generation, &value.State, &bundlePath, &checksum, &value.Bytes, &started, &finished, &runError); err != nil {
			return nil, err
		}
		value.BundlePath, value.Checksum, value.Error = bundlePath.String, checksum.String, runError.String
		value.StartedAt, err = time.Parse(timeFormat, started)
		if err != nil {
			return nil, err
		}
		if finished.Valid {
			parsed, parseErr := time.Parse(timeFormat, finished.String)
			if parseErr != nil {
				return nil, parseErr
			}
			value.FinishedAt = &parsed
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *Store) BackupCopies(runID string) ([]backup.Copy, error) {
	if err := s.ensureBackupSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT id,run_id,destination_id,object_name,checksum,bytes,state,verified,created_at,finished_at,COALESCE(error,'') FROM backup_copies WHERE run_id=? ORDER BY created_at`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]backup.Copy, 0)
	for rows.Next() {
		var value backup.Copy
		var created string
		var finished, copyError sql.NullString
		if err := rows.Scan(&value.ID, &value.RunID, &value.DestinationID, &value.Object, &value.Checksum, &value.Bytes, &value.State, &value.Verified, &created, &finished, &copyError); err != nil {
			return nil, err
		}
		value.Error = copyError.String
		value.CreatedAt, err = time.Parse(timeFormat, created)
		if err != nil {
			return nil, err
		}
		if finished.Valid {
			parsed, parseErr := time.Parse(timeFormat, finished.String)
			if parseErr != nil {
				return nil, parseErr
			}
			value.FinishedAt = &parsed
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *Store) BackupCopiesForDestination(destinationID string) ([]backup.Copy, error) {
	if err := s.ensureBackupSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT id,run_id,destination_id,object_name,checksum,bytes,state,verified,created_at,finished_at,COALESCE(error,'') FROM backup_copies WHERE destination_id=? ORDER BY created_at DESC`, destinationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]backup.Copy, 0)
	for rows.Next() {
		var value backup.Copy
		var created string
		var finished, copyError sql.NullString
		if err := rows.Scan(&value.ID, &value.RunID, &value.DestinationID, &value.Object, &value.Checksum, &value.Bytes, &value.State, &value.Verified, &created, &finished, &copyError); err != nil {
			return nil, err
		}
		value.Error = copyError.String
		value.CreatedAt, err = time.Parse(timeFormat, created)
		if err != nil {
			return nil, err
		}
		if finished.Valid {
			parsed, parseErr := time.Parse(timeFormat, finished.String)
			if parseErr != nil {
				return nil, parseErr
			}
			value.FinishedAt = &parsed
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *Store) DeleteBackupCopy(id string) error {
	if err := s.ensureBackupSchema(); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM backup_copies WHERE id=?`, id)
	return err
}

func (s *Store) SaveBackupVerification(value backup.Verification) error {
	if value.ID == "" || value.RunID == "" || value.DestinationID == "" || value.State == "" {
		return errors.New("backup verification identity and state are required")
	}
	if err := s.ensureBackupSchema(); err != nil {
		return err
	}
	_, err := s.db.Exec(`INSERT INTO backup_verifications(id,run_id,destination_id,state,verified_at,error) VALUES(?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET state=excluded.state,verified_at=excluded.verified_at,error=excluded.error`, value.ID, value.RunID, value.DestinationID, value.State, timeValue(value.VerifiedAt), nullable(value.Error))
	return err
}

func (s *Store) BackupVerifications(limit int) ([]backup.Verification, error) {
	if err := s.ensureBackupSchema(); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 500 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT id,run_id,destination_id,state,verified_at,COALESCE(error,'') FROM backup_verifications ORDER BY COALESCE(verified_at,'') DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]backup.Verification, 0)
	for rows.Next() {
		var value backup.Verification
		var verifiedAt sql.NullString
		if err := rows.Scan(&value.ID, &value.RunID, &value.DestinationID, &value.State, &verifiedAt, &value.Error); err != nil {
			return nil, err
		}
		if verifiedAt.Valid {
			parsed, parseErr := time.Parse(timeFormat, verifiedAt.String)
			if parseErr != nil {
				return nil, parseErr
			}
			value.VerifiedAt = &parsed
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func scanBackupDestination(rows interface{ Scan(...any) error }) (backup.Destination, error) {
	value, _, err := scanBackupDestinationWithCiphertext(rows)
	return value, err
}

func scanBackupDestinationWithCiphertext(rows interface{ Scan(...any) error }) (backup.Destination, []byte, error) {
	var value backup.Destination
	var kind string
	var retention string
	var enabled bool
	var ciphertext []byte
	var created, updated string
	if err := rows.Scan(&value.ID, &value.Name, &kind, &value.Target, &enabled, &retention, &ciphertext, &created, &updated); err != nil {
		return backup.Destination{}, nil, err
	}
	value.Type, value.Enabled, value.CredentialsConfigured = backup.DestinationType(kind), enabled, len(ciphertext) > 0
	if err := json.Unmarshal([]byte(retention), &value.Retention); err != nil {
		return backup.Destination{}, nil, err
	}
	var err error
	value.CreatedAt, err = time.Parse(timeFormat, created)
	if err != nil {
		return backup.Destination{}, nil, err
	}
	value.UpdatedAt, err = time.Parse(timeFormat, updated)
	if err != nil {
		return backup.Destination{}, nil, err
	}
	return value, ciphertext, nil
}
