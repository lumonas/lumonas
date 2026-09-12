package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/lumonas/lumonas/internal/auth"
	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/storage"
	_ "github.com/mattn/go-sqlite3"
)

type Store struct {
	db   *sql.DB
	mu   sync.Mutex
	path string
}

type AuditEntry struct {
	ID           string         `json:"id"`
	Timestamp    time.Time      `json:"timestamp"`
	Actor        string         `json:"actor"`
	Action       string         `json:"action"`
	Outcome      string         `json:"outcome"`
	ResourceType string         `json:"resourceType,omitempty"`
	ResourceID   string         `json:"resourceId,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

func Open(path string) (*Store, error) {
	if path == "" {
		path = "/var/lib/mynas/mynas.db"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	db, err := sql.Open("sqlite3", path+"?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	s := &Store{db: db, path: path}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) BackupBytes() ([]byte, error) {
	temporary, err := os.CreateTemp(filepath.Dir(s.path), ".mynas-db-backup-*.db")
	if err != nil {
		return nil, err
	}
	temporaryPath := temporary.Name()
	_ = temporary.Close()
	defer os.Remove(temporaryPath)
	if _, err := s.db.Exec(`VACUUM INTO ?`, temporaryPath); err != nil {
		return nil, err
	}
	return os.ReadFile(temporaryPath)
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS jobs (
  id TEXT PRIMARY KEY, type TEXT NOT NULL, title TEXT NOT NULL, resource_id TEXT,
  state TEXT NOT NULL, progress REAL, stage TEXT, created_at TEXT NOT NULL,
  started_at TEXT, finished_at TEXT, error TEXT
);
CREATE TABLE IF NOT EXISTS events (
  id TEXT PRIMARY KEY, type TEXT NOT NULL, timestamp TEXT NOT NULL,
  severity TEXT NOT NULL, resource_type TEXT, resource_id TEXT, data_json TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS config_generations (
  generation INTEGER PRIMARY KEY, state TEXT NOT NULL, plan_hash TEXT NOT NULL,
  created_at TEXT NOT NULL, committed_at TEXT
);
CREATE TABLE IF NOT EXISTS storage_operations (
  operation_id TEXT PRIMARY KEY, plan_hash TEXT NOT NULL, status TEXT NOT NULL,
  expires_at TEXT NOT NULL, plan_json TEXT NOT NULL, created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY, username TEXT UNIQUE NOT NULL, password_hash TEXT NOT NULL, created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
  token_digest TEXT PRIMARY KEY, user_id TEXT NOT NULL, expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL, FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS audit_log (
  id TEXT PRIMARY KEY, timestamp TEXT NOT NULL, actor TEXT NOT NULL,
  action TEXT NOT NULL, outcome TEXT NOT NULL, resource_type TEXT,
  resource_id TEXT, metadata_json TEXT NOT NULL
);`)
	if err != nil {
		return fmt.Errorf("migrate sqlite: %w", err)
	}
	_, err = s.db.Exec(`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(1, ?)`, time.Now().UTC().Format(timeFormat))
	if err != nil {
		return err
	}
	var generation string
	if err := s.db.QueryRow(`SELECT value FROM meta WHERE key = 'config_generation'`).Scan(&generation); err == sql.ErrNoRows {
		generation = "1"
		now := time.Now().UTC().Format(timeFormat)
		if _, err := s.db.Exec(`INSERT INTO meta(key,value) VALUES('config_generation',?)`, generation); err != nil {
			return err
		}
		if _, err := s.db.Exec(`INSERT OR IGNORE INTO config_generations(generation,state,plan_hash,created_at,committed_at) VALUES(1,'committed','bootstrap',?,?)`, now, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Meta(key string) (string, bool) {
	var value string
	if err := s.db.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&value); err != nil {
		return "", false
	}
	return value, true
}

func (s *Store) SetMeta(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

func (s *Store) CurrentGeneration() int64 {
	value, ok := s.Meta("config_generation")
	if !ok {
		return 0
	}
	var generation int64
	_, _ = fmt.Sscan(value, &generation)
	return generation
}

func (s *Store) BeginGeneration(planHash string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.CurrentGeneration() + 1
	_, err := s.db.Exec(`INSERT INTO config_generations(generation,state,plan_hash,created_at) VALUES(?,?,?,?)`, next, "pending", planHash, time.Now().UTC().Format(timeFormat))
	return next, err
}

func (s *Store) CommitGeneration(generation int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC().Format(timeFormat)
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE config_generations SET state='committed',committed_at=? WHERE generation=?`, now, generation); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`INSERT INTO meta(key,value) VALUES('config_generation',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, fmt.Sprint(generation)); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *Store) SaveAudit(entry AuditEntry) error {
	if entry.ID == "" {
		entry.ID = newStoreID("audit")
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}
	metadata, err := json.Marshal(entry.Metadata)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO audit_log(id,timestamp,actor,action,outcome,resource_type,resource_id,metadata_json) VALUES(?,?,?,?,?,?,?,?)`, entry.ID, entry.Timestamp.Format(timeFormat), entry.Actor, entry.Action, entry.Outcome, nullable(entry.ResourceType), nullable(entry.ResourceID), string(metadata))
	return err
}

func (s *Store) Audit(limit int) ([]AuditEntry, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(`SELECT id,timestamp,actor,action,outcome,COALESCE(resource_type,''),COALESCE(resource_id,''),metadata_json FROM audit_log ORDER BY timestamp DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]AuditEntry, 0)
	for rows.Next() {
		var entry AuditEntry
		var timestamp, metadata string
		if err := rows.Scan(&entry.ID, &timestamp, &entry.Actor, &entry.Action, &entry.Outcome, &entry.ResourceType, &entry.ResourceID, &metadata); err != nil {
			return nil, err
		}
		entry.Timestamp, _ = parseTime(timestamp)
		if err := json.Unmarshal([]byte(metadata), &entry.Metadata); err != nil {
			entry.Metadata = map[string]any{}
		}
		result = append(result, entry)
	}
	return result, rows.Err()
}

func (s *Store) Jobs() ([]model.Job, error) {
	rows, err := s.db.Query(`SELECT id,type,title,COALESCE(resource_id,''),state,progress,COALESCE(stage,''),created_at,started_at,finished_at,COALESCE(error,'') FROM jobs ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []model.Job
	for rows.Next() {
		var j model.Job
		var progress sql.NullFloat64
		var created string
		var started, finished sql.NullString
		if err := rows.Scan(&j.ID, &j.Type, &j.Title, &j.ResourceID, &j.State, &progress, &j.Stage, &created, &started, &finished, &j.Error); err != nil {
			return nil, err
		}
		j.CreatedAt, _ = parseTime(created)
		if progress.Valid {
			j.Progress = &progress.Float64
		}
		if started.Valid {
			t, _ := parseTime(started.String)
			j.StartedAt = &t
		}
		if finished.Valid {
			t, _ := parseTime(finished.String)
			j.FinishedAt = &t
		}
		result = append(result, j)
	}
	return result, rows.Err()
}

func (s *Store) SaveJob(j model.Job) error {
	_, err := s.db.Exec(`INSERT INTO jobs(id,type,title,resource_id,state,progress,stage,created_at,started_at,finished_at,error)
VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET state=excluded.state,progress=excluded.progress,stage=excluded.stage,started_at=excluded.started_at,finished_at=excluded.finished_at,error=excluded.error`,
		j.ID, j.Type, j.Title, nullable(j.ResourceID), j.State, nullableFloat(j.Progress), nullable(j.Stage), j.CreatedAt.Format(timeFormat), timeValue(j.StartedAt), timeValue(j.FinishedAt), nullable(j.Error))
	return err
}

func (s *Store) SaveEvent(e model.Event) error {
	data, err := json.Marshal(e.Data)
	if err != nil {
		return err
	}
	var rt, ri any
	if e.Resource != nil {
		rt, ri = e.Resource.Type, e.Resource.ID
	}
	_, err = s.db.Exec(`INSERT OR REPLACE INTO events(id,type,timestamp,severity,resource_type,resource_id,data_json) VALUES(?,?,?,?,?,?,?)`, e.ID, e.Type, e.Timestamp.Format(timeFormat), e.Severity, rt, ri, string(data))
	return err
}

func (s *Store) Events(limit int) ([]model.Event, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(`SELECT id,type,timestamp,severity,resource_type,resource_id,data_json FROM events ORDER BY timestamp DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]model.Event, 0)
	for rows.Next() {
		var event model.Event
		var timestamp, data string
		var resourceType, resourceID sql.NullString
		if err := rows.Scan(&event.ID, &event.Type, &timestamp, &event.Severity, &resourceType, &resourceID, &data); err != nil {
			return nil, err
		}
		event.Timestamp, _ = parseTime(timestamp)
		if resourceType.Valid && resourceID.Valid {
			event.Resource = &model.ResourceRef{Type: resourceType.String, ID: resourceID.String}
		}
		if err := json.Unmarshal([]byte(data), &event.Data); err != nil {
			event.Data = map[string]any{}
		}
		result = append(result, event)
	}
	return result, rows.Err()
}

// PruneEvents keeps the event database bounded. Events are intentionally
// append-only for the retained window; older telemetry is not an audit record.
func (s *Store) PruneEvents(keep int) error {
	if keep < 100 {
		keep = 100
	}
	_, err := s.db.Exec(`DELETE FROM events WHERE id NOT IN (SELECT id FROM events ORDER BY timestamp DESC LIMIT ?)`, keep)
	return err
}

func (s *Store) SavePlan(plan storage.Plan) error {
	payload, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO storage_operations(operation_id,plan_hash,status,expires_at,plan_json,created_at) VALUES(?,?,?,?,?,?) ON CONFLICT(operation_id) DO UPDATE SET status=excluded.status,plan_json=excluded.plan_json`, plan.OperationID, plan.PlanHash, plan.Status, plan.ExpiresAt.Format(timeFormat), string(payload), time.Now().UTC().Format(timeFormat))
	return err
}

func (s *Store) Plan(operationID string) (storage.Plan, error) {
	var payload string
	if err := s.db.QueryRow(`SELECT plan_json FROM storage_operations WHERE operation_id = ?`, operationID).Scan(&payload); err != nil {
		return storage.Plan{}, err
	}
	var plan storage.Plan
	if err := json.Unmarshal([]byte(payload), &plan); err != nil {
		return storage.Plan{}, err
	}
	return plan, nil
}

func (s *Store) EnsureAdmin(username, password string) error {
	var existing string
	err := s.db.QueryRow(`SELECT id FROM users WHERE username = ?`, username).Scan(&existing)
	if err == nil {
		return nil
	}
	if err != sql.ErrNoRows {
		return err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO users(id,username,password_hash,created_at) VALUES(?,?,?,?)`, newStoreID("user"), username, hash, time.Now().UTC().Format(timeFormat))
	return err
}

func (s *Store) HasUsers() bool {
	var count int
	return s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count) == nil && count > 0
}

func (s *Store) CreateSession(username, password string, duration time.Duration) (string, time.Time, error) {
	var userID, hash string
	if err := s.db.QueryRow(`SELECT id,password_hash FROM users WHERE username = ?`, username).Scan(&userID, &hash); err != nil {
		return "", time.Time{}, fmt.Errorf("invalid credentials")
	}
	if !auth.VerifyPassword(password, hash) {
		return "", time.Time{}, fmt.Errorf("invalid credentials")
	}
	token, err := auth.NewToken()
	if err != nil {
		return "", time.Time{}, err
	}
	expires := time.Now().UTC().Add(duration)
	_, err = s.db.Exec(`INSERT INTO sessions(token_digest,user_id,expires_at,created_at) VALUES(?,?,?,?)`, auth.TokenDigest(token), userID, expires.Format(timeFormat), time.Now().UTC().Format(timeFormat))
	return token, expires, err
}

func (s *Store) SessionUser(token string) (string, bool) {
	var username, expires string
	if err := s.db.QueryRow(`SELECT users.username,sessions.expires_at FROM sessions JOIN users ON users.id=sessions.user_id WHERE sessions.token_digest = ?`, auth.TokenDigest(token)).Scan(&username, &expires); err != nil {
		return "", false
	}
	when, err := parseTime(expires)
	if err != nil || !time.Now().UTC().Before(when) {
		_, _ = s.db.Exec(`DELETE FROM sessions WHERE token_digest = ?`, auth.TokenDigest(token))
		return "", false
	}
	return username, true
}

func (s *Store) DeleteSession(token string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token_digest = ?`, auth.TokenDigest(token))
	return err
}

func newStoreID(prefix string) string { return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano()) }

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}
func nullableFloat(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}
func timeValue(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.Format(timeFormat)
}

const timeFormat = "2006-01-02T15:04:05.999999999Z07:00"

func parseTime(value string) (t time.Time, err error) { return time.Parse(timeFormat, value) }
