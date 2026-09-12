package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/network"
)

const networkSchema = `
CREATE TABLE IF NOT EXISTS network_connections (
  id TEXT PRIMARY KEY,
  uuid TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL,
  interface TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  config_json TEXT NOT NULL,
  generation INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'configured',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS service_bindings (
  service TEXT PRIMARY KEY,
  config_json TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS firewall_policies (
  id INTEGER PRIMARY KEY CHECK(id=1),
  config_json TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS network_diagnostic_results (
  job_id TEXT PRIMARY KEY REFERENCES jobs(id) ON DELETE CASCADE,
  result_json TEXT NOT NULL,
  updated_at TEXT NOT NULL
);`

func (s *Store) ensureNetworkSchema() error {
	if _, err := s.db.Exec(networkSchema); err != nil {
		return err
	}
	if err := s.ensureNetworkColumn("network_connections", "generation", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.ensureNetworkColumn("network_connections", "status", "TEXT NOT NULL DEFAULT 'configured'"); err != nil {
		return err
	}
	// Older development builds used these internal table names. Preserve their
	// data while making the version-2 names canonical.
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO service_bindings(service,config_json,updated_at) SELECT service,config_json,updated_at FROM network_bindings`); err != nil && !isMissingTable(err) {
		return err
	}
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO firewall_policies(id,config_json,updated_at) SELECT id,config_json,updated_at FROM network_firewall`); err != nil && !isMissingTable(err) {
		return err
	}
	_, err := s.db.Exec(`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(2, ?)`, time.Now().UTC().Format(timeFormat))
	return err
}

func (s *Store) ensureNetworkColumn(table, column, definition string) error {
	rows, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = s.db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` ` + definition)
	return err
}

func isMissingTable(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "no such table")
}

func (s *Store) ListNetworkConnections() ([]network.Connection, error) {
	if err := s.ensureNetworkSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT config_json,generation,status FROM network_connections ORDER BY name,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]network.Connection, 0)
	for rows.Next() {
		var encoded, status string
		var generation int64
		if err := rows.Scan(&encoded, &generation, &status); err != nil {
			return nil, err
		}
		var value network.Connection
		if err := json.Unmarshal([]byte(encoded), &value); err != nil {
			return nil, fmt.Errorf("decode network connection: %w", err)
		}
		value.Generation, value.Status = generation, status
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *Store) NetworkConnection(id string) (network.Connection, error) {
	if err := s.ensureNetworkSchema(); err != nil {
		return network.Connection{}, err
	}
	var encoded, status string
	var generation int64
	if err := s.db.QueryRow(`SELECT config_json,generation,status FROM network_connections WHERE id=?`, id).Scan(&encoded, &generation, &status); err != nil {
		return network.Connection{}, err
	}
	var value network.Connection
	if err := json.Unmarshal([]byte(encoded), &value); err != nil {
		return network.Connection{}, err
	}
	value.Generation, value.Status = generation, status
	return value, nil
}

func (s *Store) UpsertNetworkConnection(value network.Connection) (network.Connection, error) {
	if err := value.Validate(); err != nil {
		return network.Connection{}, err
	}
	if err := s.ensureNetworkSchema(); err != nil {
		return network.Connection{}, err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return network.Connection{}, err
	}
	now := time.Now().UTC().Format(timeFormat)
	_, err = s.db.Exec(`INSERT INTO network_connections(id,uuid,name,interface,enabled,config_json,generation,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET uuid=excluded.uuid,name=excluded.name,interface=excluded.interface,enabled=excluded.enabled,config_json=excluded.config_json,generation=excluded.generation,status=excluded.status,updated_at=excluded.updated_at`, value.ID, value.UUID, value.Name, value.Interface, value.Enabled, string(encoded), value.Generation, value.Status, now, now)
	if err != nil {
		return network.Connection{}, fmt.Errorf("save network connection: %w", err)
	}
	return s.NetworkConnection(value.ID)
}

func (s *Store) ListNetworkBindings() ([]network.Binding, error) {
	if err := s.ensureNetworkSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT config_json FROM service_bindings ORDER BY service`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]network.Binding, 0)
	for rows.Next() {
		var encoded string
		var value network.Binding
		if err := rows.Scan(&encoded); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(encoded), &value); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return network.DefaultBindings(), nil
	}
	return result, nil
}

func (s *Store) ReplaceNetworkBindings(values []network.Binding) ([]network.Binding, error) {
	if err := network.ValidateBindings(values); err != nil {
		return nil, err
	}
	if err := s.ensureNetworkSchema(); err != nil {
		return nil, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM service_bindings`); err != nil {
		return nil, err
	}
	for _, value := range values {
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(`INSERT INTO service_bindings(service,config_json,updated_at) VALUES(?,?,?)`, value.Service, string(encoded), time.Now().UTC().Format(timeFormat)); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.ListNetworkBindings()
}

func (s *Store) NetworkFirewallPolicy() (network.FirewallPolicy, error) {
	if err := s.ensureNetworkSchema(); err != nil {
		return network.FirewallPolicy{}, err
	}
	var encoded string
	if err := s.db.QueryRow(`SELECT config_json FROM firewall_policies WHERE id=1`).Scan(&encoded); err == sql.ErrNoRows {
		return network.DefaultFirewallPolicy(), nil
	} else if err != nil {
		return network.FirewallPolicy{}, err
	}
	var value network.FirewallPolicy
	if err := json.Unmarshal([]byte(encoded), &value); err != nil {
		return network.FirewallPolicy{}, err
	}
	return value, nil
}

func (s *Store) SaveNetworkFirewallPolicy(value network.FirewallPolicy) (network.FirewallPolicy, error) {
	if err := value.Validate(); err != nil {
		return network.FirewallPolicy{}, err
	}
	if err := s.ensureNetworkSchema(); err != nil {
		return network.FirewallPolicy{}, err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return network.FirewallPolicy{}, err
	}
	_, err = s.db.Exec(`INSERT INTO firewall_policies(id,config_json,updated_at) VALUES(1,?,?) ON CONFLICT(id) DO UPDATE SET config_json=excluded.config_json,updated_at=excluded.updated_at`, string(encoded), time.Now().UTC().Format(timeFormat))
	if err != nil {
		return network.FirewallPolicy{}, err
	}
	return s.NetworkFirewallPolicy()
}

func (s *Store) SaveNetworkDiagnosticResult(jobID string, result any) error {
	if err := s.ensureNetworkSchema(); err != nil {
		return err
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO network_diagnostic_results(job_id,result_json,updated_at) VALUES(?,?,?) ON CONFLICT(job_id) DO UPDATE SET result_json=excluded.result_json,updated_at=excluded.updated_at`, jobID, string(encoded), time.Now().UTC().Format(timeFormat))
	return err
}

func (s *Store) NetworkDiagnosticResult(jobID string) (any, error) {
	if err := s.ensureNetworkSchema(); err != nil {
		return nil, err
	}
	var encoded string
	if err := s.db.QueryRow(`SELECT result_json FROM network_diagnostic_results WHERE job_id=?`, jobID).Scan(&encoded); err != nil {
		return nil, err
	}
	var result any
	if err := json.Unmarshal([]byte(encoded), &result); err != nil {
		return nil, err
	}
	return result, nil
}
