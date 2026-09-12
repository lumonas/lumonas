package store

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/lumonas/lumonas/internal/identity"
)

// ResolveAccess returns the strongest rule inherited by a principal through
// direct assignment and its groups. A missing rule is intentionally none.
func (s *Store) ResolveAccess(resourceType, resourceID, principalID string) (identity.AccessLevel, error) {
	if err := s.ensureIdentitySchema(); err != nil {
		return identity.AccessNone, err
	}
	principal, err := s.Principal(principalID)
	if err != nil {
		return identity.AccessNone, err
	}
	if !principal.Enabled {
		return identity.AccessNone, nil
	}
	ids := []string{principal.ID}
	rows, err := s.db.Query(`SELECT group_id FROM group_members WHERE principal_id=?`, principal.ID)
	if err != nil {
		return identity.AccessNone, err
	}
	for rows.Next() {
		var groupID string
		if err := rows.Scan(&groupID); err != nil {
			rows.Close()
			return identity.AccessNone, err
		}
		ids = append(ids, groupID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return identity.AccessNone, err
	}
	rows.Close()
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, 2+len(ids))
	args = append(args, resourceType, resourceID)
	for _, id := range ids {
		args = append(args, id)
	}
	query := `SELECT level FROM access_rules WHERE resource_type=? AND resource_id=? AND principal_id IN (` + placeholders + `)`
	rows, err = s.db.Query(query, args...)
	if err != nil {
		return identity.AccessNone, err
	}
	defer rows.Close()
	best := identity.AccessNone
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return identity.AccessNone, err
		}
		level := identity.AccessLevel(value)
		if accessRank(level) > accessRank(best) {
			best = level
		}
	}
	if err := rows.Err(); err != nil {
		return identity.AccessNone, err
	}
	return best, nil
}

func (s *Store) ResolveShareAccess(shareID, principalID string) (identity.AccessLevel, error) {
	return s.ResolveAccess("share", shareID, principalID)
}

func CanAccess(level, required identity.AccessLevel) bool {
	if level == identity.AccessNone || required == identity.AccessNone {
		return required == identity.AccessNone
	}
	return accessRank(level) >= accessRank(required)
}

func accessRank(level identity.AccessLevel) int {
	switch level {
	case identity.AccessWrite:
		return 2
	case identity.AccessRead:
		return 1
	case identity.AccessNone:
		return 0
	default:
		return -1
	}
}

func requirePrincipal(s *Store, id string) error {
	if _, err := s.Principal(id); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("principal %q was not found", id)
		}
		return err
	}
	return nil
}
