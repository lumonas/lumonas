package store

import (
	"database/sql"
	"errors"
	"time"
)

type FileRequest struct {
	ID            string     `json:"id"`
	ShareID       string     `json:"shareId"`
	Path          string     `json:"path"`
	TokenHash     string     `json:"-"`
	CreatedAt     time.Time  `json:"createdAt"`
	ExpiresAt     time.Time  `json:"expiresAt"`
	MaxFiles      int        `json:"maxFiles"`
	MaxBytes      int64      `json:"maxBytes"`
	ReceivedFiles int        `json:"receivedFiles"`
	ReceivedBytes int64      `json:"receivedBytes"`
	RevokedAt     *time.Time `json:"revokedAt,omitempty"`
}

// FileShareLink is a revocable, expiring read-only capability for one folder.
// PasswordHash and TokenHash are never serialized to API clients.
type FileShareLink struct {
	ID           string     `json:"id"`
	ShareID      string     `json:"shareId"`
	Path         string     `json:"path"`
	TokenHash    string     `json:"-"`
	PasswordHash string     `json:"-"`
	CreatedAt    time.Time  `json:"createdAt"`
	ExpiresAt    time.Time  `json:"expiresAt"`
	Downloads    int        `json:"downloads"`
	RevokedAt    *time.Time `json:"revokedAt,omitempty"`
}

func (s *Store) ensureFileRequestSchema() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS file_requests (
	 id TEXT PRIMARY KEY, share_id TEXT NOT NULL, path TEXT NOT NULL, token_hash TEXT NOT NULL UNIQUE,
	 created_at TEXT NOT NULL, expires_at TEXT NOT NULL, max_files INTEGER NOT NULL, max_bytes INTEGER NOT NULL,
	 received_files INTEGER NOT NULL DEFAULT 0, received_bytes INTEGER NOT NULL DEFAULT 0, revoked_at TEXT
	); CREATE INDEX IF NOT EXISTS file_requests_share_idx ON file_requests(share_id,created_at DESC)`)
	return err
}

func (s *Store) CreateFileRequest(value FileRequest) error {
	if err := s.ensureFileRequestSchema(); err != nil {
		return err
	}
	value.CreatedAt = time.Now().UTC()
	_, err := s.db.Exec(`INSERT INTO file_requests(id,share_id,path,token_hash,created_at,expires_at,max_files,max_bytes) VALUES(?,?,?,?,?,?,?,?)`, value.ID, value.ShareID, value.Path, value.TokenHash, value.CreatedAt.Format(timeFormat), value.ExpiresAt.UTC().Format(timeFormat), value.MaxFiles, value.MaxBytes)
	return err
}

func scanFileRequest(row interface{ Scan(...any) error }) (FileRequest, error) {
	var value FileRequest
	var created, expires string
	var revoked sql.NullString
	err := row.Scan(&value.ID, &value.ShareID, &value.Path, &value.TokenHash, &created, &expires, &value.MaxFiles, &value.MaxBytes, &value.ReceivedFiles, &value.ReceivedBytes, &revoked)
	if err != nil {
		return FileRequest{}, err
	}
	value.CreatedAt, _ = parseTime(created)
	value.ExpiresAt, _ = parseTime(expires)
	if revoked.Valid {
		t, _ := parseTime(revoked.String)
		value.RevokedAt = &t
	}
	return value, nil
}

const fileRequestColumns = `id,share_id,path,token_hash,created_at,expires_at,max_files,max_bytes,received_files,received_bytes,revoked_at`

func (s *Store) FileRequestByTokenHash(hash string) (FileRequest, error) {
	if err := s.ensureFileRequestSchema(); err != nil {
		return FileRequest{}, err
	}
	return scanFileRequest(s.db.QueryRow(`SELECT `+fileRequestColumns+` FROM file_requests WHERE token_hash=?`, hash))
}

func (s *Store) FileRequests(shareID string) ([]FileRequest, error) {
	if err := s.ensureFileRequestSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT `+fileRequestColumns+` FROM file_requests WHERE share_id=? ORDER BY created_at DESC LIMIT 100`, shareID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]FileRequest, 0)
	for rows.Next() {
		value, scanErr := scanFileRequest(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *Store) RecordFileRequestUpload(id string, bytes int64, now time.Time) error {
	result, err := s.db.Exec(`UPDATE file_requests SET received_files=received_files+1,received_bytes=received_bytes+? WHERE id=? AND revoked_at IS NULL AND expires_at>? AND received_files<max_files AND received_bytes+?<=max_bytes`, bytes, id, now.UTC().Format(timeFormat), bytes)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return errors.New("file request limit was reached or the link expired")
	}
	return nil
}

func (s *Store) RevokeFileRequest(id string, now time.Time) error {
	if err := s.ensureFileRequestSchema(); err != nil {
		return err
	}
	result, err := s.db.Exec(`UPDATE file_requests SET revoked_at=? WHERE id=? AND revoked_at IS NULL`, now.UTC().Format(timeFormat), id)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ensureFileShareLinkSchema() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS file_share_links (
	 id TEXT PRIMARY KEY, share_id TEXT NOT NULL, path TEXT NOT NULL, token_hash TEXT NOT NULL UNIQUE,
	 password_hash TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, expires_at TEXT NOT NULL,
	 downloads INTEGER NOT NULL DEFAULT 0, revoked_at TEXT
	); CREATE INDEX IF NOT EXISTS file_share_links_share_idx ON file_share_links(share_id,created_at DESC)`)
	return err
}

func (s *Store) CreateFileShareLink(value FileShareLink) error {
	if err := s.ensureFileShareLinkSchema(); err != nil {
		return err
	}
	value.CreatedAt = time.Now().UTC()
	_, err := s.db.Exec(`INSERT INTO file_share_links(id,share_id,path,token_hash,password_hash,created_at,expires_at) VALUES(?,?,?,?,?,?,?)`, value.ID, value.ShareID, value.Path, value.TokenHash, value.PasswordHash, value.CreatedAt.Format(timeFormat), value.ExpiresAt.UTC().Format(timeFormat))
	return err
}

const fileShareLinkColumns = `id,share_id,path,token_hash,password_hash,created_at,expires_at,downloads,revoked_at`

func scanFileShareLink(row interface{ Scan(...any) error }) (FileShareLink, error) {
	var value FileShareLink
	var created, expires string
	var revoked sql.NullString
	err := row.Scan(&value.ID, &value.ShareID, &value.Path, &value.TokenHash, &value.PasswordHash, &created, &expires, &value.Downloads, &revoked)
	if err != nil {
		return FileShareLink{}, err
	}
	value.CreatedAt, _ = parseTime(created)
	value.ExpiresAt, _ = parseTime(expires)
	if revoked.Valid {
		t, _ := parseTime(revoked.String)
		value.RevokedAt = &t
	}
	return value, nil
}

func (s *Store) FileShareLinkByTokenHash(hash string) (FileShareLink, error) {
	if err := s.ensureFileShareLinkSchema(); err != nil {
		return FileShareLink{}, err
	}
	return scanFileShareLink(s.db.QueryRow(`SELECT `+fileShareLinkColumns+` FROM file_share_links WHERE token_hash=?`, hash))
}

func (s *Store) FileShareLinks(shareID string) ([]FileShareLink, error) {
	if err := s.ensureFileShareLinkSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT `+fileShareLinkColumns+` FROM file_share_links WHERE share_id=? ORDER BY created_at DESC LIMIT 100`, shareID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]FileShareLink, 0)
	for rows.Next() {
		value, scanErr := scanFileShareLink(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *Store) RevokeFileShareLink(id string, now time.Time) error {
	if err := s.ensureFileShareLinkSchema(); err != nil {
		return err
	}
	result, err := s.db.Exec(`UPDATE file_share_links SET revoked_at=? WHERE id=? AND revoked_at IS NULL`, now.UTC().Format(timeFormat), id)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) RecordFileShareDownload(tokenHash string, now time.Time) error {
	if err := s.ensureFileShareLinkSchema(); err != nil {
		return err
	}
	result, err := s.db.Exec(`UPDATE file_share_links SET downloads=downloads+1 WHERE token_hash=? AND revoked_at IS NULL AND expires_at>?`, tokenHash, now.UTC().Format(timeFormat))
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return errors.New("share link expired or revoked")
	}
	return nil
}
