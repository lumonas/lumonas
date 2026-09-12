package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/lumonas/lumonas/internal/model"
	_ "github.com/mattn/go-sqlite3"
)

type Store struct {
	db *sql.DB
	mu sync.Mutex
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
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

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
);`)
	if err != nil {
		return fmt.Errorf("migrate sqlite: %w", err)
	}
	_, err = s.db.Exec(`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(1, ?)`, time.Now().UTC().Format(timeFormat))
	return err
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
