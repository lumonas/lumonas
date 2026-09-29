package main

import (
	"encoding/json"
	"net/http"

	"github.com/lumonas/lumonas/internal/backup"
	"github.com/lumonas/lumonas/internal/model"
)

func (s *apiServer) backupPolicyTemplates(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.backupActor(w, r, false); !ok {
		return
	}
	writeJSON(w, http.StatusOK, backup.PolicyTemplates())
}

func (s *apiServer) applyBackupPolicyTemplate(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.backupActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		TemplateID string `json:"templateId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if _, ok := backup.PolicyTemplateByID(input.TemplateID); !ok {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "unknown backup policy template"})
		return
	}
	schedule, err := s.store.ApplyBackupPolicyTemplate(input.TemplateID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not apply backup policy template"})
		return
	}
	s.recordRequestAudit(r, actor, "backup.policy_template.apply", input.TemplateID, map[string]any{"intervalSeconds": schedule.IntervalSeconds})
	s.advanceGeneration("backup.policy_template.apply")
	s.publishActor(actor, "backup.policy_template.applied", "info", &model.ResourceRef{Type: "backup-policy-template", ID: input.TemplateID}, map[string]any{"intervalSeconds": schedule.IntervalSeconds})
	writeJSON(w, http.StatusOK, schedule)
}
