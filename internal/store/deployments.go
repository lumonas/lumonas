package store

import (
	"database/sql"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

const deploymentsSchema = `
CREATE TABLE IF NOT EXISTS docker_deployments (
  id TEXT PRIMARY KEY,
  stack_name TEXT NOT NULL,
  kind TEXT NOT NULL,
  state TEXT NOT NULL,
  compose_before TEXT,
  compose_after TEXT NOT NULL,
  error TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS docker_deployments_stack_idx ON docker_deployments(stack_name, state, updated_at);`

func (s *Store) ensureDeploymentsSchema() error {
	_, err := s.db.Exec(deploymentsSchema)
	return err
}

// CreateDockerDeployment persists a new pending deployment record. Any still
// pending deployment for the same stack is marked superseded first so a stack
// never carries two open transactions.
func (s *Store) CreateDockerDeployment(deployment model.DockerDeployment) error {
	if err := s.ensureDeploymentsSchema(); err != nil {
		return err
	}
	now := time.Now().UTC().Format(timeFormat)
	if _, err := s.db.Exec(`UPDATE docker_deployments SET state='failed',error='superseded by a newer deployment',updated_at=? WHERE stack_name=? AND state='pending'`, now, deployment.StackName); err != nil {
		return err
	}
	_, err := s.db.Exec(`INSERT INTO docker_deployments(id,stack_name,kind,state,compose_before,compose_after,error,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`,
		deployment.ID, deployment.StackName, deployment.Kind, "pending", nullableText(deployment.ComposeBefore), deployment.ComposeAfter, "", now, now)
	return err
}

// UpdateDockerDeployment transitions a deployment to a terminal state.
func (s *Store) UpdateDockerDeployment(id, state, failure string) error {
	if err := s.ensureDeploymentsSchema(); err != nil {
		return err
	}
	switch state {
	case "committed", "rolled_back", "failed":
	default:
		return sql.ErrNoRows
	}
	_, err := s.db.Exec(`UPDATE docker_deployments SET state=?,error=?,updated_at=? WHERE id=?`, state, failure, time.Now().UTC().Format(timeFormat), id)
	return err
}

// PendingDockerDeployments lists deployments the daemon never drove to a
// terminal state, oldest first — the startup rollback queue.
func (s *Store) PendingDockerDeployments() ([]model.DockerDeployment, error) {
	if err := s.ensureDeploymentsSchema(); err != nil {
		return nil, err
	}
	return s.scanDockerDeployments(`SELECT id,stack_name,kind,state,compose_before,compose_after,error,created_at,updated_at FROM docker_deployments WHERE state='pending' ORDER BY created_at ASC`)
}

// DockerDeployments lists recent deployments, newest first, optionally scoped
// to a single stack.
func (s *Store) DockerDeployments(limit int, stackName string) ([]model.DockerDeployment, error) {
	if err := s.ensureDeploymentsSchema(); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	query := `SELECT id,stack_name,kind,state,compose_before,compose_after,error,created_at,updated_at FROM docker_deployments`
	args := []any{}
	if stackName != "" {
		query += ` WHERE stack_name=?`
		args = append(args, stackName)
	}
	query += ` ORDER BY updated_at DESC LIMIT ?`
	args = append(args, limit)
	return s.scanDockerDeployments(query, args...)
}

func (s *Store) scanDockerDeployments(query string, args ...any) ([]model.DockerDeployment, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]model.DockerDeployment, 0)
	for rows.Next() {
		var deployment model.DockerDeployment
		var before sql.NullString
		var created, updated string
		if err := rows.Scan(&deployment.ID, &deployment.StackName, &deployment.Kind, &deployment.State, &before, &deployment.ComposeAfter, &deployment.Error, &created, &updated); err != nil {
			return nil, err
		}
		if before.Valid {
			value := before.String
			deployment.ComposeBefore = &value
		}
		deployment.CreatedAt, _ = parseTime(created)
		deployment.UpdatedAt, _ = parseTime(updated)
		result = append(result, deployment)
	}
	return result, rows.Err()
}

func nullableText(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}
