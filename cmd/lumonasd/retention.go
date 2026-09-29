package main

import (
	"strconv"
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
	days := 365
	if value, ok := s.store.Meta("audit_retention_days"); ok {
		if parsed, err := strconv.Atoi(value); err == nil && parsed >= 30 && parsed <= 3650 {
			days = parsed
		}
	}
	if err := s.store.PruneAuditBefore(time.Now().UTC().AddDate(0, 0, -days)); err != nil && s.log != nil {
		s.log.Warn("audit age retention failed", "error", err)
	}
}
