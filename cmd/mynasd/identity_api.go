package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/lumonas/lumonas/internal/identity"
	"github.com/lumonas/lumonas/internal/store"
)

func (s *apiServer) identityActor(w http.ResponseWriter, r *http.Request, mutate bool) (string, bool) {
	if !s.authRequired {
		return "local", true
	}
	cookie, err := r.Cookie("mynas_session")
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return "", false
	}
	username, ok := s.store.SessionUser(cookie.Value)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired session"})
		return "", false
	}
	principal, err := s.store.PrincipalByName(username)
	if err != nil || !principal.Enabled || principal.ManagementRole == identity.RoleNone {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "management access is required"})
		return "", false
	}
	if mutate && principal.ManagementRole != identity.RoleOwner && principal.ManagementRole != identity.RoleAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "administrator access is required"})
		return "", false
	}
	return username, true
}

func (s *apiServer) expectedIdentityGeneration(w http.ResponseWriter, expected *int64) bool {
	if expected != nil && *expected != s.currentGeneration() {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "configuration generation changed; refresh and retry"})
		return false
	}
	return true
}

func (s *apiServer) listUsers(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	values, err := s.store.ListPrincipals("")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	filtered := make([]identity.Principal, 0, len(values))
	for _, value := range values {
		if value.Kind != identity.KindGroup {
			filtered = append(filtered, value)
		}
	}
	writeJSON(w, http.StatusOK, filtered)
}

func (s *apiServer) listGroups(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	values, err := s.store.ListPrincipals(identity.KindGroup)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, values)
}

func (s *apiServer) createUser(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		Kind               identity.Kind           `json:"kind"`
		Name               string                  `json:"name"`
		Password           string                  `json:"password"`
		ManagementRole     identity.ManagementRole `json:"managementRole"`
		ExpectedGeneration *int64                  `json:"expectedGeneration"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if !s.expectedIdentityGeneration(w, input.ExpectedGeneration) {
		return
	}
	if input.Kind == "" {
		input.Kind = identity.KindUser
	}
	if input.Kind != identity.KindUser && input.Kind != identity.KindService {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "users endpoint accepts user or service identities"})
		return
	}
	principal, err := s.store.CreatePrincipal(identity.CreateInput{Kind: input.Kind, Name: input.Name, Password: input.Password, ManagementRole: input.ManagementRole})
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	s.recordIdentityAudit(actor, "identity.create", principal.ID, map[string]any{"kind": principal.Kind})
	s.advanceGeneration("identity.create")
	writeJSON(w, http.StatusCreated, principal)
}

func (s *apiServer) createGroup(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		Name               string `json:"name"`
		ExpectedGeneration *int64 `json:"expectedGeneration"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if !s.expectedIdentityGeneration(w, input.ExpectedGeneration) {
		return
	}
	principal, err := s.store.CreatePrincipal(identity.CreateInput{Kind: identity.KindGroup, Name: input.Name})
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	s.recordIdentityAudit(actor, "identity.group.create", principal.ID, nil)
	s.advanceGeneration("identity.group.create")
	writeJSON(w, http.StatusCreated, principal)
}

func (s *apiServer) updateUser(w http.ResponseWriter, r *http.Request, id string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		Name               *string                  `json:"name"`
		Enabled            *bool                    `json:"enabled"`
		ManagementRole     *identity.ManagementRole `json:"managementRole"`
		ExpectedGeneration *int64                   `json:"expectedGeneration"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if !s.expectedIdentityGeneration(w, input.ExpectedGeneration) {
		return
	}
	principal, err := s.store.UpdatePrincipal(id, identity.UpdateInput{Name: input.Name, Enabled: input.Enabled, ManagementRole: input.ManagementRole})
	if err != nil {
		writeJSON(w, statusForIdentityError(err), map[string]string{"error": err.Error()})
		return
	}
	s.recordIdentityAudit(actor, "identity.update", id, map[string]any{"enabled": principal.Enabled, "managementRole": principal.ManagementRole})
	s.advanceGeneration("identity.update")
	writeJSON(w, http.StatusOK, principal)
}

func (s *apiServer) updateGroup(w http.ResponseWriter, r *http.Request, id string) {
	s.updateUser(w, r, id)
}

func (s *apiServer) setUserPassword(w http.ResponseWriter, r *http.Request, id string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		Password           string `json:"password"`
		ExpectedGeneration *int64 `json:"expectedGeneration"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if !s.expectedIdentityGeneration(w, input.ExpectedGeneration) {
		return
	}
	if err := s.store.SetPrincipalPassword(id, input.Password); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	s.recordIdentityAudit(actor, "identity.password.rotate", id, nil)
	s.advanceGeneration("identity.password.rotate")
	w.WriteHeader(http.StatusNoContent)
}

func (s *apiServer) deletePrincipal(w http.ResponseWriter, r *http.Request, id string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	if err := s.store.DeletePrincipal(id); err != nil {
		writeJSON(w, statusForIdentityError(err), map[string]string{"error": err.Error()})
		return
	}
	s.recordIdentityAudit(actor, "identity.delete", id, nil)
	s.advanceGeneration("identity.delete")
	w.WriteHeader(http.StatusNoContent)
}

func (s *apiServer) setGroupMembers(w http.ResponseWriter, r *http.Request, id string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		MemberIDs          []string `json:"memberIds"`
		ExpectedGeneration *int64   `json:"expectedGeneration"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if !s.expectedIdentityGeneration(w, input.ExpectedGeneration) {
		return
	}
	if err := s.store.SetGroupMembers(id, input.MemberIDs); err != nil {
		writeJSON(w, statusForIdentityError(err), map[string]string{"error": err.Error()})
		return
	}
	s.recordIdentityAudit(actor, "identity.group.members.update", id, map[string]any{"memberCount": len(input.MemberIDs)})
	s.advanceGeneration("identity.group.members.update")
	members, err := s.store.GroupMembers(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, members)
}

func (s *apiServer) recordIdentityAudit(actor, action, id string, metadata map[string]any) {
	_ = s.store.SaveAudit(store.AuditEntry{Actor: actor, Action: action, Outcome: "committed", ResourceType: "principal", ResourceID: id, Metadata: metadata})
}

func statusForIdentityError(err error) int {
	if errors.Is(err, sql.ErrNoRows) {
		return http.StatusNotFound
	}
	if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "referenced") || strings.Contains(err.Error(), "last enabled") {
		return http.StatusConflict
	}
	return http.StatusUnprocessableEntity
}
