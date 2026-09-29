package store

import (
	"database/sql"
	"time"
)

type ReplicationPeer struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	URL             string     `json:"url"`
	CreatedAt       time.Time  `json:"createdAt"`
	LastSyncAt      *time.Time `json:"lastSyncAt,omitempty"`
	LastError       string     `json:"lastError,omitempty"`
	RemoteStatus    string     `json:"remoteStatus,omitempty"`
	RemoteVersion   string     `json:"remoteVersion,omitempty"`
	RemoteHealth    string     `json:"remoteHealth,omitempty"`
	RemoteCheckedAt *time.Time `json:"remoteCheckedAt,omitempty"`
}

type SnapshotReplicationTask struct {
	ID                 string     `json:"id"`
	PeerID             string     `json:"peerId"`
	Name               string     `json:"name"`
	SourceShareID      string     `json:"sourceShareId"`
	DestinationShareID string     `json:"destinationShareId"`
	ScheduleKind       string     `json:"scheduleKind"`
	TimeOfDay          string     `json:"timeOfDay"`
	Weekday            string     `json:"weekday,omitempty"`
	LastSnapshotName   string     `json:"lastSnapshotName,omitempty"`
	LastAttemptAt      *time.Time `json:"lastAttemptAt,omitempty"`
	LastSyncAt         *time.Time `json:"lastSyncAt,omitempty"`
	LastError          string     `json:"lastError,omitempty"`
	Running            bool       `json:"running,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"`
}

type SnapshotReplicationRun struct {
	ID           string     `json:"id"`
	TaskID       string     `json:"taskId"`
	JobID        string     `json:"jobId"`
	State        string     `json:"state"`
	Stage        string     `json:"stage"`
	Progress     float64    `json:"progress"`
	Bytes        int64      `json:"bytes"`
	SnapshotName string     `json:"snapshotName,omitempty"`
	Error        string     `json:"error,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	StartedAt    *time.Time `json:"startedAt,omitempty"`
	FinishedAt   *time.Time `json:"finishedAt,omitempty"`
}

func (s *Store) ensureReplicationSchema() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS replication_peers (
  id TEXT PRIMARY KEY, name TEXT NOT NULL, url TEXT NOT NULL, token_ciphertext BLOB NOT NULL,
  created_at TEXT NOT NULL, last_sync_at TEXT, last_error TEXT NOT NULL DEFAULT ''

);
CREATE TABLE IF NOT EXISTS snapshot_replication_tasks (
  id TEXT PRIMARY KEY, peer_id TEXT NOT NULL, name TEXT NOT NULL,
  source_share_id TEXT NOT NULL, destination_share_id TEXT NOT NULL,
  token_ciphertext BLOB NOT NULL, schedule_kind TEXT NOT NULL DEFAULT 'manual',
  time_of_day TEXT NOT NULL DEFAULT '02:00', weekday TEXT NOT NULL DEFAULT 'sunday',
  last_snapshot_name TEXT NOT NULL DEFAULT '', last_attempt_at TEXT, last_sync_at TEXT,
  last_error TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL,
  UNIQUE(peer_id, source_share_id, destination_share_id)
);
CREATE INDEX IF NOT EXISTS snapshot_replication_tasks_peer ON snapshot_replication_tasks(peer_id, name);`)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`CREATE TABLE IF NOT EXISTS snapshot_replication_runs (
  id TEXT PRIMARY KEY, task_id TEXT NOT NULL, job_id TEXT NOT NULL,
  state TEXT NOT NULL, stage TEXT NOT NULL DEFAULT '', progress REAL NOT NULL DEFAULT 0,
  bytes INTEGER NOT NULL DEFAULT 0, snapshot_name TEXT NOT NULL DEFAULT '',
  error TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, started_at TEXT, finished_at TEXT
);
CREATE INDEX IF NOT EXISTS snapshot_replication_runs_task ON snapshot_replication_runs(task_id, created_at DESC);`)
	return err
}

func (s *Store) CreateSnapshotReplicationRun(run SnapshotReplicationRun) error {
	if err := s.ensureReplicationSchema(); err != nil {
		return err
	}
	if run.CreatedAt.IsZero() {
		run.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.Exec(`INSERT INTO snapshot_replication_runs(id,task_id,job_id,state,stage,progress,bytes,snapshot_name,error,created_at,started_at,finished_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, run.ID, run.TaskID, run.JobID, run.State, run.Stage, run.Progress, run.Bytes, run.SnapshotName, run.Error, run.CreatedAt.UTC().Format(timeFormat), timeValue(run.StartedAt), timeValue(run.FinishedAt))
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`DELETE FROM snapshot_replication_runs WHERE task_id=? AND id NOT IN (SELECT id FROM snapshot_replication_runs WHERE task_id=? ORDER BY created_at DESC LIMIT 500)`, run.TaskID, run.TaskID)
	return err
}

func (s *Store) UpdateSnapshotReplicationRun(run SnapshotReplicationRun) error {
	if err := s.ensureReplicationSchema(); err != nil {
		return err
	}
	_, err := s.db.Exec(`UPDATE snapshot_replication_runs SET state=?,stage=?,progress=?,bytes=?,snapshot_name=?,error=?,started_at=COALESCE(?,started_at),finished_at=? WHERE id=?`, run.State, run.Stage, run.Progress, run.Bytes, run.SnapshotName, run.Error, timeValue(run.StartedAt), timeValue(run.FinishedAt), run.ID)
	return err
}

func (s *Store) SnapshotReplicationRuns(taskID string, limit int) ([]SnapshotReplicationRun, error) {
	if err := s.ensureReplicationSchema(); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT id,task_id,job_id,state,stage,progress,bytes,snapshot_name,error,created_at,started_at,finished_at FROM snapshot_replication_runs WHERE task_id=? ORDER BY created_at DESC LIMIT ?`, taskID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []SnapshotReplicationRun{}
	for rows.Next() {
		var run SnapshotReplicationRun
		var created string
		var started, finished sql.NullString
		if err := rows.Scan(&run.ID, &run.TaskID, &run.JobID, &run.State, &run.Stage, &run.Progress, &run.Bytes, &run.SnapshotName, &run.Error, &created, &started, &finished); err != nil {
			return nil, err
		}
		run.CreatedAt, _ = parseTime(created)
		if started.Valid {
			value, _ := parseTime(started.String)
			run.StartedAt = &value
		}
		if finished.Valid {
			value, _ := parseTime(finished.String)
			run.FinishedAt = &value
		}
		result = append(result, run)
	}
	return result, rows.Err()
}

func (s *Store) SaveSnapshotReplicationTask(task SnapshotReplicationTask, ciphertext []byte) error {
	if err := s.ensureReplicationSchema(); err != nil {
		return err
	}
	now := time.Now().UTC()
	task.CreatedAt = now
	_, err := s.db.Exec(`INSERT INTO snapshot_replication_tasks(id,peer_id,name,source_share_id,destination_share_id,token_ciphertext,schedule_kind,time_of_day,weekday,created_at)
VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET peer_id=excluded.peer_id,name=excluded.name,source_share_id=excluded.source_share_id,destination_share_id=excluded.destination_share_id,token_ciphertext=excluded.token_ciphertext,schedule_kind=excluded.schedule_kind,time_of_day=excluded.time_of_day,weekday=excluded.weekday`,
		task.ID, task.PeerID, task.Name, task.SourceShareID, task.DestinationShareID, ciphertext, task.ScheduleKind, task.TimeOfDay, task.Weekday, now.Format(timeFormat))
	return err
}

func (s *Store) SnapshotReplicationTasks(peerID string) ([]SnapshotReplicationTask, error) {
	if err := s.ensureReplicationSchema(); err != nil {
		return nil, err
	}
	query := `SELECT id,peer_id,name,source_share_id,destination_share_id,schedule_kind,time_of_day,weekday,last_snapshot_name,last_attempt_at,last_sync_at,last_error,created_at FROM snapshot_replication_tasks`
	args := []any{}
	if peerID != "" {
		query += ` WHERE peer_id=?`
		args = append(args, peerID)
	}
	query += ` ORDER BY name,id`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []SnapshotReplicationTask{}
	for rows.Next() {
		var task SnapshotReplicationTask
		var attempted, synced sql.NullString
		var created string
		if err := rows.Scan(&task.ID, &task.PeerID, &task.Name, &task.SourceShareID, &task.DestinationShareID, &task.ScheduleKind, &task.TimeOfDay, &task.Weekday, &task.LastSnapshotName, &attempted, &synced, &task.LastError, &created); err != nil {
			return nil, err
		}
		task.CreatedAt, _ = parseTime(created)
		if attempted.Valid {
			value, _ := parseTime(attempted.String)
			task.LastAttemptAt = &value
		}
		if synced.Valid {
			value, _ := parseTime(synced.String)
			task.LastSyncAt = &value
		}
		result = append(result, task)
	}
	return result, rows.Err()
}

func (s *Store) SnapshotReplicationTask(id string) (SnapshotReplicationTask, []byte, error) {
	if err := s.ensureReplicationSchema(); err != nil {
		return SnapshotReplicationTask{}, nil, err
	}
	var task SnapshotReplicationTask
	var attempted, synced sql.NullString
	var created string
	var ciphertext []byte
	err := s.db.QueryRow(`SELECT id,peer_id,name,source_share_id,destination_share_id,token_ciphertext,schedule_kind,time_of_day,weekday,last_snapshot_name,last_attempt_at,last_sync_at,last_error,created_at FROM snapshot_replication_tasks WHERE id=?`, id).
		Scan(&task.ID, &task.PeerID, &task.Name, &task.SourceShareID, &task.DestinationShareID, &ciphertext, &task.ScheduleKind, &task.TimeOfDay, &task.Weekday, &task.LastSnapshotName, &attempted, &synced, &task.LastError, &created)
	if err != nil {
		return SnapshotReplicationTask{}, nil, err
	}
	task.CreatedAt, _ = parseTime(created)
	if attempted.Valid {
		value, _ := parseTime(attempted.String)
		task.LastAttemptAt = &value
	}
	if synced.Valid {
		value, _ := parseTime(synced.String)
		task.LastSyncAt = &value
	}
	return task, ciphertext, nil
}

func (s *Store) RecordSnapshotReplicationResult(id, snapshotName, errText string) error {
	var synced any
	if errText == "" {
		synced = time.Now().UTC().Format(timeFormat)
	} else {
		snapshotName = ""
	}
	_, err := s.db.Exec(`UPDATE snapshot_replication_tasks SET last_sync_at=COALESCE(?,last_sync_at),last_snapshot_name=CASE WHEN ?='' THEN last_snapshot_name ELSE ? END,last_error=? WHERE id=?`, synced, snapshotName, snapshotName, errText, id)
	return err
}

func (s *Store) RecordSnapshotReplicationAttempt(id string, at time.Time) error {
	if err := s.ensureReplicationSchema(); err != nil {
		return err
	}
	_, err := s.db.Exec(`UPDATE snapshot_replication_tasks SET last_attempt_at=? WHERE id=?`, at.UTC().Format(timeFormat), id)
	return err
}

func (s *Store) DeleteSnapshotReplicationTask(id string) error {
	if err := s.ensureReplicationSchema(); err != nil {
		return err
	}
	result, err := s.db.Exec(`DELETE FROM snapshot_replication_tasks WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) SaveReplicationPeer(peer ReplicationPeer, ciphertext []byte) error {
	if err := s.ensureReplicationSchema(); err != nil {
		return err
	}
	now := time.Now().UTC()
	peer.CreatedAt = now
	_, err := s.db.Exec(`INSERT INTO replication_peers(id,name,url,token_ciphertext,created_at,last_sync_at,last_error) VALUES(?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,url=excluded.url,token_ciphertext=excluded.token_ciphertext,last_error=''`, peer.ID, peer.Name, peer.URL, ciphertext, now.Format(timeFormat), nil, "")
	return err
}

func (s *Store) ReplicationPeers() ([]ReplicationPeer, error) {
	if err := s.ensureReplicationSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT id,name,url,created_at,last_sync_at,last_error FROM replication_peers ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []ReplicationPeer{}
	for rows.Next() {
		var peer ReplicationPeer
		var created string
		var synced sql.NullString
		if err := rows.Scan(&peer.ID, &peer.Name, &peer.URL, &created, &synced, &peer.LastError); err != nil {
			return nil, err
		}
		peer.CreatedAt, _ = parseTime(created)
		if synced.Valid {
			value, _ := parseTime(synced.String)
			peer.LastSyncAt = &value
		}
		result = append(result, peer)
	}
	return result, rows.Err()
}

func (s *Store) ReplicationPeerCredentials(id string) (ReplicationPeer, []byte, error) {
	var peer ReplicationPeer
	var created string
	var synced sql.NullString
	var encrypted []byte
	err := s.db.QueryRow(`SELECT id,name,url,token_ciphertext,created_at,last_sync_at,last_error FROM replication_peers WHERE id=?`, id).Scan(&peer.ID, &peer.Name, &peer.URL, &encrypted, &created, &synced, &peer.LastError)
	if err != nil {
		return ReplicationPeer{}, nil, err
	}
	peer.CreatedAt, _ = parseTime(created)
	if synced.Valid {
		value, _ := parseTime(synced.String)
		peer.LastSyncAt = &value
	}
	return peer, encrypted, nil
}

func (s *Store) RecordReplicationResult(id string, errText string) error {
	var synced any
	if errText == "" {
		synced = time.Now().UTC().Format(timeFormat)
	}
	_, err := s.db.Exec(`UPDATE replication_peers SET last_sync_at=COALESCE(?,last_sync_at),last_error=? WHERE id=?`, synced, errText, id)
	return err
}
func (s *Store) DeleteReplicationPeer(id string) error {
	result, err := s.db.Exec(`DELETE FROM replication_peers WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
