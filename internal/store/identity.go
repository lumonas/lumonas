package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lumonas/lumonas/internal/auth"
	"github.com/lumonas/lumonas/internal/identity"
)

const identitySchema = `
CREATE TABLE IF NOT EXISTS principals (
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL CHECK(kind IN ('user','group','service')),
  name TEXT NOT NULL,
  uid INTEGER UNIQUE,
  gid INTEGER UNIQUE,
  password_hash TEXT,
  management_role TEXT NOT NULL DEFAULT 'none',
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(kind, name)
);
CREATE TABLE IF NOT EXISTS group_members (
  group_id TEXT NOT NULL REFERENCES principals(id) ON DELETE CASCADE,
  principal_id TEXT NOT NULL REFERENCES principals(id) ON DELETE CASCADE,
  PRIMARY KEY(group_id, principal_id)
);
CREATE TABLE IF NOT EXISTS access_rules (
  resource_type TEXT NOT NULL,
  resource_id TEXT NOT NULL,
  principal_id TEXT NOT NULL REFERENCES principals(id) ON DELETE RESTRICT,
  level TEXT NOT NULL CHECK(level IN ('none','read','write')),
  PRIMARY KEY(resource_type, resource_id, principal_id)
);`

func (s *Store) ensureIdentitySchema() error {
	if _, err := s.db.Exec(identitySchema); err != nil {
		return err
	}
	if err := s.ensureTOTPColumns(); err != nil {
		return err
	}
	return s.syncLegacyUsers()
}

func (s *Store) syncLegacyUsers() error {
	rows, err := s.db.Query(`SELECT id,username,password_hash,created_at FROM users`)
	if err != nil {
		return err
	}
	type legacyUser struct {
		id, username, passwordHash, createdAt string
	}
	legacy := make([]legacyUser, 0)
	for rows.Next() {
		var id, username, passwordHash, createdAt string
		if err := rows.Scan(&id, &username, &passwordHash, &createdAt); err != nil {
			rows.Close()
			return err
		}
		legacy = append(legacy, legacyUser{id: id, username: username, passwordHash: passwordHash, createdAt: createdAt})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, user := range legacy {
		id, username, passwordHash, createdAt := user.id, user.username, user.passwordHash, user.createdAt
		var exists int
		if err := s.db.QueryRow(`SELECT 1 FROM principals WHERE id=?`, id).Scan(&exists); err != sql.ErrNoRows {
			if err != nil {
				return err
			}
			continue
		}
		uid, _, err := allocateIdentityIDForStore(s)
		if err != nil {
			return err
		}
		role := identity.RoleAdmin
		if username == "admin" {
			role = identity.RoleOwner
		}
		if _, err := s.db.Exec(`INSERT INTO principals(id,kind,name,uid,password_hash,management_role,enabled,created_at,updated_at) VALUES(?,?,?, ?,?,?,1,?,?)`, id, identity.KindUser, username, uid, passwordHash, role, createdAt, createdAt); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (s *Store) ListPrincipals(kind identity.Kind) ([]identity.Principal, error) {
	if err := s.ensureIdentitySchema(); err != nil {
		return nil, err
	}
	query := `SELECT id,kind,name,uid,gid,management_role,enabled,created_at,updated_at FROM principals`
	args := []any{}
	if kind != "" {
		query += ` WHERE kind = ?`
		args = append(args, kind)
	}
	query += ` ORDER BY kind,name`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]identity.Principal, 0)
	for rows.Next() {
		principal, err := scanPrincipal(rows)
		if err != nil {
			return nil, err
		}
		principal.Groups, err = s.groupsForPrincipal(principal.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, principal)
	}
	return result, rows.Err()
}

func (s *Store) Principal(id string) (identity.Principal, error) {
	if err := s.ensureIdentitySchema(); err != nil {
		return identity.Principal{}, err
	}
	row := s.db.QueryRow(`SELECT id,kind,name,uid,gid,management_role,enabled,created_at,updated_at FROM principals WHERE id = ?`, id)
	principal, err := scanPrincipal(row)
	if err != nil {
		return identity.Principal{}, err
	}
	principal.Groups, err = s.groupsForPrincipal(principal.ID)
	return principal, err
}

func (s *Store) PrincipalByName(name string) (identity.Principal, error) {
	if err := s.ensureIdentitySchema(); err != nil {
		return identity.Principal{}, err
	}
	row := s.db.QueryRow(`SELECT id,kind,name,uid,gid,management_role,enabled,created_at,updated_at FROM principals WHERE name = ? ORDER BY kind LIMIT 1`, name)
	principal, err := scanPrincipal(row)
	if err != nil {
		return identity.Principal{}, err
	}
	principal.Groups, err = s.groupsForPrincipal(principal.ID)
	return principal, err
}

func (s *Store) CreatePrincipal(input identity.CreateInput) (identity.Principal, error) {
	input.Name = identity.NormalizeName(input.Name)
	input.ManagementRole = identity.NormalizeRole(input.ManagementRole)
	if err := identity.ValidateCreate(input); err != nil {
		return identity.Principal{}, err
	}
	if err := s.ensureIdentitySchema(); err != nil {
		return identity.Principal{}, err
	}
	var passwordHash any
	if input.Password != "" {
		hash, err := auth.HashPassword(input.Password)
		if err != nil {
			return identity.Principal{}, err
		}
		passwordHash = hash
	}
	now := time.Now().UTC().Format(timeFormat)
	id := newStoreID("principal")
	tx, err := s.db.Begin()
	if err != nil {
		return identity.Principal{}, err
	}
	defer tx.Rollback()
	uid, gid, err := allocateIdentityID(tx, input.Kind)
	if err != nil {
		return identity.Principal{}, err
	}
	_, err = tx.Exec(`INSERT INTO principals(id,kind,name,uid,gid,password_hash,management_role,enabled,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, id, input.Kind, input.Name, uid, gid, passwordHash, input.ManagementRole, 1, now, now)
	if err != nil {
		return identity.Principal{}, principalConflict(err)
	}
	if input.Kind == identity.KindUser && input.ManagementRole != identity.RoleNone {
		if _, err := tx.Exec(`INSERT INTO users(id,username,password_hash,created_at) VALUES(?,?,?,?)`, id, input.Name, passwordHash, now); err != nil {
			return identity.Principal{}, principalConflict(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return identity.Principal{}, err
	}
	return s.Principal(id)
}

func (s *Store) UpdatePrincipal(id string, input identity.UpdateInput) (identity.Principal, error) {
	if err := s.ensureIdentitySchema(); err != nil {
		return identity.Principal{}, err
	}
	current, err := s.Principal(id)
	if err != nil {
		return identity.Principal{}, err
	}
	name := current.Name
	if input.Name != nil {
		name = identity.NormalizeName(*input.Name)
	}
	role := current.ManagementRole
	if input.ManagementRole != nil {
		role = identity.NormalizeRole(*input.ManagementRole)
	}
	if err := identity.ValidateUpdate(current.Kind, name, role); err != nil {
		return identity.Principal{}, err
	}
	enabled := current.Enabled
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	now := time.Now().UTC().Format(timeFormat)
	_, err = s.db.Exec(`UPDATE principals SET name=?,management_role=?,enabled=?,updated_at=? WHERE id=?`, name, role, enabled, now, id)
	if err != nil {
		return identity.Principal{}, principalConflict(err)
	}
	if current.Kind == identity.KindUser {
		if role == identity.RoleNone || !enabled {
			if _, err := s.db.Exec(`DELETE FROM users WHERE id = ?`, id); err != nil {
				return identity.Principal{}, err
			}
		} else {
			var passwordHash string
			if err := s.db.QueryRow(`SELECT password_hash FROM principals WHERE id=?`, id).Scan(&passwordHash); err != nil {
				return identity.Principal{}, err
			}
			if _, err := s.db.Exec(`INSERT INTO users(id,username,password_hash,created_at) VALUES(?,?,?,?) ON CONFLICT(id) DO UPDATE SET username=excluded.username,password_hash=excluded.password_hash`, id, name, passwordHash, now); err != nil {
				return identity.Principal{}, principalConflict(err)
			}
		}
	}
	return s.Principal(id)
}

func (s *Store) SetPrincipalPassword(id, password string) error {
	if len(password) < 12 {
		return errors.New("password must contain at least 12 characters")
	}
	if err := s.ensureIdentitySchema(); err != nil {
		return err
	}
	principal, err := s.Principal(id)
	if err != nil {
		return err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(timeFormat)
	if _, err := s.db.Exec(`UPDATE principals SET password_hash=?,updated_at=? WHERE id=?`, hash, now, id); err != nil {
		return err
	}
	if principal.Kind == identity.KindUser && principal.ManagementRole != identity.RoleNone {
		_, err = s.db.Exec(`UPDATE users SET password_hash=? WHERE id=?`, hash, id)
	}
	return err
}

func (s *Store) DeletePrincipal(id string) error {
	if err := s.ensureIdentitySchema(); err != nil {
		return err
	}
	principal, err := s.Principal(id)
	if err != nil {
		return err
	}
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM access_rules WHERE principal_id=?`, id).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return errors.New("principal is referenced by access rules")
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM group_members WHERE principal_id=? OR group_id=?`, id, id).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return errors.New("principal is referenced by group membership")
	}
	if principal.ManagementRole != identity.RoleNone {
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM principals WHERE management_role <> 'none' AND enabled=1 AND id<>?`, id).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			return errors.New("cannot delete the last enabled management user")
		}
	}
	if _, err := s.db.Exec(`DELETE FROM principals WHERE id=?`, id); err != nil {
		return err
	}
	_, err = s.db.Exec(`DELETE FROM users WHERE id=?`, id)
	return err
}

func (s *Store) SetGroupMembers(groupID string, memberIDs []string) error {
	if err := s.ensureIdentitySchema(); err != nil {
		return err
	}
	group, err := s.Principal(groupID)
	if err != nil {
		return err
	}
	if group.Kind != identity.KindGroup {
		return errors.New("principal is not a group")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM group_members WHERE group_id=?`, groupID); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, memberID := range memberIDs {
		if seen[memberID] || memberID == groupID {
			return errors.New("group membership contains a duplicate or self-reference")
		}
		seen[memberID] = true
		var kind string
		if err := tx.QueryRow(`SELECT kind FROM principals WHERE id=? AND enabled=1`, memberID).Scan(&kind); err != nil {
			return fmt.Errorf("member %q does not exist or is disabled", memberID)
		}
		if kind == string(identity.KindGroup) {
			return errors.New("nested groups are not supported in the simple access model")
		}
		if _, err := tx.Exec(`INSERT INTO group_members(group_id,principal_id) VALUES(?,?)`, groupID, memberID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) GroupMembers(groupID string) ([]identity.Principal, error) {
	if err := s.ensureIdentitySchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT p.id,p.kind,p.name,p.uid,p.gid,p.management_role,p.enabled,p.created_at,p.updated_at FROM principals p JOIN group_members m ON m.principal_id=p.id WHERE m.group_id=? ORDER BY p.name`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]identity.Principal, 0)
	for rows.Next() {
		principal, err := scanPrincipal(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, principal)
	}
	return result, rows.Err()
}

func (s *Store) groupsForPrincipal(id string) ([]string, error) {
	rows, err := s.db.Query(`SELECT p.name FROM principals p JOIN group_members m ON m.group_id=p.id WHERE m.principal_id=? ORDER BY p.name`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]string, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		result = append(result, name)
	}
	return result, rows.Err()
}

type scanner interface{ Scan(...any) error }

func scanPrincipal(row scanner) (identity.Principal, error) {
	var principal identity.Principal
	var kind, role string
	var uid, gid sql.NullInt64
	var enabled int
	if err := row.Scan(&principal.ID, &kind, &principal.Name, &uid, &gid, &role, &enabled, &principal.CreatedAt, &principal.UpdatedAt); err != nil {
		return identity.Principal{}, err
	}
	principal.Kind = identity.Kind(kind)
	principal.ManagementRole = identity.ManagementRole(role)
	principal.Enabled = enabled != 0
	if uid.Valid {
		principal.UID = &uid.Int64
	}
	if gid.Valid {
		principal.GID = &gid.Int64
	}
	return principal, nil
}

func allocateIdentityID(tx *sql.Tx, kind identity.Kind) (any, any, error) {
	if kind == identity.KindGroup {
		var gid int64
		if err := tx.QueryRow(`SELECT COALESCE(MAX(gid),9999)+1 FROM principals`).Scan(&gid); err != nil {
			return nil, nil, err
		}
		return nil, gid, nil
	}
	var uid int64
	if err := tx.QueryRow(`SELECT COALESCE(MAX(uid),9999)+1 FROM principals`).Scan(&uid); err != nil {
		return nil, nil, err
	}
	return uid, nil, nil
}

func allocateIdentityIDForStore(s *Store) (int64, int64, error) {
	var uid, gid int64
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(uid),9999)+1 FROM principals`).Scan(&uid); err != nil {
		return 0, 0, err
	}
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(gid),9999)+1 FROM principals`).Scan(&gid); err != nil {
		return 0, 0, err
	}
	return uid, gid, nil
}

func principalConflict(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("principal already exists or conflicts with an existing identity: %w", err)
}
