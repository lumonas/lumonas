package store

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/lumonas/lumonas/internal/storage"
)

func (s *Store) SavePoolPlan(plan storage.PoolPlan) error {
	payload, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO storage_operations(operation_id,plan_hash,status,expires_at,plan_json,created_at) VALUES(?,?,?,?,?,?) ON CONFLICT(operation_id) DO UPDATE SET plan_hash=excluded.plan_hash,status=excluded.status,expires_at=excluded.expires_at,plan_json=excluded.plan_json`, plan.OperationID, plan.PlanHash, plan.Status, plan.ExpiresAt.Format(timeFormat), string(payload), time.Now().UTC().Format(timeFormat))
	return err
}

func (s *Store) PoolPlan(operationID string) (storage.PoolPlan, error) {
	var payload string
	if err := s.db.QueryRow(`SELECT plan_json FROM storage_operations WHERE operation_id = ?`, operationID).Scan(&payload); err != nil {
		return storage.PoolPlan{}, err
	}
	var plan storage.PoolPlan
	if err := json.Unmarshal([]byte(payload), &plan); err != nil {
		return storage.PoolPlan{}, err
	}
	return plan, nil
}

func (s *Store) SavePoolUnmountPlan(plan storage.PoolUnmountPlan) error {
	payload, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO storage_operations(operation_id,plan_hash,status,expires_at,plan_json,created_at) VALUES(?,?,?,?,?,?) ON CONFLICT(operation_id) DO UPDATE SET plan_hash=excluded.plan_hash,status=excluded.status,expires_at=excluded.expires_at,plan_json=excluded.plan_json`, plan.OperationID, plan.PlanHash, plan.Status, plan.ExpiresAt.Format(timeFormat), string(payload), time.Now().UTC().Format(timeFormat))
	return err
}

func (s *Store) PoolUnmountPlan(operationID string) (storage.PoolUnmountPlan, error) {
	var payload string
	if err := s.db.QueryRow(`SELECT plan_json FROM storage_operations WHERE operation_id = ?`, operationID).Scan(&payload); err != nil {
		return storage.PoolUnmountPlan{}, err
	}
	var plan storage.PoolUnmountPlan
	if err := json.Unmarshal([]byte(payload), &plan); err != nil {
		return storage.PoolUnmountPlan{}, err
	}
	return plan, nil
}

func IsMissingPoolPlan(err error) bool { return err == sql.ErrNoRows }
