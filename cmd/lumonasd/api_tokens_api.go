package main

import (
	"encoding/json"
	"net/http"
	"path"
	"strings"

	"github.com/lumonas/lumonas/internal/identity"
	"github.com/lumonas/lumonas/internal/store"
)

func (s *apiServer) apiTokenOwner(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return "", "", false
	}
	name := strings.TrimPrefix(actor, "token:")
	principal, err := s.store.PrincipalByName(name)
	if err != nil || principal.ManagementRole != identity.RoleOwner && principal.ManagementRole != identity.RoleAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "owner access is required to manage API tokens"})
		return "", "", false
	}
	return actor, principal.ID, true
}

func (s *apiServer) listAPITokens(w http.ResponseWriter, r *http.Request) {
	_, ownerID, ok := s.apiTokenOwner(w, r)
	if !ok {
		return
	}
	values, err := s.store.ListAPITokens(ownerID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, values)
}

func (s *apiServer) createAPIToken(w http.ResponseWriter, r *http.Request) {
	actor, ownerID, ok := s.apiTokenOwner(w, r)
	if !ok {
		return
	}
	var input struct {
		Name      string   `json:"name"`
		Scopes    []string `json:"scopes"`
		ExpiresAt string   `json:"expiresAt"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || !validTokenScopes(input.Scopes) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "choose a supported API token scope"})
		return
	}
	expires, err := parseTokenExpiry(input.ExpiresAt)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	summary, token, err := s.store.CreateAPIToken(store.APITokenCreate{ID: newID("token"), OwnerID: ownerID, Name: input.Name, Scopes: input.Scopes, ExpiresAt: expires})
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "security.api_token.create", summary.ID, map[string]any{"scopes": summary.Scopes})
	writeJSON(w, http.StatusCreated, map[string]any{"token": token, "summary": summary})
}

func (s *apiServer) deleteAPIToken(w http.ResponseWriter, r *http.Request, id string) {
	actor, ownerID, ok := s.apiTokenOwner(w, r)
	if !ok {
		return
	}
	if err := s.store.DeleteAPIToken(ownerID, path.Base(id)); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "API token not found"})
		return
	}
	s.recordRequestAudit(r, actor, "security.api_token.revoke", path.Base(id), nil)
	w.WriteHeader(http.StatusNoContent)
}
