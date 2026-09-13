package store

// PruneConfigGenerations bounds committed configuration history while keeping
// pending generations available for recovery and diagnosis.
func (s *Store) PruneConfigGenerations(keep int) error {
	keep = retentionLimit(keep)
	_, err := s.db.Exec(`DELETE FROM config_generations
WHERE state = 'committed'
  AND generation NOT IN (
    SELECT generation FROM config_generations
    WHERE state = 'committed'
    ORDER BY generation DESC
    LIMIT ?
  )`, keep)
	return err
}
