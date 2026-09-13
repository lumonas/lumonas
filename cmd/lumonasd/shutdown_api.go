package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/power"
	"github.com/lumonas/lumonas/internal/privileged"
)

type upsPolicyRequest struct {
	Enabled           bool    `json:"enabled"`
	MinimumRuntimeSec float64 `json:"minimumRuntimeSec"`
	MinimumCharge     float64 `json:"minimumCharge"`
}

func (s *apiServer) upsStatus(w http.ResponseWriter, r *http.Request) {
	s.ups(w, r)
}

func (s *apiServer) upsPolicy(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	writeJSON(w, http.StatusOK, upsPolicyJSON(s.autoShutdownPolicy()))
}

func (s *apiServer) updateUPSPolicy(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input upsPolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if input.MinimumRuntimeSec < 0 || input.MinimumRuntimeSec > 86400 || input.MinimumCharge < 0 || input.MinimumCharge > 100 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "UPS thresholds are outside the supported range"})
		return
	}
	encoded, _ := json.Marshal(input)
	if err := s.store.SetMeta("ups_shutdown_policy", string(encoded)); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "ups.policy.update", "ups", map[string]any{"enabled": input.Enabled, "minimumRuntimeSec": input.MinimumRuntimeSec, "minimumCharge": input.MinimumCharge})
	s.advanceGeneration("ups.policy.update")
	writeJSON(w, http.StatusOK, upsPolicyJSON(power.ShutdownPolicy{Enabled: input.Enabled, MinimumRuntimeSec: input.MinimumRuntimeSec, MinimumCharge: input.MinimumCharge}))
}

func upsPolicyJSON(policy power.ShutdownPolicy) map[string]any {
	return map[string]any{"enabled": policy.Enabled, "minimumRuntimeSec": policy.MinimumRuntimeSec, "minimumCharge": policy.MinimumCharge}
}

func (s *apiServer) upsMonitorLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		if s.maintenanceModeEnabled() {
			continue
		}
		if !s.autoShutdownPolicy().Enabled {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		for _, unit := range power.Discover(ctx, nil, nil) {
			if power.ShouldShutdown(unit, s.autoShutdownPolicy()) {
				s.publish("ups.shutdown.pending", "critical", nil, map[string]any{"ups": unit.Name, "runtimeSec": unit.RuntimeSec, "chargePercent": unit.ChargePercent})
				s.requestUPSShutdown(unit.Name)
				break
			}
		}
		cancel()
	}
}

func (s *apiServer) maintenanceModeEnabled() bool {
	settings, err := s.loadSettings()
	if err != nil {
		if s.log != nil {
			s.log.Warn("maintenance mode lookup failed", "error", err)
		}
		return false
	}
	powerSettings, ok := settings["power"].(map[string]any)
	if !ok {
		return false
	}
	enabled, _ := powerSettings["maintenanceMode"].(bool)
	return enabled
}

func (s *apiServer) requestUPSShutdown(upsName string) {
	operationID := newID("ups-shutdown")
	result, err := s.executePrivileged(context.Background(), privileged.Request{Operation: "power.shutdown", OperationID: operationID, PlanHash: operationID, RequestedState: map[string]any{"action": "poweroff"}, ExpiresAt: time.Now().UTC().Add(2 * time.Minute), Confirmed: true})
	if err != nil || !result.OK {
		if s.log != nil {
			s.log.Error("UPS shutdown sequence failed", "ups", upsName, "error", err)
		}
		return
	}
	s.publish("ups.shutdown.started", "critical", nil, map[string]any{"ups": upsName, "operationId": operationID})
}

func (s *apiServer) shutdownPlan(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	action := r.URL.Query().Get("action")
	if action == "" {
		action = "poweroff"
	}
	steps, err := power.OrderedShutdown(action)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"action": action, "steps": steps})
}

func (s *apiServer) shutdownPower(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		Action          string `json:"action"`
		Reauthenticated bool   `json:"reauthenticated"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if !input.Reauthenticated {
		writeJSON(w, http.StatusLocked, map[string]string{"error": "reauthentication is required for shutdown"})
		return
	}
	if _, err := power.OrderedShutdown(input.Action); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	operationID := newID("shutdown")
	result, err := s.executePrivileged(r.Context(), privileged.Request{Operation: "power.shutdown", OperationID: operationID, PlanHash: operationID, RequestedState: map[string]any{"action": input.Action}, ExpiresAt: time.Now().UTC().Add(2 * time.Minute), Confirmed: true})
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	if !result.OK {
		writeJSON(w, http.StatusConflict, map[string]string{"error": result.Error})
		return
	}
	s.recordRequestAudit(r, actor, "power.shutdown", operationID, map[string]any{"operationId": operationID, "action": input.Action})
	s.publish("power.shutdown", "critical", nil, map[string]any{"operationId": operationID, "action": input.Action})
	writeJSON(w, http.StatusAccepted, map[string]any{"operationId": operationID, "action": input.Action})
}

func (s *apiServer) autoShutdownPolicy() power.ShutdownPolicy {
	policy := power.ShutdownPolicy{Enabled: strings.EqualFold(envOr("LUMONAS_UPS_AUTO_SHUTDOWN", "false"), "true"), MinimumRuntimeSec: 300, MinimumCharge: 10}
	if encoded, ok := s.store.Meta("ups_shutdown_policy"); ok {
		var configured upsPolicyRequest
		if json.Unmarshal([]byte(encoded), &configured) == nil {
			return power.ShutdownPolicy{Enabled: configured.Enabled, MinimumRuntimeSec: configured.MinimumRuntimeSec, MinimumCharge: configured.MinimumCharge}
		}
	}
	return policy
}
