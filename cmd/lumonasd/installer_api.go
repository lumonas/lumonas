package main

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/lumonas/lumonas/internal/installer"
	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/privileged"
)

// installerMode reports whether the daemon runs from the offline medium and
// may expose the installation endpoints. Outside installer mode the routes
// return 404 so a running appliance never offers to install over itself.
func (s *apiServer) installerMode() bool {
	return envOr("LUMONAS_INSTALLER_MODE", "false") == "true"
}

func (s *apiServer) installerUnavailable(w http.ResponseWriter) {
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "installation endpoints are only available from the offline installer medium"})
}

// installerState carries the single in-flight installation. The installer
// medium serves one operator at a time, so a mutex-guarded slot is
// intentionally simple.
type installerState struct {
	mu    sync.Mutex
	plan  *installer.Plan
	hash  string
	stage string
	detail string
}

func (s *apiServer) installerTargets(w http.ResponseWriter, r *http.Request) {
	if !s.installerMode() {
		s.installerUnavailable(w)
		return
	}
	disks, err := s.diskFunc()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, installer.EvaluateTargets(disks, s.systemDiskID(disks)))
}

func (s *apiServer) systemDiskID(disks []model.Disk) string {
	for _, disk := range disks {
		if disk.Role == "system" {
			return disk.ID
		}
	}
	return ""
}

func (s *apiServer) installerPlan(w http.ResponseWriter, r *http.Request) {
	if !s.installerMode() {
		s.installerUnavailable(w)
		return
	}
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		TargetDiskID  string `json:"targetDiskId"`
		Hostname      string `json:"hostname"`
		AdminUsername string `json:"adminUsername"`
		Filesystem    string `json:"filesystem"`
		UEFI          bool   `json:"uefi"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	disks, err := s.diskFunc()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	plan, targets, err := installer.BuildPlan(disks, s.systemDiskID(disks), installer.BuildRequest{
		TargetDiskID:  input.TargetDiskID,
		Hostname:      input.Hostname,
		AdminUsername: input.AdminUsername,
		Filesystem:    input.Filesystem,
		UEFI:          input.UEFI,
	}, time.Now())
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error(), "targets": targets})
		return
	}
	hash := plan.Hash()
	s.installer.mu.Lock()
	s.installer.plan, s.installer.hash, s.installer.stage, s.installer.detail = &plan, hash, "planned", ""
	s.installer.mu.Unlock()
	s.recordRequestAudit(r, actor, "install.plan", plan.TargetDiskID, map[string]any{"planId": plan.ID})
	writeJSON(w, http.StatusCreated, map[string]any{"plan": plan, "hash": hash, "targets": targets})
}

// installerApply validates the quoted plan and drives the privileged
// installation. The administrator password is accepted here, forwarded to the
// broker over the Unix socket, and never persisted or logged.
func (s *apiServer) installerApply(w http.ResponseWriter, r *http.Request) {
	if !s.installerMode() {
		s.installerUnavailable(w)
		return
	}
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		Hash          string `json:"hash"`
		Confirm       bool   `json:"confirm"`
		AdminPassword string `json:"adminPassword"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	s.installer.mu.Lock()
	plan, hash, stage := s.installer.plan, s.installer.hash, s.installer.stage
	s.installer.mu.Unlock()
	if plan == nil || stage != "planned" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "generate an installation plan first"})
		return
	}
	if err := plan.ValidateForApply(input.Hash, input.Confirm, time.Now()); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	if len(input.AdminPassword) < 12 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "administrator password must contain at least 12 characters"})
		return
	}
	request := privileged.Request{
		Operation:   "install.apply",
		OperationID: newID("install"),
		PlanHash:    hash,
		TargetDiskID: plan.TargetDiskID,
		ExpectedIdentity: plan.ExpectedIdentity,
		RequestedState: map[string]any{
			"filesystem":    plan.Filesystem,
			"uefi":          plan.UEFI,
			"hostname":      plan.Hostname,
			"adminUsername": plan.AdminUsername,
			"adminPassword": input.AdminPassword,
		},
		ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
		Confirmed: true,
	}
	s.setInstallerStage("applying", "")
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Minute)
	defer cancel()
	result, err := s.executePrivileged(ctx, request)
	if err != nil || !result.OK {
		detail := ""
		if err != nil {
			detail = err.Error()
		} else {
			detail = result.Error
		}
		s.setInstallerStage("failed", detail)
		s.publish("install.failed", "critical", &model.ResourceRef{Type: "install", ID: plan.ID}, map[string]any{"reason": detail})
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": detail})
		return
	}
	s.setInstallerStage("succeeded", "")
	s.recordRequestAudit(r, actor, "install.apply", plan.TargetDiskID, map[string]any{"planId": plan.ID})
	s.publish("install.completed", "info", &model.ResourceRef{Type: "install", ID: plan.ID}, map[string]any{"hostname": plan.Hostname})
	writeJSON(w, http.StatusOK, map[string]any{"status": "succeeded", "hostname": plan.Hostname})
}

func (s *apiServer) installerStatus(w http.ResponseWriter, r *http.Request) {
	if !s.installerMode() {
		s.installerUnavailable(w)
		return
	}
	s.installer.mu.Lock()
	defer s.installer.mu.Unlock()
	payload := map[string]any{"stage": s.installer.stage, "detail": s.installer.detail}
	if s.installer.plan != nil {
		payload["planId"] = s.installer.plan.ID
		payload["hostname"] = s.installer.plan.Hostname
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *apiServer) setInstallerStage(stage, detail string) {
	s.installer.mu.Lock()
	s.installer.stage, s.installer.detail = stage, detail
	s.installer.mu.Unlock()
}
