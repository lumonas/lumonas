package main

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/store"
)

func hasAuditFilters(r *http.Request) bool {
	for _, key := range []string{"actor", "action", "outcome", "resourceType", "q", "from", "to"} {
		if r.URL.Query().Get(key) != "" {
			return true
		}
	}
	return false
}

func parseAuditFilters(r *http.Request) (store.AuditFilters, error) {
	query := r.URL.Query()
	filters := store.AuditFilters{
		Actor: strings.TrimSpace(query.Get("actor")), Action: strings.TrimSpace(query.Get("action")),
		Outcome: strings.TrimSpace(query.Get("outcome")), ResourceType: strings.TrimSpace(query.Get("resourceType")),
		Query: strings.TrimSpace(query.Get("q")),
	}
	if len(filters.Actor) > 256 || len(filters.Action) > 256 || len(filters.Outcome) > 128 || len(filters.ResourceType) > 128 || len(filters.Query) > 256 {
		return filters, errors.New("audit filter is too long")
	}
	for key, target := range map[string]*time.Time{"from": &filters.From, "to": &filters.To} {
		if raw := query.Get(key); raw != "" {
			parsed, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				return filters, errors.New("audit from and to filters must be RFC3339 timestamps")
			}
			*target = parsed.UTC()
		}
	}
	if !filters.From.IsZero() && !filters.To.IsZero() && filters.From.After(filters.To) {
		return filters, errors.New("audit from timestamp must be before to timestamp")
	}
	return filters, nil
}

func safeCSV(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "'" + value
	}
	return value
}
