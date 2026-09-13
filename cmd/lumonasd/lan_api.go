package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/network"
	"github.com/lumonas/lumonas/internal/privileged"
	"github.com/lumonas/lumonas/internal/runner"
	"github.com/lumonas/lumonas/internal/store"
)

var lanHostnamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

// lanScanFunc observes the kernel neighbor table; tests replace it.
func (s *apiServer) defaultLanScan(ctx context.Context) ([]network.LanHost, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := runner.OutputContext(ctx, "ip", "neigh", "show")
	if err != nil {
		return nil, err
	}
	return network.ParseNeighbors(string(out)), nil
}

// lanScan observes the neighbor table and records fresh observations without
// losing operator-set hostnames.
func (s *apiServer) lanScan(ctx context.Context) ([]store.LanHostRecord, error) {
	scan := s.lanScanImpl
	if scan == nil {
		scan = s.defaultLanScan
	}
	hosts, err := scan(ctx)
	if err != nil {
		return nil, err
	}
	records := make([]store.LanHostRecord, 0, len(hosts))
	for _, host := range hosts {
		records = append(records, store.LanHostRecord{MAC: host.MAC, Interface: host.Interface, IP: host.IP})
	}
	if err := s.store.UpsertLanHosts(records); err != nil {
		return nil, err
	}
	return s.store.LanHosts()
}

// lanScanLoop keeps the neighbor inventory fresh without any operator action.
func (s *apiServer) lanScanLoop() {
	interval := envInt("LUMONAS_LAN_SCAN_INTERVAL", 600)
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if _, err := s.lanScan(ctx); err != nil && s.log != nil {
			s.log.Warn("LAN scan failed", "error", err)
		}
		cancel()
	}
}

// lanHosts lists known neighbors from the persisted inventory.
func (s *apiServer) lanHosts(w http.ResponseWriter, _ *http.Request) {
	hosts, err := s.store.LanHosts()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, hosts)
}

// lanScanNow runs an on-demand discovery pass and returns the refreshed list.
func (s *apiServer) lanScanNow(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	hosts, err := s.lanScan(ctx)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "network.lan.scan", "lan", map[string]any{"hosts": len(hosts)})
	writeJSON(w, http.StatusOK, hosts)
}

// lanWake sends a Wake-on-LAN packet through the privileged broker.
func (s *apiServer) lanWake(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		MAC       string `json:"mac"`
		Interface string `json:"interface"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if err := network.ValidateWakeTarget(input.Interface, input.MAC); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	known, err := s.store.LanHostKnown(strings.ToLower(input.MAC), input.Interface)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if !known {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "LAN host has not been discovered on this interface"})
		return
	}
	operationID := newID("wake")
	request := privileged.Request{
		Operation:   "network.wol.wake",
		OperationID: operationID,
		PlanHash:    operationID,
		RequestedState: map[string]any{
			"interface": input.Interface,
			"mac":       strings.ToLower(input.MAC),
		},
		ExpiresAt: time.Now().UTC().Add(2 * time.Minute),
		Confirmed: true,
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := s.brokerExecute(ctx, request); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "network.lan.wake", input.MAC, map[string]any{"interface": input.Interface})
	s.publish("network.lan.wake", "info", &model.ResourceRef{Type: "lan-host", ID: strings.ToLower(input.MAC)}, map[string]any{"mac": strings.ToLower(input.MAC), "interface": input.Interface})
	writeJSON(w, http.StatusOK, map[string]string{"status": "wake packet sent"})
}

// lanRename attaches an operator hostname to a known neighbor.
func (s *apiServer) lanRename(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		MAC       string `json:"mac"`
		Interface string `json:"interface"`
		Hostname  string `json:"hostname"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	input.Hostname = strings.TrimSpace(input.Hostname)
	if input.Hostname != "" && !lanHostnamePattern.MatchString(input.Hostname) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "hostname contains unsupported characters"})
		return
	}
	if err := s.store.RenameLanHost(strings.ToLower(input.MAC), input.Interface, input.Hostname); err != nil {
		if errors.Is(err, store.ErrLanHostNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "LAN host has not been discovered on this interface"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "network.lan.rename", input.MAC, map[string]any{"hostname": input.Hostname})
	writeJSON(w, http.StatusOK, map[string]string{"status": "renamed"})
}
