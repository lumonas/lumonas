package store

import (
	"path/filepath"
	"testing"
)

// TestOpenProvisionsSupplementalSchemas guards the packaged upgrade path.
// These tables used to be created lazily inside their accessors, so
// `lumonas-migrate` (which only calls Open) never provisioned them and an
// upgraded appliance could be missing tables the daemon assumes exist.
func TestOpenProvisionsSupplementalSchemas(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "lumonas.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer database.Close()

	expected := []string{
		"folder_sync_tasks",
		"storage_quotas",
		"storage_quota_status",
		"file_requests",
		"file_content_index",
	}
	for _, table := range expected {
		var name string
		err := database.db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&name)
		if err != nil {
			t.Errorf("schema missing table %q after Open: %v", table, err)
		}
	}

	// The two-factor columns are added to principals rather than a new table.
	for _, column := range []string{totpPendingColumn, totpSecretColumn, totpEnabledColumn, totpRecoveryColumn} {
		found := false
		rows, err := database.db.Query(`PRAGMA table_info(principals)`)
		if err != nil {
			t.Fatalf("inspect principals: %v", err)
		}
		for rows.Next() {
			var cid int
			var name, dataType string
			var notNull, primaryKey int
			var defaultValue any
			if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
				rows.Close()
				t.Fatalf("scan principals column: %v", err)
			}
			if name == column {
				found = true
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			t.Fatalf("iterate principals: %v", err)
		}
		rows.Close()
		if !found {
			t.Errorf("principals is missing %q after Open", column)
		}
	}
}
