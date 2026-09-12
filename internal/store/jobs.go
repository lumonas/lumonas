package store

const defaultJobRetention = 1000

// PruneJobs keeps active work visible while bounding terminal job history.
// Queued and running jobs are never removed by retention.
func (s *Store) PruneJobs(keep int) error {
	if keep < 100 {
		keep = 100
	}
	_, err := s.db.Exec(`DELETE FROM jobs
WHERE state IN ('completed','failed','canceled')
  AND id NOT IN (
    SELECT id FROM jobs
    WHERE state IN ('completed','failed','canceled')
    ORDER BY created_at DESC
    LIMIT ?
  )`, keep)
	return err
}
