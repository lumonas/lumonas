package store

import "time"

const defaultOperationalRetention = 1000

func retentionLimit(keep int) int {
	if keep < 100 {
		return 100
	}
	return keep
}

// PruneNotificationDeliveries bounds delivery history without touching the
// configured notification channels.
func (s *Store) PruneNotificationDeliveries(keep int) error {
	keep = retentionLimit(keep)
	_, err := s.db.Exec(`DELETE FROM notification_deliveries
WHERE id NOT IN (SELECT id FROM notification_deliveries ORDER BY attempted_at DESC LIMIT ?)`, keep)
	return err
}

// PruneNotificationFailures removes stale suppression rows for channels that
// no longer exist. Active cooldowns are bounded by the channel/event primary
// key and are intentionally retained across restarts.
func (s *Store) PruneNotificationFailures() error {
	_, err := s.db.Exec(`DELETE FROM notification_failures WHERE channel_id NOT IN (SELECT id FROM notification_channels)`)
	return err
}

// PruneExpiredSessions removes only sessions that can no longer authenticate.
func (s *Store) PruneExpiredSessions(now time.Time) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE expires_at <= ?`, now.UTC().Format(timeFormat))
	return err
}

// PruneBackupRuns removes only finished runs outside the retained history.
// Cascading foreign keys remove their copies and verification records.
func (s *Store) PruneBackupRuns(keep int) error {
	keep = retentionLimit(keep)
	_, err := s.db.Exec(`DELETE FROM backup_runs
WHERE finished_at IS NOT NULL
  AND id NOT IN (
    SELECT id FROM backup_runs
    WHERE finished_at IS NOT NULL
    ORDER BY started_at DESC
    LIMIT ?
  )`, keep)
	return err
}

// PruneStorageOperations removes expired plans outside the retained window.
// Unexpired plans remain available for confirmation even when they are older
// than the normal history window.
func (s *Store) PruneStorageOperations(now time.Time, keep int) error {
	keep = retentionLimit(keep)
	_, err := s.db.Exec(`DELETE FROM storage_operations
WHERE expires_at <= ?
  AND operation_id NOT IN (
    SELECT operation_id FROM storage_operations
    WHERE expires_at <= ?
    ORDER BY created_at DESC
    LIMIT ?
  )`, now.UTC().Format(timeFormat), now.UTC().Format(timeFormat), keep)
	return err
}

// PruneNetworkCheckpoints removes completed checkpoint history while keeping
// pending/active rollback state regardless of age.
func (s *Store) PruneNetworkCheckpoints(keep int) error {
	keep = retentionLimit(keep)
	_, err := s.db.Exec(`DELETE FROM network_checkpoints
WHERE state NOT IN ('pending','active')
  AND operation_id NOT IN (
    SELECT operation_id FROM network_checkpoints
    WHERE state NOT IN ('pending','active')
    ORDER BY updated_at DESC
    LIMIT ?
  )`, keep)
	return err
}

// PruneGeneratedAlerts bounds resolved alert history while preserving alerts
// that are still visible to operators or awaiting resolution.
func (s *Store) PruneGeneratedAlerts(keep int) error {
	keep = retentionLimit(keep)
	_, err := s.db.Exec(`DELETE FROM generated_alerts
WHERE state NOT IN ('firing','acknowledged')
  AND id NOT IN (
    SELECT id FROM generated_alerts
    WHERE state NOT IN ('firing','acknowledged')
    ORDER BY updated_at DESC
    LIMIT ?
  )`, keep)
	return err
}

// PruneDockerDeployments bounds terminal deployment history while preserving
// pending transactions for startup reconciliation.
func (s *Store) PruneDockerDeployments(keep int) error {
	keep = retentionLimit(keep)
	_, err := s.db.Exec(`DELETE FROM docker_deployments
WHERE state <> 'pending'
  AND id NOT IN (
    SELECT id FROM docker_deployments
    WHERE state <> 'pending'
    ORDER BY updated_at DESC
    LIMIT ?
  )`, keep)
	return err
}

// PruneOperationalHistory applies one bounded policy to operational tables.
// It is safe to call at startup and periodically while the daemon is running.
func (s *Store) PruneOperationalHistory(now time.Time) error {
	// Keep the scheduled pass authoritative even for rows created by recovery,
	// migrations, or administrative tooling that bypasses normal write helpers.
	if err := s.PruneEvents(10000); err != nil {
		return err
	}
	if err := s.PruneAudit(10000); err != nil {
		return err
	}
	if err := s.PruneJobs(defaultJobRetention); err != nil {
		return err
	}
	if err := s.PruneConfigGenerations(defaultOperationalRetention); err != nil {
		return err
	}
	if err := s.PruneExpiredSessions(now); err != nil {
		return err
	}
	if err := s.PruneNotificationDeliveries(defaultOperationalRetention); err != nil {
		return err
	}
	if err := s.PruneNotificationFailures(); err != nil {
		return err
	}
	if err := s.PruneBackupRuns(defaultOperationalRetention); err != nil {
		return err
	}
	if err := s.PruneStorageOperations(now, defaultOperationalRetention); err != nil {
		return err
	}
	if err := s.PruneNetworkCheckpoints(defaultOperationalRetention); err != nil {
		return err
	}
	if err := s.PruneGeneratedAlerts(defaultOperationalRetention); err != nil {
		return err
	}
	if err := s.PruneDockerDeployments(defaultOperationalRetention); err != nil {
		return err
	}
	if err := s.PruneCapacitySnapshots(now.UTC().Add(-180 * 24 * time.Hour)); err != nil {
		return err
	}
	return s.PruneSystemMetricSamples(now.UTC().Add(-7 * 24 * time.Hour))
}
