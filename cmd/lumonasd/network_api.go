package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/network"
	"github.com/lumonas/lumonas/internal/privileged"
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

func (s *apiServer) createNetworkConnection(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	idempotencyKey, err := requestIdempotencyKey(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if idempotencyKey != "" {
		if connectionID, found := s.store.Meta(idempotencyMetaKey("network.connection.create", idempotencyKey)); found {
			if existing, getErr := s.store.NetworkConnection(connectionID); getErr == nil {
				writeJSON(w, http.StatusOK, existing)
				return
			}
		}
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
	if !input.Reauthenticated {
		writeJSON(w, http.StatusLocked, map[string]string{"error": "reauthentication is required for network changes"})
		return
	}
	if input.ID == "" {
		input.ID = newID("connection")
	}
	input.Generation, input.Status = s.currentGeneration()+1, "pending-checkpoint"
	if err := input.Connection.Validate(); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	value, err := s.store.UpsertNetworkConnection(input.Connection)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "network.connection.create", value.ID, map[string]any{"interface": value.Interface})
	if idempotencyKey != "" {
		_ = s.store.SetMeta(idempotencyMetaKey("network.connection.create", idempotencyKey), value.ID)
	}
	s.advanceGeneration("network.connection.create")
	s.publish("network.connection.updated", "warning", &model.ResourceRef{Type: "network-connection", ID: value.ID}, map[string]any{"requiresCheckpoint": true})
	writeJSON(w, http.StatusCreated, value)
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
	input.Generation, input.Status = s.currentGeneration()+1, "pending-checkpoint"
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
	s.recordRequestAudit(r, actor, "network.connection.update", id, map[string]any{"interface": value.Interface})
	s.advanceGeneration("network.connection.update")
	s.publish("network.connection.updated", "warning", &model.ResourceRef{Type: "network-connection", ID: id}, map[string]any{"requiresCheckpoint": true})
	writeJSON(w, http.StatusOK, value)
}

func (s *apiServer) applyNetworkConnection(w http.ResponseWriter, r *http.Request, id string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		Devices            []string `json:"devices"`
		TimeoutSeconds     int      `json:"timeoutSeconds"`
		ExpectedGeneration *int64   `json:"expectedGeneration"`
		Reauthenticated    bool     `json:"reauthenticated"`
		WiFiPassword       string   `json:"wifiPassword"`
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
	connection, err := s.store.NetworkConnection(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "network connection not found"})
		return
	}
	if connection.Type == "wifi" {
		s.applyWiFiConnection(w, r, actor, id, input.WiFiPassword, input.TimeoutSeconds, connection)
		return
	}
	changes, err := connection.NetworkManagerChanges()
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	if input.TimeoutSeconds == 0 {
		input.TimeoutSeconds = 60
	}
	operationID := newID("net")
	requested := map[string]any{"connectionUuid": connection.UUID, "connectionId": id, "devices": input.Devices, "changes": changes, "timeoutSeconds": input.TimeoutSeconds}
	result, err := (privileged.Client{Socket: envOr("LUMONAS_PRIVD_SOCKET", "/run/lumonas/privd.sock")}).Execute(r.Context(), privileged.Request{Operation: "network.checkpoint.begin", OperationID: operationID, PlanHash: operationID, RequestedState: requested, ExpiresAt: time.Now().UTC().Add(time.Duration(input.TimeoutSeconds+60) * time.Second), Confirmed: true})
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	if !result.OK {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": result.Error})
		return
	}
	connection.Status = "checkpoint-pending"
	if _, err := s.store.UpsertNetworkConnection(connection); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := s.store.RecordNetworkCheckpoint(operationID, id, "pending"); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "network.connection.apply", id, map[string]any{"operationId": operationID})
	s.publish("network.checkpoint.created", "warning", &model.ResourceRef{Type: "network-connection", ID: id}, map[string]any{"operationId": operationID, "timeoutSeconds": input.TimeoutSeconds})
	writeJSON(w, http.StatusAccepted, result)
}

// applyWiFiConnection connects a Wi-Fi connection through the privileged
// worker. The password travels request-to-socket only: it is never written to
// the store and privd feeds it to nmcli over stdin instead of argv.
func (s *apiServer) applyWiFiConnection(w http.ResponseWriter, r *http.Request, actor, id, password string, timeoutSeconds int, connection network.Connection) {
	if !connection.WiFiOpen {
		if err := network.ValidateWiFiPSK(password); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
			return
		}
	}
	if timeoutSeconds == 0 {
		timeoutSeconds = 60
	}
	if timeoutSeconds < 30 || timeoutSeconds > 900 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "timeoutSeconds must be between 30 and 900"})
		return
	}
	operationID := newID("wifi")
	requested := map[string]any{"ssid": connection.SSID, "ifname": connection.Interface, "timeoutSeconds": timeoutSeconds}
	if !connection.WiFiOpen {
		requested["psk"] = password
	}
	result, err := (privileged.Client{Socket: envOr("LUMONAS_PRIVD_SOCKET", "/run/lumonas/privd.sock")}).Execute(r.Context(), privileged.Request{Operation: "network.wifi.connect", OperationID: operationID, PlanHash: operationID, RequestedState: requested, ExpiresAt: time.Now().UTC().Add(time.Duration(timeoutSeconds+60) * time.Second), Confirmed: true})
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	if !result.OK {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": result.Error})
		return
	}
	connection.Status = "activating"
	if _, err := s.store.UpsertNetworkConnection(connection); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "network.connection.apply", id, map[string]any{"operationId": operationID, "type": "wifi"})
	s.publish("network.connection.updated", "warning", &model.ResourceRef{Type: "network-connection", ID: id}, map[string]any{"operationId": operationID, "state": "activating"})
	writeJSON(w, http.StatusAccepted, result)
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
	policy, err := s.store.NetworkFirewallPolicy()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	rules, err := network.RenderNftables(policy, input.Bindings)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	policy.GeneratedRules = rules
	rollbackFirewall, err := s.activateFirewallRules(r.Context(), rules)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "firewall activation failed: " + err.Error()})
		return
	}
	values, err := s.store.ReplaceNetworkBindings(input.Bindings)
	if err != nil {
		rollbackFirewall()
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	if _, err := s.store.SaveNetworkFirewallPolicy(policy); err != nil {
		rollbackFirewall()
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "network.binding.update", "bindings", nil)
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
	bindings, err := s.store.ListNetworkBindings()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	rules, err := network.RenderNftables(input.FirewallPolicy, bindings)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	input.GeneratedRules = rules
	rollbackFirewall, err := s.activateFirewallRules(r.Context(), rules)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "firewall activation failed: " + err.Error()})
		return
	}
	value, err := s.store.SaveNetworkFirewallPolicy(input.FirewallPolicy)
	if err != nil {
		rollbackFirewall()
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "network.firewall.update", "firewall", nil)
	s.advanceGeneration("network.firewall.update")
	s.publish("network.firewall.updated", "warning", nil, nil)
	writeJSON(w, http.StatusOK, value)
}

func (s *apiServer) activateFirewallRules(ctx context.Context, rules string) (func(), error) {
	path := envOr("LUMONAS_FIREWALL_CONFIG", "/var/lib/lumonas/generated/nftables.conf")
	previous, readErr := os.ReadFile(path)
	existed := readErr == nil
	if readErr != nil && !os.IsNotExist(readErr) {
		return func() {}, readErr
	}
	if err := network.WriteNftables(path, rules); err != nil {
		return func() {}, err
	}
	rollback := func() {
		if existed {
			_ = os.WriteFile(path, previous, 0o640)
		} else {
			_ = os.Remove(path)
		}
	}
	socket := envOr("LUMONAS_PRIVD_SOCKET", "/run/lumonas/privd.sock")
	if _, err := os.Stat(socket); err == nil {
		result, err := (privileged.Client{Socket: socket}).Execute(ctx, privileged.Request{
			Operation: "firewall.apply", OperationID: newID("firewall"), PlanHash: newID("firewall-plan"),
			RequestedState: map[string]any{"configPath": path}, Confirmed: true,
		})
		if err != nil {
			rollback()
			return func() {}, err
		}
		if !result.OK {
			rollback()
			return func() {}, fmt.Errorf("%s", result.Error)
		}
	}
	return rollback, nil
}

func (s *apiServer) networkDiagnostic(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	idempotencyKey, err := requestIdempotencyKey(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if idempotencyKey != "" {
		if jobID, found := s.store.Meta(idempotencyMetaKey("network.diagnostic", idempotencyKey)); found {
			if job, getErr := s.store.Job(jobID); getErr == nil {
				writeJSON(w, http.StatusAccepted, job)
				return
			}
		}
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
	if err := validateDiagnostic(input.Kind, input.Target, input.Port); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	job := model.Job{ID: newID("job"), CorrelationID: requestCorrelationID(r), Type: "network.diagnostic", Title: "Network " + input.Kind, ResourceID: input.Target, State: "queued", CreatedAt: time.Now().UTC()}
	if err := s.store.SaveJob(job); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.publish("job.state_changed", "info", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job})
	if idempotencyKey != "" {
		_ = s.store.SetMeta(idempotencyMetaKey("network.diagnostic", idempotencyKey), job.ID)
	}
	go s.runNetworkDiagnostic(job, input.Kind, input.Target, input.Port)
	writeJSON(w, http.StatusAccepted, job)
}

func (s *apiServer) networkDiagnosticJob(w http.ResponseWriter, r *http.Request, id string) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	job, err := s.store.Job(id)
	if err != nil || job.Type != "network.diagnostic" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "network diagnostic job not found"})
		return
	}
	result, err := s.store.NetworkDiagnosticResult(id)
	if err == nil {
		writeJSON(w, http.StatusOK, map[string]any{"job": job, "result": result})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"job": job})
}

type diagnosticInput struct {
	Kind   string
	Target string
	Port   int
}

func validateDiagnostic(kind, target string, port int) error {
	switch kind {
	case "interfaces", "route-table", "neighbor-table":
		if target != "" || port != 0 {
			return fmt.Errorf("%s does not accept a target", kind)
		}
	case "ping", "dns-lookup", "traceroute", "gateway-reachability", "internet-reachability", "update-endpoint", "docker-registry":
		if !validDiagnosticHost(target) {
			return errors.New("diagnostic target is invalid")
		}
	case "port-test":
		if !validDiagnosticHost(target) || port < 1 || port > 65535 {
			return errors.New("port test target or port is invalid")
		}
	default:
		return errors.New("unsupported diagnostic kind")
	}
	return nil
}

func validDiagnosticHost(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 253 || strings.ContainsAny(value, " /\\\r\n") {
		return false
	}
	if net.ParseIP(value) != nil {
		return true
	}
	for _, label := range strings.Split(value, ".") {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, char := range label {
			if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '-' {
				return false
			}
		}
	}
	return true
}

func (s *apiServer) runNetworkDiagnostic(job model.Job, kind, target string, port int) {
	now := time.Now().UTC()
	progress := 10.0
	job.State, job.Stage, job.StartedAt, job.Progress = "running", "Running diagnostic", &now, &progress
	_ = s.store.SaveJob(job)
	s.publish("job.state_changed", "info", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := executeDiagnostic(ctx, kind, target, port)
	now = time.Now().UTC()
	progress = 100
	job.FinishedAt, job.Progress = &now, &progress
	if err != nil {
		job.State, job.Stage, job.Error = "failed", "Diagnostic failed", err.Error()
		_ = s.store.SaveJob(job)
		s.publish("job.state_changed", "warning", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job})
		return
	}
	job.State, job.Stage = "successful", "Diagnostic completed"
	_ = s.store.SaveNetworkDiagnosticResult(job.ID, result)
	_ = s.store.SaveJob(job)
	s.publish("job.state_changed", "info", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job, "result": result})
}

func executeDiagnostic(ctx context.Context, kind, target string, port int) (any, error) {
	switch kind {
	case "interfaces":
		return network.Interfaces()
	case "dns-lookup":
		return net.DefaultResolver.LookupHost(ctx, target)
	case "port-test":
		conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(target, strconv.Itoa(port)))
		if err != nil {
			return nil, err
		}
		_ = conn.Close()
		return map[string]any{"target": target, "port": port, "reachable": true}, nil
	case "ping", "traceroute":
		command := "ping"
		args := []string{"-c", "1", "-W", "2", target}
		if kind == "traceroute" {
			command, args = "traceroute", []string{"-m", "8", "-w", "2", target}
		}
		output, err := exec.CommandContext(ctx, command, args...).CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("%s failed", kind)
		}
		if len(output) > 4096 {
			output = output[:4096]
		}
		return map[string]any{"target": target, "output": string(output)}, nil
	case "route-table":
		return readDiagnosticFile("/proc/net/route")
	case "neighbor-table":
		return readDiagnosticFile("/proc/net/arp")
	case "gateway-reachability", "internet-reachability", "update-endpoint", "docker-registry":
		conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(target, "443"))
		if err != nil {
			return nil, err
		}
		_ = conn.Close()
		return map[string]any{"target": target, "port": 443, "reachable": true}, nil
	default:
		return nil, errors.New("unsupported diagnostic kind")
	}
}

func readDiagnosticFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if len(data) > 16384 {
		data = data[:16384]
	}
	return string(data), nil
}
