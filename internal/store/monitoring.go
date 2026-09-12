package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lumonas/lumonas/internal/monitoring"
)

const monitoringSchema = `
CREATE TABLE IF NOT EXISTS alert_rules (
  id TEXT PRIMARY KEY,
  config_json TEXT NOT NULL,
  updated_at TEXT NOT NULL
);`

func (s *Store) ensureMonitoringSchema() error {
	_, err := s.db.Exec(monitoringSchema)
	return err
}

func (s *Store) AlertRules() ([]monitoring.AlertRule, error) {
	if err := s.ensureMonitoringSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT config_json FROM alert_rules ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]monitoring.AlertRule, 0)
	for rows.Next() {
		var encoded string
		var rule monitoring.AlertRule
		if err := rows.Scan(&encoded); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(encoded), &rule); err != nil {
			return nil, err
		}
		result = append(result, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(result) == 0 {
		result = monitoring.DefaultAlertRules()
		for _, rule := range result {
			if err := s.SaveAlertRule(rule); err != nil {
				return nil, err
			}
		}
	}
	return result, nil
}

func (s *Store) AlertRule(id string) (monitoring.AlertRule, error) {
	rules, err := s.AlertRules()
	if err != nil {
		return monitoring.AlertRule{}, err
	}
	for _, rule := range rules {
		if rule.ID == id {
			return rule, nil
		}
	}
	return monitoring.AlertRule{}, sql.ErrNoRows
}

func (s *Store) SaveAlertRule(rule monitoring.AlertRule) error {
	if rule.ID == "" || rule.Name == "" || rule.Severity == "" {
		return fmt.Errorf("invalid alert rule")
	}
	if err := s.ensureMonitoringSchema(); err != nil {
		return err
	}
	encoded, err := json.Marshal(rule)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO alert_rules(id,config_json,updated_at) VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET config_json=excluded.config_json,updated_at=excluded.updated_at`, rule.ID, string(encoded), time.Now().UTC().Format(timeFormat))
	return err
}
