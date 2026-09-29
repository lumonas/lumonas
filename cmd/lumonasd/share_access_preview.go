package main

import (
	"net/http"
	"sort"

	"github.com/lumonas/lumonas/internal/identity"
)

type shareAccessPreviewEntry struct {
	PrincipalID string               `json:"principalId"`
	Name        string               `json:"name"`
	Kind        identity.Kind        `json:"kind"`
	Enabled     bool                 `json:"enabled"`
	Level       identity.AccessLevel `json:"level"`
	GrantedBy   []string             `json:"grantedBy,omitempty"`
	Guest       bool                 `json:"guest,omitempty"`
}

func (s *apiServer) shareAccessPreview(w http.ResponseWriter, r *http.Request, id string) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	share, err := s.store.ManagedShare(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "share not found"})
		return
	}
	principals, err := s.store.ListPrincipals("")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	groupNames := make(map[string]string)
	for _, principal := range principals {
		if principal.Kind == identity.KindGroup {
			groupNames[principal.Name] = principal.ID
		}
	}
	entries := make([]shareAccessPreviewEntry, 0, len(principals)+1)
	for _, principal := range principals {
		if principal.Kind == identity.KindGroup {
			continue
		}
		level, resolveErr := s.store.ResolveShareAccess(id, principal.ID)
		if resolveErr != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": resolveErr.Error()})
			return
		}
		entry := shareAccessPreviewEntry{PrincipalID: principal.ID, Name: principal.Name, Kind: principal.Kind, Enabled: principal.Enabled, Level: level}
		if principal.Enabled && level != identity.AccessNone {
			for _, rule := range share.Access {
				if rule.PrincipalID == principal.ID && rule.Level == string(level) {
					entry.GrantedBy = append(entry.GrantedBy, "direct")
				}
				for _, groupName := range principal.Groups {
					if rule.PrincipalID == groupNames[groupName] && rule.Level == string(level) {
						entry.GrantedBy = append(entry.GrantedBy, "group:"+groupName)
					}
				}
			}
		}
		entries = append(entries, entry)
	}
	if share.Guest {
		entries = append(entries, shareAccessPreviewEntry{PrincipalID: "guest", Name: "Guest access", Kind: identity.KindService, Enabled: true, Level: identity.AccessRead, Guest: true, GrantedBy: []string{"guest"}})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Kind != entries[j].Kind {
			return entries[i].Kind < entries[j].Kind
		}
		return entries[i].Name < entries[j].Name
	})
	writeJSON(w, http.StatusOK, map[string]any{"shareId": share.ID, "name": share.Name, "enabled": share.Enabled, "guestAccess": share.Guest, "entries": entries, "accessRules": share.Access, "protocols": share.Protocols})
}
