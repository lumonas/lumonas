package store

import (
	"database/sql"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

const alertsSchema = `
CREATE TABLE IF NOT EXISTS alert_acks (
  alert_id TEXT PRIMARY KEY,
  acknowledged_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS generated_alerts (
  id TEXT PRIMARY KEY,
  rule_id TEXT NOT NULL,
  severity TEXT NOT NULL,
  title TEXT NOT NULL,
  description TEXT NOT NULL,
  resource_type TEXT,
  resource_id TEXT,
  state TEXT NOT NULL,
  started_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS generated_alerts_rule_idx ON generated_alerts(rule_id, resource_id, state);`

func (s *Store) ensureAlertsSchema() error {
	_, err := s.db.Exec(alertsSchema)
	return err
}

// AckAlert persists an acknowledgement for any alert id (derived or generated).
func (s *Store) AckAlert(id string) error {
	if err := s.ensureAlertsSchema(); err != nil {
		return err
	}
	if _, err := s.db.Exec(`INSERT INTO alert_acks(alert_id,acknowledged_at) VALUES(?,?) ON CONFLICT(alert_id) DO NOTHING`, id, time.Now().UTC().Format(timeFormat)); err != nil {
		return err
	}
	_, err := s.db.Exec(`UPDATE generated_alerts SET state='acknowledged',updated_at=? WHERE id=?`, time.Now().UTC().Format(timeFormat), id)
	return err
}

// AcknowledgedAlerts returns the set of acknowledged alert ids.
func (s *Store) AcknowledgedAlerts() (map[string]bool, error) {
	if err := s.ensureAlertsSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT alert_id FROM alert_acks`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		result[id] = true
	}
	return result, rows.Err()
}

// OpenGeneratedAlert inserts a rule-fired alert unless an open alert for the
// same rule and resource already exists. Reports whether it was inserted.
func (s *Store) OpenGeneratedAlert(alert model.Alert, ruleID string) (bool, error) {
	if err := s.ensureAlertsSchema(); err != nil {
		return false, err
	}
	resourceID := ""
	if alert.Resource != nil {
		resourceID = alert.Resource.ID
	}
	resourceType := ""
	if alert.Resource != nil {
		resourceType = alert.Resource.Type
	}
	var existing string
	err := s.db.QueryRow(`SELECT id FROM generated_alerts WHERE rule_id=? AND COALESCE(resource_id,'')=? AND state IN ('firing','acknowledged') LIMIT 1`, ruleID, resourceID).Scan(&existing)
	if err == nil {
		return false, nil
	}
	if err != sql.ErrNoRows {
		return false, err
	}
	_, err = s.db.Exec(`INSERT INTO generated_alerts(id,rule_id,severity,title,description,resource_type,resource_id,state,started_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		alert.ID, ruleID, alert.Severity, alert.Title, alert.Description, nullable(resourceType), nullable(resourceID), "firing", alert.StartedAt.Format(timeFormat), time.Now().UTC().Format(timeFormat))
	if err != nil {
		return false, err
	}
	return true, nil
}

// GeneratedAlerts lists rule-fired alerts that have not been resolved.
func (s *Store) GeneratedAlerts() ([]model.Alert, error) {
	if err := s.ensureAlertsSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT id,severity,title,description,COALESCE(resource_type,''),COALESCE(resource_id,''),state,started_at FROM generated_alerts WHERE state IN ('firing','acknowledged') ORDER BY started_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]model.Alert, 0)
	for rows.Next() {
		var alert model.Alert
		var resourceType, resourceID string
		var started string
		if err := rows.Scan(&alert.ID, &alert.Severity, &alert.Title, &alert.Description, &resourceType, &resourceID, &alert.State, &started); err != nil {
			return nil, err
		}
		if resourceType != "" {
			alert.Resource = &model.ResourceRef{Type: resourceType, ID: resourceID}
		}
		alert.StartedAt, _ = parseTime(started)
		result = append(result, alert)
	}
	return result, rows.Err()
}

// GeneratedAlertHistory lists resolved rule-fired alerts, most recently
// resolved first, so operators can see what fired and when it cleared.
func (s *Store) GeneratedAlertHistory(limit int) ([]model.Alert, error) {
	if err := s.ensureAlertsSchema(); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(`SELECT id,severity,title,description,COALESCE(resource_type,''),COALESCE(resource_id,''),state,started_at,updated_at FROM generated_alerts WHERE state='resolved' ORDER BY updated_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]model.Alert, 0)
	for rows.Next() {
		var alert model.Alert
		var resourceType, resourceID string
		var started, updated string
		if err := rows.Scan(&alert.ID, &alert.Severity, &alert.Title, &alert.Description, &resourceType, &resourceID, &alert.State, &started, &updated); err != nil {
			return nil, err
		}
		if resourceType != "" {
			alert.Resource = &model.ResourceRef{Type: resourceType, ID: resourceID}
		}
		alert.StartedAt, _ = parseTime(started)
		if resolvedAt, err := parseTime(updated); err == nil {
			alert.ResolvedAt = &resolvedAt
		}
		result = append(result, alert)
	}
	return result, rows.Err()
}

// ResolveGeneratedAlerts marks open alerts for the rule/resource as resolved
// and reports whether anything changed.
func (s *Store) ResolveGeneratedAlerts(ruleID, resourceID string) (bool, error) {
	if err := s.ensureAlertsSchema(); err != nil {
		return false, err
	}
	result, err := s.db.Exec(`UPDATE generated_alerts SET state='resolved',updated_at=? WHERE rule_id=? AND COALESCE(resource_id,'')=? AND state IN ('firing','acknowledged')`, time.Now().UTC().Format(timeFormat), ruleID, resourceID)
	if err != nil {
		return false, err
	}
	count, _ := result.RowsAffected()
	return count > 0, nil
}
