package main

import (
	"time"
)

const operationalRetentionInterval = 15 * time.Minute

func (s *apiServer) retentionLoop() {
	s.pruneOperationalHistory()
	ticker := time.NewTicker(operationalRetentionInterval)
	defer ticker.Stop()
	for range ticker.C {
		s.pruneOperationalHistory()
	}
}

func (s *apiServer) pruneOperationalHistory() {
	if err := s.store.PruneOperationalHistory(time.Now().UTC()); err != nil && s.log != nil {
		s.log.Warn("operational history retention failed", "error", err)
	}
}
