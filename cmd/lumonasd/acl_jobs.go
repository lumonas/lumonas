package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/lumonas/lumonas/internal/acl"
	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/privileged"
)

type aclJobRequest struct {
	Path               string      `json:"path"`
	Entries            []acl.Entry `json:"entries"`
	Recursive          bool        `json:"recursive"`
	ExpectedGeneration *int64      `json:"expectedGeneration"`
	Reauthenticated    bool        `json:"reauthenticated"`
}

func (s *apiServer) createACLJob(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input aclJobRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if !s.expectedIdentityGeneration(w, input.ExpectedGeneration) {
		return
	}
	if !input.Reauthenticated {
		writeJSON(w, http.StatusLocked, map[string]string{"error": "reauthentication is required for ACL changes"})
		return
	}
	if err := acl.Validate(input.Path, input.Entries, input.Recursive); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	if _, err := os.Stat(filepath.Clean(input.Path)); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "ACL path does not exist"})
		return
	}
	job := model.Job{ID: newID("job"), CorrelationID: requestCorrelationID(r), Type: "acl.apply", Title: "Apply ACL changes", ResourceID: input.Path, State: "queued", CreatedAt: time.Now().UTC()}
	job.Stage = "Estimated " + formatACLCount(input.Path, input.Recursive) + " filesystem entries"
	if err := s.store.SaveJob(job); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "acl.apply.queued", input.Path, map[string]any{"jobId": job.ID, "recursive": input.Recursive, "entryCount": len(input.Entries)})
	s.publish("job.state_changed", "info", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job})
	go s.runACLJob(job, input, actor)
	writeJSON(w, http.StatusAccepted, job)
}

func (s *apiServer) cancelACLJob(w http.ResponseWriter, r *http.Request, id string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		ExpectedGeneration *int64 `json:"expectedGeneration"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
	}
	if !s.expectedIdentityGeneration(w, input.ExpectedGeneration) {
		return
	}
	job, err := s.store.Job(id)
	if err != nil || job.Type != "acl.apply" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "ACL job not found"})
		return
	}
	if job.State != "queued" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "running ACL jobs cannot be cancelled safely"})
		return
	}
	now := time.Now().UTC()
	job.State, job.Stage, job.FinishedAt = "cancelled", "Cancelled before execution", &now
	if err := s.store.SaveJob(job); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "acl.apply.cancel", id, map[string]any{"operationId": id})
	s.advanceGeneration("acl.apply.cancel")
	writeJSON(w, http.StatusOK, job)
}

func (s *apiServer) runACLJob(job model.Job, input aclJobRequest, actor string) {
	if current, err := s.store.Job(job.ID); err != nil || current.State == "cancelled" {
		return
	}
	now := time.Now().UTC()
	progress := 10.0
	job.State, job.Stage, job.StartedAt, job.Progress = "running", "Applying ACL entries", &now, &progress
	_ = s.store.SaveJob(job)
	s.publish("job.state_changed", "info", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job})
	entries := make([]any, 0, len(input.Entries))
	for _, entry := range input.Entries {
		entries = append(entries, map[string]any{"principal": entry.Principal, "level": entry.Level})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	result, err := (privileged.Client{Socket: envOr("LUMONAS_PRIVD_SOCKET", "/run/lumonas/privd.sock")}).Execute(ctx, privileged.Request{Operation: "acl.apply", OperationID: job.ID, PlanHash: job.ID, RequestedState: map[string]any{"path": input.Path, "entries": entries, "recursive": input.Recursive}, Confirmed: true})
	now = time.Now().UTC()
	progress = 100
	job.FinishedAt, job.Progress = &now, &progress
	if err != nil {
		job.State, job.Stage, job.Error = "failed", "ACL application failed", err.Error()
	} else if !result.OK {
		job.State, job.Stage, job.Error = "failed", "ACL application failed", result.Error
	} else {
		job.State, job.Stage = "successful", "ACL changes applied"
		s.recordIdentityAudit(actor, "acl.apply", input.Path, map[string]any{"jobId": job.ID, "correlationId": job.CorrelationID, "recursive": input.Recursive})
		s.advanceGeneration("acl.apply")
	}
	_ = s.store.SaveJob(job)
	severity := "info"
	if job.State == "failed" {
		severity = "warning"
	}
	s.publish("job.state_changed", severity, &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job})
}

func formatACLCount(path string, recursive bool) string {
	if !recursive {
		return "1"
	}
	count := 0
	_ = filepath.WalkDir(path, func(_ string, entry os.DirEntry, err error) error {
		if err == nil && entry != nil {
			count++
		}
		return nil
	})
	if count == 0 {
		return "at least 1"
	}
	return formatInt(count)
}

func formatInt(value int) string {
	return fmt.Sprintf("%d", value)
}
