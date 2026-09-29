package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/lumonas/lumonas/internal/foldersync"
)

func (s *Store) ensureFolderSyncSchema() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS folder_sync_tasks (id TEXT PRIMARY KEY, config_json TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS folder_sync_runs (id TEXT PRIMARY KEY, task_id TEXT NOT NULL, run_json TEXT NOT NULL, started_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS folder_sync_baselines (task_id TEXT PRIMARY KEY, initialized INTEGER NOT NULL, manifest_json TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS folder_sync_runs_task_idx ON folder_sync_runs(task_id,started_at);`)
	return err
}

func (s *Store) FolderSyncTasks() ([]foldersync.Task, error) {
	if err := s.ensureFolderSyncSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT config_json FROM folder_sync_tasks ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]foldersync.Task, 0)
	for rows.Next() {
		var raw string
		var value foldersync.Task
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *Store) FolderSyncTask(id string) (foldersync.Task, error) {
	if err := s.ensureFolderSyncSchema(); err != nil {
		return foldersync.Task{}, err
	}
	var raw string
	if err := s.db.QueryRow(`SELECT config_json FROM folder_sync_tasks WHERE id=?`, id).Scan(&raw); err != nil {
		return foldersync.Task{}, err
	}
	var value foldersync.Task
	err := json.Unmarshal([]byte(raw), &value)
	return value, err
}

func (s *Store) SaveFolderSyncTask(value foldersync.Task) error {
	if err := s.ensureFolderSyncSchema(); err != nil {
		return err
	}
	value.UpdatedAt = time.Now().UTC()
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO folder_sync_tasks(id,config_json,updated_at) VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET config_json=excluded.config_json,updated_at=excluded.updated_at`, value.ID, string(raw), value.UpdatedAt.Format(timeFormat))
	return err
}

func (s *Store) DeleteFolderSyncTask(id string) error {
	if err := s.ensureFolderSyncSchema(); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`DELETE FROM folder_sync_tasks WHERE id=?`, id)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return sql.ErrNoRows
	}
	if _, err := tx.Exec(`DELETE FROM folder_sync_baselines WHERE task_id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) FolderSyncBaseline(taskID string) (map[string]foldersync.Entry, bool, error) {
	if err := s.ensureFolderSyncSchema(); err != nil {
		return nil, false, err
	}
	var initialized int
	var raw string
	err := s.db.QueryRow(`SELECT initialized,manifest_json FROM folder_sync_baselines WHERE task_id=?`, taskID).Scan(&initialized, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	value := map[string]foldersync.Entry{}
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, false, err
	}
	return value, initialized != 0, nil
}

func (s *Store) SaveFolderSyncBaseline(taskID string, value map[string]foldersync.Entry) error {
	if err := s.ensureFolderSyncSchema(); err != nil {
		return err
	}
	if value == nil {
		value = map[string]foldersync.Entry{}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO folder_sync_baselines(task_id,initialized,manifest_json,updated_at) VALUES(?,1,?,?) ON CONFLICT(task_id) DO UPDATE SET initialized=1,manifest_json=excluded.manifest_json,updated_at=excluded.updated_at`, taskID, string(raw), time.Now().UTC().Format(timeFormat))
	return err
}

func (s *Store) DeleteFolderSyncBaseline(taskID string) error {
	if err := s.ensureFolderSyncSchema(); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM folder_sync_baselines WHERE task_id=?`, taskID)
	return err
}

func (s *Store) SaveFolderSyncRun(value foldersync.Run) error {
	if err := s.ensureFolderSyncSchema(); err != nil {
		return err
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO folder_sync_runs(id,task_id,run_json,started_at) VALUES(?,?,?,?)`, value.ID, value.TaskID, string(raw), value.StartedAt.UTC().Format(timeFormat))
	return err
}

func (s *Store) UpdateFolderSyncRun(value foldersync.Run) error {
	if err := s.ensureFolderSyncSchema(); err != nil {
		return err
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	result, err := s.db.Exec(`UPDATE folder_sync_runs SET run_json=? WHERE id=?`, string(raw), value.ID)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return errors.New("folder sync run not found")
	}
	return nil
}

func (s *Store) FolderSyncRuns(taskID string, limit int) ([]foldersync.Run, error) {
	if err := s.ensureFolderSyncSchema(); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(`SELECT run_json FROM folder_sync_runs WHERE task_id=? ORDER BY started_at DESC LIMIT ?`, taskID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]foldersync.Run, 0)
	for rows.Next() {
		var raw string
		var value foldersync.Run
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *Store) ReconcileInterruptedFolderSyncRuns() error {
	if err := s.ensureFolderSyncSchema(); err != nil {
		return err
	}
	rows, err := s.db.Query(`SELECT run_json FROM folder_sync_runs`)
	if err != nil {
		return err
	}
	var interrupted []foldersync.Run
	for rows.Next() {
		var raw string
		var value foldersync.Run
		if err := rows.Scan(&raw); err != nil {
			rows.Close()
			return err
		}
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			rows.Close()
			return err
		}
		if value.State == "running" {
			value.State = "failed"
			value.Error = "daemon restarted while this sync was running"
			finished := time.Now().UTC()
			value.FinishedAt = &finished
			interrupted = append(interrupted, value)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, value := range interrupted {
		if err := s.UpdateFolderSyncRun(value); err != nil {
			return err
		}
	}
	return nil
}
