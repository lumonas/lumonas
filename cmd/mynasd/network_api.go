package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/network"
)

func (s *apiServer) listNetworkConnections(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	values, err := s.store.ListNetworkConnections()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, values)
}

func (s *apiServer) updateNetworkConnection(w http.ResponseWriter, r *http.Request, id string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		network.Connection
		ExpectedGeneration *int64 `json:"expectedGeneration"`
		Reauthenticated    bool   `json:"reauthenticated"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if !s.expectedIdentityGeneration(w, input.ExpectedGeneration) {
		return
	}
	input.ID = id
	if err := input.Connection.Validate(); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	if !input.Reauthenticated {
		writeJSON(w, http.StatusLocked, map[string]string{"error": "reauthentication is required for network changes"})
		return
	}
	value, err := s.store.UpsertNetworkConnection(input.Connection)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	s.recordIdentityAudit(actor, "network.connection.update", id, map[string]any{"interface": value.Interface})
	s.advanceGeneration("network.connection.update")
	s.publish("network.connection.updated", "warning", &model.ResourceRef{Type: "network-connection", ID: id}, map[string]any{"requiresCheckpoint": true})
	writeJSON(w, http.StatusOK, value)
}

func (s *apiServer) listNetworkBindings(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	values, err := s.store.ListNetworkBindings()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, values)
}

func (s *apiServer) updateNetworkBindings(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		Bindings           []network.Binding `json:"bindings"`
		ExpectedGeneration *int64            `json:"expectedGeneration"`
		Reauthenticated    bool              `json:"reauthenticated"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if !s.expectedIdentityGeneration(w, input.ExpectedGeneration) {
		return
	}
	if !input.Reauthenticated {
		writeJSON(w, http.StatusLocked, map[string]string{"error": "reauthentication is required for network changes"})
		return
	}
	values, err := s.store.ReplaceNetworkBindings(input.Bindings)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	s.recordIdentityAudit(actor, "network.binding.update", "bindings", nil)
	s.advanceGeneration("network.binding.update")
	s.publish("network.binding.updated", "warning", nil, map[string]any{"requiresFirewallRegeneration": true})
	writeJSON(w, http.StatusOK, values)
}

func (s *apiServer) getNetworkFirewall(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	value, err := s.store.NetworkFirewallPolicy()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *apiServer) updateNetworkFirewall(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		network.FirewallPolicy
		ExpectedGeneration *int64 `json:"expectedGeneration"`
		Reauthenticated    bool   `json:"reauthenticated"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if !s.expectedIdentityGeneration(w, input.ExpectedGeneration) {
		return
	}
	if !input.Reauthenticated {
		writeJSON(w, http.StatusLocked, map[string]string{"error": "reauthentication is required for firewall changes"})
		return
	}
	value, err := s.store.SaveNetworkFirewallPolicy(input.FirewallPolicy)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	s.recordIdentityAudit(actor, "network.firewall.update", "firewall", nil)
	s.advanceGeneration("network.firewall.update")
	s.publish("network.firewall.updated", "warning", nil, nil)
	writeJSON(w, http.StatusOK, value)
}

func (s *apiServer) networkDiagnostic(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	var input struct {
		Kind   string `json:"kind"`
		Target string `json:"target"`
		Port   int    `json:"port"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	switch input.Kind {
	case "interfaces":
		values, err := network.Interfaces()
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"kind": input.Kind, "result": values})
	case "dns-lookup":
		if strings.TrimSpace(input.Target) == "" || len(input.Target) > 253 || strings.ContainsAny(input.Target, " /\\") {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "DNS target is invalid"})
			return
		}
		values, err := net.DefaultResolver.LookupHost(ctx, input.Target)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "DNS lookup failed"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"kind": input.Kind, "target": input.Target, "result": values})
	case "port-test":
		if strings.TrimSpace(input.Target) == "" || strings.ContainsAny(input.Target, " /\\") || input.Port < 1 || input.Port > 65535 {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "port test target or port is invalid"})
			return
		}
		conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", input.Target, input.Port))
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "port test failed"})
			return
		}
		_ = conn.Close()
		writeJSON(w, http.StatusOK, map[string]any{"kind": input.Kind, "target": input.Target, "port": input.Port, "reachable": true})
	default:
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "unsupported diagnostic kind"})
	}
}
