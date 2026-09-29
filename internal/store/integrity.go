package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

type IntegrityRecord struct {
	Path       string    `json:"path"`
	SHA256     string    `json:"sha256"`
	SizeBytes  int64     `json:"sizeBytes"`
	ModifiedAt time.Time `json:"modifiedAt"`
}

type IntegrityReport struct {
	BaselineAt   time.Time `json:"baselineAt"`
	VerifiedAt   time.Time `json:"verifiedAt"`
	Unchanged    int       `json:"unchanged"`
	ChangedCount int       `json:"changedCount"`
	MissingCount int       `json:"missingCount"`
	AddedCount   int       `json:"addedCount"`
	Changed      []string  `json:"changed"`
	Missing      []string  `json:"missing"`
	Added        []string  `json:"added"`
}

type IntegrityStatus struct {
	ShareID    string           `json:"shareId"`
	FileCount  int              `json:"fileCount"`
	BaselineAt *time.Time       `json:"baselineAt,omitempty"`
	Report     *IntegrityReport `json:"report,omitempty"`
}

func (s *Store) ensureIntegritySchema() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS share_integrity_baselines(share_id TEXT PRIMARY KEY,baseline_at TEXT NOT NULL,file_count INTEGER NOT NULL);
	CREATE TABLE IF NOT EXISTS share_integrity_files(share_id TEXT NOT NULL,path TEXT NOT NULL,sha256 TEXT NOT NULL,size_bytes INTEGER NOT NULL,modified_at TEXT NOT NULL,PRIMARY KEY(share_id,path));
	CREATE TABLE IF NOT EXISTS share_integrity_reports(share_id TEXT PRIMARY KEY,report_json TEXT NOT NULL,verified_at TEXT NOT NULL);`)
	return err
}

func (s *Store) ReplaceIntegrityBaseline(shareID string, records []IntegrityRecord, now time.Time) error {
	if err := s.ensureIntegritySchema(); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`DELETE FROM share_integrity_files WHERE share_id=?`, shareID); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO share_integrity_files(share_id,path,sha256,size_bytes,modified_at) VALUES(?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, record := range records {
		if _, err = stmt.Exec(shareID, record.Path, record.SHA256, record.SizeBytes, record.ModifiedAt.UTC().Format(timeFormat)); err != nil {
			return err
		}
	}
	_, err = tx.Exec(`INSERT INTO share_integrity_baselines(share_id,baseline_at,file_count) VALUES(?,?,?) ON CONFLICT(share_id) DO UPDATE SET baseline_at=excluded.baseline_at,file_count=excluded.file_count`, shareID, now.UTC().Format(timeFormat), len(records))
	if err != nil {
		return err
	}
	_, err = tx.Exec(`DELETE FROM share_integrity_reports WHERE share_id=?`, shareID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) IntegrityBaseline(shareID string) (time.Time, map[string]IntegrityRecord, error) {
	if err := s.ensureIntegritySchema(); err != nil {
		return time.Time{}, nil, err
	}
	var timestamp string
	if err := s.db.QueryRow(`SELECT baseline_at FROM share_integrity_baselines WHERE share_id=?`, shareID).Scan(&timestamp); err != nil {
		return time.Time{}, nil, err
	}
	baselineAt, _ := parseTime(timestamp)
	rows, err := s.db.Query(`SELECT path,sha256,size_bytes,modified_at FROM share_integrity_files WHERE share_id=?`, shareID)
	if err != nil {
		return time.Time{}, nil, err
	}
	defer rows.Close()
	files := map[string]IntegrityRecord{}
	for rows.Next() {
		var value IntegrityRecord
		var modified string
		if err := rows.Scan(&value.Path, &value.SHA256, &value.SizeBytes, &modified); err != nil {
			return time.Time{}, nil, err
		}
		value.ModifiedAt, _ = parseTime(modified)
		files[value.Path] = value
	}
	return baselineAt, files, rows.Err()
}

func (s *Store) SaveIntegrityReport(shareID string, report IntegrityReport) error {
	if err := s.ensureIntegritySchema(); err != nil {
		return err
	}
	data, err := json.Marshal(report)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO share_integrity_reports(share_id,report_json,verified_at) VALUES(?,?,?) ON CONFLICT(share_id) DO UPDATE SET report_json=excluded.report_json,verified_at=excluded.verified_at`, shareID, string(data), report.VerifiedAt.UTC().Format(timeFormat))
	return err
}

func (s *Store) IntegrityStatus(shareID string) (IntegrityStatus, error) {
	if err := s.ensureIntegritySchema(); err != nil {
		return IntegrityStatus{}, err
	}
	status := IntegrityStatus{ShareID: shareID}
	var baseline string
	err := s.db.QueryRow(`SELECT baseline_at,file_count FROM share_integrity_baselines WHERE share_id=?`, shareID).Scan(&baseline, &status.FileCount)
	if err == nil {
		value, _ := parseTime(baseline)
		status.BaselineAt = &value
	} else if !errors.Is(err, sql.ErrNoRows) {
		return status, err
	}
	var encoded string
	err = s.db.QueryRow(`SELECT report_json FROM share_integrity_reports WHERE share_id=?`, shareID).Scan(&encoded)
	if err == nil {
		var report IntegrityReport
		if err = json.Unmarshal([]byte(encoded), &report); err != nil {
			return status, err
		}
		status.Report = &report
	} else if !errors.Is(err, sql.ErrNoRows) {
		return status, err
	}
	return status, nil
}
