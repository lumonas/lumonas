package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/privileged"
)

type smbSessionStatus struct {
	SessionID string `json:"sessionId"`
	Username  string `json:"username"`
	Machine   string `json:"machine"`
	Dialect   string `json:"dialect,omitempty"`
	Share     string `json:"share,omitempty"`
}

type smbOpenFileStatus struct {
	Path      string `json:"path"`
	SharePath string `json:"sharePath,omitempty"`
	Opens     int    `json:"opens"`
}

func (s *apiServer) shareClients(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	operationID := newID("smb-status")
	result, err := s.executePrivileged(ctx, privileged.Request{Operation: "samba.status.read", OperationID: operationID, PlanHash: operationID, ExpiresAt: time.Now().UTC().Add(10 * time.Second), Confirmed: true})
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Samba status could not be read"})
		return
	}
	if !result.OK {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": result.Error})
		return
	}
	encoded, err := json.Marshal(result.Data)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Samba status could not be decoded"})
		return
	}
	var raw struct {
		Sessions map[string]struct {
			SessionID string `json:"session_id"`
			Username  string `json:"username"`
			Machine   string `json:"remote_machine"`
			Dialect   string `json:"session_dialect"`
		} `json:"sessions"`
		TCons map[string]struct {
			SessionID string `json:"session_id"`
			Service   string `json:"service"`
			Machine   string `json:"machine"`
		} `json:"tcons"`
		OpenFiles map[string]struct {
			ServicePath string         `json:"service_path"`
			Filename    string         `json:"filename"`
			Opens       map[string]any `json:"opens"`
		} `json:"open_files"`
	}
	if err := json.Unmarshal(encoded, &raw); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Samba returned an unsupported status format"})
		return
	}
	sharesBySession := map[string][]string{}
	for _, tcon := range raw.TCons {
		if tcon.Service != "" {
			sharesBySession[tcon.SessionID] = append(sharesBySession[tcon.SessionID], tcon.Service)
		}
	}
	sessions := make([]smbSessionStatus, 0, len(raw.Sessions))
	for key, session := range raw.Sessions {
		id := session.SessionID
		if id == "" {
			id = key
		}
		shares := uniqueSorted(sharesBySession[id])
		sessions = append(sessions, smbSessionStatus{SessionID: id, Username: session.Username, Machine: session.Machine, Dialect: session.Dialect, Share: strings.Join(shares, ", ")})
	}
	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].Username != sessions[j].Username {
			return sessions[i].Username < sessions[j].Username
		}
		return sessions[i].Machine < sessions[j].Machine
	})
	openFiles := make([]smbOpenFileStatus, 0, len(raw.OpenFiles))
	for key, file := range raw.OpenFiles {
		name := file.Filename
		if name == "" {
			name = key
		}
		openFiles = append(openFiles, smbOpenFileStatus{Path: name, SharePath: file.ServicePath, Opens: len(file.Opens)})
	}
	sort.Slice(openFiles, func(i, j int) bool { return openFiles[i].Path < openFiles[j].Path })
	writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions, "openFiles": openFiles, "capturedAt": time.Now().UTC(), "service": "smbd.service"})
}

func (s *apiServer) disconnectShareClient(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		Address   string `json:"address"`
		Confirmed bool   `json:"confirmed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if net.ParseIP(input.Address) == nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "a valid client IP address is required"})
		return
	}
	if !input.Confirmed {
		writeJSON(w, http.StatusLocked, map[string]string{"error": "explicit confirmation is required to disconnect every SMB session from this client"})
		return
	}
	operationID := newID("smb-disconnect")
	result, err := s.executePrivileged(r.Context(), privileged.Request{Operation: "samba.client.disconnect", OperationID: operationID, PlanHash: operationID, RequestedState: map[string]any{"address": input.Address}, ExpiresAt: time.Now().UTC().Add(2 * time.Minute), Confirmed: true})
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Samba client could not be disconnected"})
		return
	}
	if !result.OK {
		writeJSON(w, http.StatusConflict, map[string]string{"error": result.Error})
		return
	}
	s.recordRequestAudit(r, actor, "share.client.disconnect", input.Address, map[string]any{"operationId": operationID})
	s.publishActor(actor, "share.client.disconnected", "warning", &model.ResourceRef{Type: "smb-client", ID: input.Address}, map[string]any{"operationId": operationID})
	writeJSON(w, http.StatusOK, map[string]any{"address": input.Address, "disconnected": true})
}

func uniqueSorted(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}
