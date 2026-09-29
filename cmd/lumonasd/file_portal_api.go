package main

import (
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	fileops "github.com/lumonas/lumonas/internal/files"
	"github.com/lumonas/lumonas/internal/identity"
	"github.com/lumonas/lumonas/internal/shares"
	"github.com/lumonas/lumonas/internal/store"
)

func (s *apiServer) filePortalRoutes(w http.ResponseWriter, r *http.Request, endpoint string) bool {
	switch {
	case endpoint == "/portal/shares" && r.Method == http.MethodGet:
		s.filePortalShares(w, r)
	case endpoint == "/portal/files" && r.Method == http.MethodGet:
		s.filePortalList(w, r)
	case endpoint == "/portal/files/download" && r.Method == http.MethodGet:
		s.filePortalDownload(w, r)
	case endpoint == "/portal/files/upload" && r.Method == http.MethodPost:
		s.filePortalUpload(w, r)
	case endpoint == "/portal/snapshots" && r.Method == http.MethodGet:
		s.filePortalSnapshots(w, r)
	case strings.HasPrefix(endpoint, "/portal/snapshots/") && strings.HasSuffix(endpoint, "/files") && r.Method == http.MethodGet:
		s.filePortalSnapshotFiles(w, r, path.Base(path.Dir(endpoint)))
	default:
		return false
	}
	return true
}

func (s *apiServer) filePortalPrincipal(w http.ResponseWriter, r *http.Request) (identity.Principal, string, bool) {
	if !s.authEnabled() {
		return identity.Principal{}, "local", true
	}
	cookie, err := r.Cookie("lumonas_session")
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return identity.Principal{}, "", false
	}
	username, ok := s.store.SessionUser(cookie.Value)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired session"})
		return identity.Principal{}, "", false
	}
	principal, err := s.store.PrincipalByName(username)
	if err != nil || !principal.Enabled || principal.Kind != identity.KindUser {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "an enabled NAS user account is required"})
		return identity.Principal{}, "", false
	}
	return principal, username, true
}

func (s *apiServer) filePortalAccess(w http.ResponseWriter, r *http.Request, shareID string, required identity.AccessLevel) (shares.ManagedShare, string, bool) {
	principal, username, ok := s.filePortalPrincipal(w, r)
	if !ok {
		return shares.ManagedShare{}, "", false
	}
	share, err := s.fileShare(shareID)
	if err != nil {
		writeFileError(w, err)
		return shares.ManagedShare{}, "", false
	}
	if principal.ID != "" && principal.ManagementRole != identity.RoleOwner && principal.ManagementRole != identity.RoleAdmin {
		level, accessErr := s.store.ResolveShareAccess(share.ID, principal.ID)
		if accessErr != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "share access could not be checked"})
			return shares.ManagedShare{}, "", false
		}
		if !store.CanAccess(level, required) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "this account does not have access to the selected share"})
			return shares.ManagedShare{}, "", false
		}
	}
	return share, username, true
}

func (s *apiServer) filePortalShares(w http.ResponseWriter, r *http.Request) {
	principal, _, ok := s.filePortalPrincipal(w, r)
	if !ok {
		return
	}
	sharesList, err := s.store.ListManagedShares()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "shares could not be loaded"})
		return
	}
	values := make([]map[string]any, 0, len(sharesList))
	for _, share := range sharesList {
		if !share.Enabled {
			continue
		}
		level := identity.AccessRead
		if principal.ID != "" {
			level, err = s.store.ResolveShareAccess(share.ID, principal.ID)
			if err != nil || !store.CanAccess(level, identity.AccessRead) {
				continue
			}
		}
		values = append(values, map[string]any{"id": share.ID, "name": share.Name, "description": share.Description, "access": level})
	}
	writeJSON(w, http.StatusOK, values)
}

func (s *apiServer) filePortalList(w http.ResponseWriter, r *http.Request) {
	share, _, ok := s.filePortalAccess(w, r, r.URL.Query().Get("shareId"), identity.AccessRead)
	if !ok {
		return
	}
	relative := r.URL.Query().Get("path")
	entries, err := fileops.List(share.Path, relative)
	if err != nil {
		writeFileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"shareId": share.ID, "path": pathOrRoot(relative), "entries": entries})
}

func (s *apiServer) filePortalDownload(w http.ResponseWriter, r *http.Request) {
	share, _, ok := s.filePortalAccess(w, r, r.URL.Query().Get("shareId"), identity.AccessRead)
	if !ok {
		return
	}
	filename := r.URL.Query().Get("name")
	target, info, err := fileops.ResolveEntry(share.Path, r.URL.Query().Get("path"), filename)
	if err != nil {
		writeFileError(w, err)
		return
	}
	if !info.Mode().IsRegular() {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "only regular files can be downloaded"})
		return
	}
	file, err := os.Open(target)
	if err != nil {
		writeFileError(w, err)
		return
	}
	defer file.Close()
	contentType := mime.TypeByExtension(filepath.Ext(filename))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(filename))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, filename, info.ModTime(), file)
}

func (s *apiServer) filePortalUpload(w http.ResponseWriter, r *http.Request) {
	share, actor, ok := s.filePortalAccess(w, r, r.URL.Query().Get("shareId"), identity.AccessWrite)
	if !ok {
		return
	}
	if r.ContentLength > maxFileRequestUploadBytes+(1<<20) {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "upload exceeds the 2 GiB per-file limit"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxFileRequestUploadBytes+(1<<20))
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "choose one file to upload"})
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "choose one file to upload"})
		return
	}
	defer file.Close()
	name := filepath.Base(strings.ReplaceAll(header.Filename, `\\`, "/"))
	stored, err := fileops.WriteUpload(share.Path, r.URL.Query().Get("path"), name, file, maxFileRequestUploadBytes)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "file could not be uploaded to this folder"})
		return
	}
	s.recordRequestAudit(r, actor, "file.portal.upload", share.ID, map[string]any{"name": stored, "sizeBytes": header.Size})
	writeJSON(w, http.StatusCreated, map[string]any{"name": stored, "sizeBytes": header.Size})
}

func (s *apiServer) filePortalSnapshots(w http.ResponseWriter, r *http.Request) {
	share, _, ok := s.filePortalAccess(w, r, r.URL.Query().Get("shareId"), identity.AccessRead)
	if !ok {
		return
	}
	values, err := s.store.StorageSnapshots(share.Path, 100)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "snapshot history could not be loaded"})
		return
	}
	writeJSON(w, http.StatusOK, values)
}

func (s *apiServer) filePortalSnapshotFiles(w http.ResponseWriter, r *http.Request, id string) {
	share, _, ok := s.filePortalAccess(w, r, r.URL.Query().Get("shareId"), identity.AccessRead)
	if !ok {
		return
	}
	record, found, err := s.store.StorageSnapshot(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "snapshot history could not be loaded"})
		return
	}
	if !found || filepath.Clean(record.Source) != filepath.Clean(share.Path) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "snapshot not found for this share"})
		return
	}
	s.storageSnapshotFiles(w, r, id)
}

func (s *apiServer) snapshotReadAllowed(w http.ResponseWriter, r *http.Request, source string) bool {
	principal, _, ok := s.filePortalPrincipal(w, r)
	if !ok {
		return false
	}
	if principal.ID == "" || principal.ManagementRole == identity.RoleOwner || principal.ManagementRole == identity.RoleAdmin {
		return true
	}
	sharesList, err := s.store.ListManagedShares()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "share access could not be checked"})
		return false
	}
	for _, share := range sharesList {
		if filepath.Clean(share.Path) != filepath.Clean(source) {
			continue
		}
		level, accessErr := s.store.ResolveShareAccess(share.ID, principal.ID)
		if accessErr == nil && store.CanAccess(level, identity.AccessRead) {
			return true
		}
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "this account does not have access to this snapshot"})
		return false
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "snapshot share not found"})
	return false
}

func (s *apiServer) filePortalRestoreActor(w http.ResponseWriter, r *http.Request, shareID string) (string, bool) {
	if !s.authEnabled() {
		return "local", true
	}
	cookie, err := r.Cookie("lumonas_session")
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
	if err != nil || !principal.Enabled || principal.Kind != identity.KindUser {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "an enabled NAS user account is required"})
		return "", false
	}
	if principal.ManagementRole == identity.RoleOwner || principal.ManagementRole == identity.RoleAdmin {
		return username, true
	}
	level, err := s.store.ResolveShareAccess(shareID, principal.ID)
	if err != nil || !store.CanAccess(level, identity.AccessWrite) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "write access to this share is required to restore a snapshot"})
		return "", false
	}
	return username, true
}
