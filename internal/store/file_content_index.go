package store

import (
	"database/sql"
	"strings"
	"time"
)

type FileContentDocument struct {
	ShareID    string
	Path       string
	Name       string
	Content    string
	SizeBytes  int64
	ModifiedAt time.Time
}

type FileContentResult struct {
	ShareID    string    `json:"shareId"`
	Path       string    `json:"path"`
	Name       string    `json:"name"`
	Snippet    string    `json:"snippet"`
	SizeBytes  int64     `json:"sizeBytes"`
	ModifiedAt time.Time `json:"modifiedAt"`
}

func (s *Store) ensureFileContentIndexSchema() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS file_content_index (
	 share_id TEXT NOT NULL, path TEXT NOT NULL, name TEXT NOT NULL, content TEXT NOT NULL,
	 size_bytes INTEGER NOT NULL, modified_at TEXT NOT NULL, indexed_at TEXT NOT NULL,
	 PRIMARY KEY(share_id,path)
	); CREATE INDEX IF NOT EXISTS file_content_search_idx ON file_content_index(share_id,name);
	CREATE TABLE IF NOT EXISTS file_content_index_meta (share_id TEXT PRIMARY KEY, document_count INTEGER NOT NULL, indexed_at TEXT NOT NULL)`)
	return err
}

// ReplaceFileContentIndex replaces one share's index atomically after its scan
// has completed, so an interrupted scan does not erase the previous index.
func (s *Store) ReplaceFileContentIndex(shareID string, documents []FileContentDocument, now time.Time) error {
	if err := s.ensureFileContentIndexSchema(); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`DELETE FROM file_content_index WHERE share_id=?`, shareID); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO file_content_index(share_id,path,name,content,size_bytes,modified_at,indexed_at) VALUES(?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	indexedAt := now.UTC().Format(timeFormat)
	for _, document := range documents {
		if _, err = stmt.Exec(shareID, document.Path, document.Name, document.Content, document.SizeBytes, document.ModifiedAt.UTC().Format(timeFormat), indexedAt); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(`INSERT INTO file_content_index_meta(share_id,document_count,indexed_at) VALUES(?,?,?) ON CONFLICT(share_id) DO UPDATE SET document_count=excluded.document_count,indexed_at=excluded.indexed_at`, shareID, len(documents), indexedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SearchFileContent(shareID, query string, limit int) ([]FileContentResult, error) {
	if err := s.ensureFileContentIndexSchema(); err != nil {
		return nil, err
	}
	query = strings.TrimSpace(query)
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(`SELECT share_id,path,name,substr(content,max(1,instr(lower(content),lower(?))-80),240),size_bytes,modified_at FROM file_content_index WHERE share_id=? AND instr(lower(content),lower(?))>0 ORDER BY name LIMIT ?`, query, shareID, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]FileContentResult, 0)
	for rows.Next() {
		var value FileContentResult
		var modified string
		if err := rows.Scan(&value.ShareID, &value.Path, &value.Name, &value.Snippet, &value.SizeBytes, &modified); err != nil {
			return nil, err
		}
		value.ModifiedAt, _ = parseTime(modified)
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *Store) FileContentIndexStatus(shareID string) (documents int, indexedAt *time.Time, err error) {
	if err = s.ensureFileContentIndexSchema(); err != nil {
		return
	}
	var timestamp sql.NullString
	err = s.db.QueryRow(`SELECT document_count,indexed_at FROM file_content_index_meta WHERE share_id=?`, shareID).Scan(&documents, &timestamp)
	if err == sql.ErrNoRows {
		// Preserve status for indexes created before the explicit metadata row.
		err = s.db.QueryRow(`SELECT count(*),max(indexed_at) FROM file_content_index WHERE share_id=?`, shareID).Scan(&documents, &timestamp)
	}
	if err != nil {
		return
	}
	if timestamp.Valid {
		value, _ := parseTime(timestamp.String)
		indexedAt = &value
	}
	return
}
