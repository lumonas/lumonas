package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lumonas/lumonas/internal/identity"
	"github.com/lumonas/lumonas/internal/shares"
)

const shareSchema = `
CREATE TABLE IF NOT EXISTS shares (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  path TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  guest INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS share_protocols (
  share_id TEXT NOT NULL REFERENCES shares(id) ON DELETE CASCADE,
  protocol TEXT NOT NULL,
  settings_json TEXT NOT NULL,
  PRIMARY KEY(share_id, protocol)
);`

func (s *Store) ensureShareSchema() error {
	if _, err := s.db.Exec(shareSchema); err != nil {
		return err
	}
	return nil
}

func (s *Store) ListManagedShares() ([]shares.ManagedShare, error) {
	if err := s.ensureShareSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT id,name,path,description,enabled,guest FROM shares ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]shares.ManagedShare, 0)
	for rows.Next() {
		var share shares.ManagedShare
		var enabled, guest int
		if err := rows.Scan(&share.ID, &share.Name, &share.Path, &share.Description, &enabled, &guest); err != nil {
			return nil, err
		}
		share.Enabled, share.Guest = enabled != 0, guest != 0
		share.Protocols, err = s.shareProtocols(share.ID)
		if err != nil {
			return nil, err
		}
		share.Access, err = s.shareAccess(share.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, share)
	}
	return result, rows.Err()
}

func (s *Store) ManagedShare(id string) (shares.ManagedShare, error) {
	if err := s.ensureShareSchema(); err != nil {
		return shares.ManagedShare{}, err
	}
	var share shares.ManagedShare
	var enabled, guest int
	if err := s.db.QueryRow(`SELECT id,name,path,description,enabled,guest FROM shares WHERE id=?`, id).Scan(&share.ID, &share.Name, &share.Path, &share.Description, &enabled, &guest); err != nil {
		return shares.ManagedShare{}, err
	}
	share.Enabled, share.Guest = enabled != 0, guest != 0
	var err error
	share.Protocols, err = s.shareProtocols(id)
	if err != nil {
		return shares.ManagedShare{}, err
	}
	share.Access, err = s.shareAccess(id)
	return share, err
}

func (s *Store) CreateManagedShare(share shares.ManagedShare) (shares.ManagedShare, error) {
	if err := share.Validate(); err != nil {
		return shares.ManagedShare{}, err
	}
	if share.ID == "" {
		share.ID = newStoreID("share")
	}
	if err := s.ensureShareSchema(); err != nil {
		return shares.ManagedShare{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return shares.ManagedShare{}, err
	}
	defer tx.Rollback()
	if err := insertManagedShare(tx, share); err != nil {
		return shares.ManagedShare{}, shareConflict(err)
	}
	if err := insertShareRelations(tx, share); err != nil {
		return shares.ManagedShare{}, err
	}
	if err := tx.Commit(); err != nil {
		return shares.ManagedShare{}, err
	}
	return s.ManagedShare(share.ID)
}

func (s *Store) UpdateManagedShare(share shares.ManagedShare) (shares.ManagedShare, error) {
	if err := share.Validate(); err != nil {
		return shares.ManagedShare{}, err
	}
	if err := s.ensureShareSchema(); err != nil {
		return shares.ManagedShare{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return shares.ManagedShare{}, err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(timeFormat)
	result, err := tx.Exec(`UPDATE shares SET name=?,path=?,description=?,enabled=?,guest=?,updated_at=? WHERE id=?`, share.Name, share.Path, share.Description, share.Enabled, share.Guest, now, share.ID)
	if err != nil {
		return shares.ManagedShare{}, shareConflict(err)
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return shares.ManagedShare{}, sql.ErrNoRows
	}
	if _, err := tx.Exec(`DELETE FROM share_protocols WHERE share_id=?`, share.ID); err != nil {
		return shares.ManagedShare{}, err
	}
	if _, err := tx.Exec(`DELETE FROM access_rules WHERE resource_type='share' AND resource_id=?`, share.ID); err != nil {
		return shares.ManagedShare{}, err
	}
	if err := insertShareRelations(tx, share); err != nil {
		return shares.ManagedShare{}, err
	}
	if err := tx.Commit(); err != nil {
		return shares.ManagedShare{}, err
	}
	return s.ManagedShare(share.ID)
}

func (s *Store) DeleteManagedShare(id string) error {
	if err := s.ensureShareSchema(); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM access_rules WHERE resource_type='share' AND resource_id=?`, id); err != nil {
		return err
	}
	result, err := tx.Exec(`DELETE FROM shares WHERE id=?`, id)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return sql.ErrNoRows
	}
	return tx.Commit()
}

func (s *Store) ImportLegacyShares(path string) error {
	if err := s.ensureShareSchema(); err != nil {
		return err
	}
	if value, ok := s.Meta("legacy_shares_imported"); ok && value == "true" {
		return nil
	}
	legacy, err := (shares.Store{Path: path}).Load()
	if err != nil {
		return err
	}
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM shares`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		for _, old := range legacy {
			managed := shares.ManagedShare{ID: old.ID, Name: old.Name, Path: old.Path, Description: old.Description, Enabled: old.Enabled, Guest: old.Guest}
			for _, protocol := range old.Protocols {
				managed.Protocols = append(managed.Protocols, shares.Protocol{Name: protocol})
			}
			for principal, level := range old.Access {
				managed.Access = append(managed.Access, shares.AccessRule{PrincipalName: principal, Level: level})
			}
			for _, rule := range managed.Access {
				if _, err := s.PrincipalByName(rule.PrincipalName); err == sql.ErrNoRows {
					if _, err := s.CreatePrincipal(identity.CreateInput{Kind: identity.KindGroup, Name: rule.PrincipalName}); err != nil {
						return fmt.Errorf("create legacy access group %q: %w", rule.PrincipalName, err)
					}
				}
			}
			if _, err := s.CreateManagedShare(managed); err != nil {
				return fmt.Errorf("import legacy share %q: %w", old.Name, err)
			}
		}
	}
	return s.SetMeta("legacy_shares_imported", "true")
}

func insertManagedShare(tx *sql.Tx, share shares.ManagedShare) error {
	now := time.Now().UTC().Format(timeFormat)
	_, err := tx.Exec(`INSERT INTO shares(id,name,path,description,enabled,guest,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, share.ID, share.Name, share.Path, share.Description, share.Enabled, share.Guest, now, now)
	return err
}

func insertShareRelations(tx *sql.Tx, share shares.ManagedShare) error {
	for _, protocol := range share.Protocols {
		settings, err := json.Marshal(protocol.Settings)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO share_protocols(share_id,protocol,settings_json) VALUES(?,?,?)`, share.ID, protocol.Name, string(settings)); err != nil {
			return err
		}
	}
	for _, rule := range share.Access {
		principalID := rule.PrincipalID
		if principalID == "" {
			if err := tx.QueryRow(`SELECT id FROM principals WHERE name=?`, rule.PrincipalName).Scan(&principalID); err != nil {
				return fmt.Errorf("access principal %q was not found", rule.PrincipalName)
			}
		}
		if _, err := tx.Exec(`INSERT INTO access_rules(resource_type,resource_id,principal_id,level) VALUES('share',?,?,?)`, share.ID, principalID, rule.Level); err != nil {
			return fmt.Errorf("save access rule for %q: %w", principalID, err)
		}
	}
	return nil
}

func (s *Store) shareProtocols(id string) ([]shares.Protocol, error) {
	rows, err := s.db.Query(`SELECT protocol,settings_json FROM share_protocols WHERE share_id=? ORDER BY protocol`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]shares.Protocol, 0)
	for rows.Next() {
		var protocol, settings string
		if err := rows.Scan(&protocol, &settings); err != nil {
			return nil, err
		}
		values := map[string]any{}
		if err := json.Unmarshal([]byte(settings), &values); err != nil {
			return nil, err
		}
		result = append(result, shares.Protocol{Name: protocol, Settings: values})
	}
	return result, rows.Err()
}

func (s *Store) shareAccess(id string) ([]shares.AccessRule, error) {
	rows, err := s.db.Query(`SELECT a.principal_id,p.name,a.level FROM access_rules a JOIN principals p ON p.id=a.principal_id WHERE a.resource_type='share' AND a.resource_id=? ORDER BY p.name`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]shares.AccessRule, 0)
	for rows.Next() {
		var rule shares.AccessRule
		if err := rows.Scan(&rule.PrincipalID, &rule.PrincipalName, &rule.Level); err != nil {
			return nil, err
		}
		result = append(result, rule)
	}
	return result, rows.Err()
}

func shareConflict(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("share already exists or conflicts with an existing share: %w", err)
}
