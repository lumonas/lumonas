package main

import (
	"net/http"
	"strconv"
	"time"

	"github.com/lumonas/lumonas/internal/capacity"
)

func (s *apiServer) capacityForecast(w http.ResponseWriter, r *http.Request) {
	windowDays := 30
	if value := r.URL.Query().Get("days"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 366 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "days must be between 1 and 366"})
			return
		}
		windowDays = parsed
	}
	since := time.Now().UTC().Add(-time.Duration(windowDays) * 24 * time.Hour)
	resourceID := r.URL.Query().Get("resourceId")
	resources := []string{resourceID}
	if resourceID == "" {
		var err error
		resources, err = s.store.CapacityResources(since)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}
	result := make([]capacity.Forecast, 0, len(resources))
	for _, resource := range resources {
		snapshots, err := s.store.CapacitySnapshots(resource, since, 366)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		result = append(result, capacity.Build(resource, snapshots, time.Now().UTC()))
	}
	writeJSON(w, http.StatusOK, result)
}
