package store

import (
	"encoding/json"
	"time"

	"github.com/lumonas/lumonas/internal/storage"
)

func (s *Store) SaveReplacementPlan(plan storage.ReplacementPlan) error {
	payload, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO storage_operations(operation_id,plan_hash,status,expires_at,plan_json,created_at) VALUES(?,?,?,?,?,?) ON CONFLICT(operation_id) DO UPDATE SET plan_hash=excluded.plan_hash,status=excluded.status,expires_at=excluded.expires_at,plan_json=excluded.plan_json`, plan.OperationID, plan.PlanHash, plan.Status, plan.ExpiresAt.Format(timeFormat), string(payload), time.Now().UTC().Format(timeFormat))
	return err
}

func (s *Store) ReplacementPlan(operationID string) (storage.ReplacementPlan, error) {
	var payload string
	if err := s.db.QueryRow(`SELECT plan_json FROM storage_operations WHERE operation_id = ?`, operationID).Scan(&payload); err != nil {
		return storage.ReplacementPlan{}, err
	}
	var plan storage.ReplacementPlan
	if err := json.Unmarshal([]byte(payload), &plan); err != nil {
		return storage.ReplacementPlan{}, err
	}
	return plan, nil
}
