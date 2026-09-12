package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
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
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS network_bindings (
  service TEXT PRIMARY KEY,
  config_json TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS network_firewall (
  id INTEGER PRIMARY KEY CHECK(id=1),
  config_json TEXT NOT NULL,
  updated_at TEXT NOT NULL
);`

func (s *Store) ensureNetworkSchema() error {
	if _, err := s.db.Exec(networkSchema); err != nil {
		return err
	}
	_, err := s.db.Exec(`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(2, ?)`, time.Now().UTC().Format(timeFormat))
	return err
}

func (s *Store) ListNetworkConnections() ([]network.Connection, error) {
	if err := s.ensureNetworkSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT config_json FROM network_connections ORDER BY name,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]network.Connection, 0)
	for rows.Next() {
		var encoded string
		if err := rows.Scan(&encoded); err != nil {
			return nil, err
		}
		var value network.Connection
		if err := json.Unmarshal([]byte(encoded), &value); err != nil {
			return nil, fmt.Errorf("decode network connection: %w", err)
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *Store) NetworkConnection(id string) (network.Connection, error) {
	if err := s.ensureNetworkSchema(); err != nil {
		return network.Connection{}, err
	}
	var encoded string
	if err := s.db.QueryRow(`SELECT config_json FROM network_connections WHERE id=?`, id).Scan(&encoded); err != nil {
		return network.Connection{}, err
	}
	var value network.Connection
	if err := json.Unmarshal([]byte(encoded), &value); err != nil {
		return network.Connection{}, err
	}
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
	_, err = s.db.Exec(`INSERT INTO network_connections(id,uuid,name,interface,enabled,config_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET uuid=excluded.uuid,name=excluded.name,interface=excluded.interface,enabled=excluded.enabled,config_json=excluded.config_json,updated_at=excluded.updated_at`, value.ID, value.UUID, value.Name, value.Interface, value.Enabled, string(encoded), now, now)
	if err != nil {
		return network.Connection{}, fmt.Errorf("save network connection: %w", err)
	}
	return s.NetworkConnection(value.ID)
}

func (s *Store) ListNetworkBindings() ([]network.Binding, error) {
	if err := s.ensureNetworkSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT config_json FROM network_bindings ORDER BY service`)
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
	if _, err := tx.Exec(`DELETE FROM network_bindings`); err != nil {
		return nil, err
	}
	for _, value := range values {
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(`INSERT INTO network_bindings(service,config_json,updated_at) VALUES(?,?,?)`, value.Service, string(encoded), time.Now().UTC().Format(timeFormat)); err != nil {
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
	if err := s.db.QueryRow(`SELECT config_json FROM network_firewall WHERE id=1`).Scan(&encoded); err == sql.ErrNoRows {
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
	_, err = s.db.Exec(`INSERT INTO network_firewall(id,config_json,updated_at) VALUES(1,?,?) ON CONFLICT(id) DO UPDATE SET config_json=excluded.config_json,updated_at=excluded.updated_at`, string(encoded), time.Now().UTC().Format(timeFormat))
	if err != nil {
		return network.FirewallPolicy{}, err
	}
	return s.NetworkFirewallPolicy()
}
