package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/auth"
)

// APITokenSummary never includes token material or its digest.
type APITokenSummary struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Scopes     []string   `json:"scopes"`
	CreatedAt  time.Time  `json:"createdAt"`
	ExpiresAt  *time.Time `json:"expiresAt,omitempty"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
}

type APITokenCreate struct {
	ID        string
	OwnerID   string
	Name      string
	Scopes    []string
	ExpiresAt *time.Time
}

func (s *Store) ensureAPITokenSchema() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS api_tokens (
  id TEXT PRIMARY KEY, owner_id TEXT NOT NULL, name TEXT NOT NULL,
  token_digest TEXT NOT NULL UNIQUE, scopes_json TEXT NOT NULL,
  created_at TEXT NOT NULL, expires_at TEXT, last_used_at TEXT,
  FOREIGN KEY(owner_id) REFERENCES principals(id) ON DELETE CASCADE
)`)
	return err
}

func (s *Store) CreateAPIToken(input APITokenCreate) (APITokenSummary, string, error) {
	if strings.TrimSpace(input.ID) == "" || strings.TrimSpace(input.OwnerID) == "" || strings.TrimSpace(input.Name) == "" || len(input.Name) > 80 || len(input.Scopes) == 0 {
		return APITokenSummary{}, "", errors.New("token id, owner, name, and at least one scope are required")
	}
	scopes, err := json.Marshal(input.Scopes)
	if err != nil {
		return APITokenSummary{}, "", err
	}
	raw, err := auth.NewToken()
	if err != nil {
		return APITokenSummary{}, "", err
	}
	now := time.Now().UTC()
	_, err = s.db.Exec(`INSERT INTO api_tokens(id,owner_id,name,token_digest,scopes_json,created_at,expires_at) VALUES(?,?,?,?,?,?,?)`, input.ID, input.OwnerID, strings.TrimSpace(input.Name), auth.TokenDigest(raw), string(scopes), now.Format(timeFormat), nullableTokenTime(input.ExpiresAt))
	if err != nil {
		return APITokenSummary{}, "", err
	}
	return APITokenSummary{ID: input.ID, Name: strings.TrimSpace(input.Name), Scopes: input.Scopes, CreatedAt: now, ExpiresAt: input.ExpiresAt}, raw, nil
}

func (s *Store) ListAPITokens(ownerID string) ([]APITokenSummary, error) {
	rows, err := s.db.Query(`SELECT id,name,scopes_json,created_at,expires_at,last_used_at FROM api_tokens WHERE owner_id=? ORDER BY created_at DESC`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []APITokenSummary{}
	for rows.Next() {
		var token APITokenSummary
		var created string
		var expires, used sql.NullString
		var scopes string
		if err := rows.Scan(&token.ID, &token.Name, &scopes, &created, &expires, &used); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(scopes), &token.Scopes); err != nil {
			return nil, err
		}
		token.CreatedAt, _ = parseTime(created)
		if expires.Valid {
			value, _ := parseTime(expires.String)
			token.ExpiresAt = &value
		}
		if used.Valid {
			value, _ := parseTime(used.String)
			token.LastUsedAt = &value
		}
		result = append(result, token)
	}
	return result, rows.Err()
}

// ResolveAPIToken validates a bearer token and updates its last-use marker.
func (s *Store) ResolveAPIToken(raw string) (ownerName string, scopes []string, ok bool) {
	if strings.TrimSpace(raw) == "" {
		return "", nil, false
	}
	var ownerID, encoded, expiry string
	err := s.db.QueryRow(`SELECT owner_id,scopes_json,COALESCE(expires_at,'') FROM api_tokens WHERE token_digest=?`, auth.TokenDigest(raw)).Scan(&ownerID, &encoded, &expiry)
	if err != nil {
		return "", nil, false
	}
	if expiry != "" {
		at, parseErr := parseTime(expiry)
		if parseErr != nil || !at.After(time.Now().UTC()) {
			return "", nil, false
		}
	}
	if json.Unmarshal([]byte(encoded), &scopes) != nil {
		return "", nil, false
	}
	var name string
	if s.db.QueryRow(`SELECT name FROM principals WHERE id=? AND enabled=1`, ownerID).Scan(&name) != nil {
		return "", nil, false
	}
	_, _ = s.db.Exec(`UPDATE api_tokens SET last_used_at=? WHERE token_digest=?`, time.Now().UTC().Format(timeFormat), auth.TokenDigest(raw))
	return name, scopes, true
}

func (s *Store) DeleteAPIToken(ownerID, id string) error {
	result, err := s.db.Exec(`DELETE FROM api_tokens WHERE id=? AND owner_id=?`, id, ownerID)
	if err != nil {
		return err
	}
	changed, _ := result.RowsAffected()
	if changed == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func nullableTokenTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(timeFormat)
}
