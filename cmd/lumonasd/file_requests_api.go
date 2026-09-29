package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	fileops "github.com/lumonas/lumonas/internal/files"
	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/store"
)

const maxFileRequestUploadBytes = int64(2 << 30)

func hashFileRequestToken(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func activeFileRequest(value store.FileRequest, now time.Time) bool {
	return value.RevokedAt == nil && value.ExpiresAt.After(now)
}

func (s *apiServer) listFileRequests(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	share, err := s.fileShare(r.URL.Query().Get("shareId"))
	if err != nil {
		writeFileError(w, err)
		return
	}
	values, err := s.store.FileRequests(share.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, values)
}

func (s *apiServer) createFileRequest(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		ShareID        string `json:"shareId"`
		Path           string `json:"path"`
		ExpiresInHours int    `json:"expiresInHours"`
		MaxFiles       int    `json:"maxFiles"`
		MaxBytes       int64  `json:"maxBytes"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if input.ExpiresInHours < 1 || input.ExpiresInHours > 720 || input.MaxFiles < 1 || input.MaxFiles > 1000 || input.MaxBytes < 1 || input.MaxBytes > 1<<40 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "expiration must be 1–720 hours, file limit 1–1000, and byte limit 1 byte–1 TiB"})
		return
	}
	share, err := s.fileShare(input.ShareID)
	if err != nil {
		writeFileError(w, err)
		return
	}
	directory, err := fileops.Resolve(share.Path, input.Path)
	if err != nil {
		writeFileError(w, err)
		return
	}
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "file requests must target an existing directory"})
		return
	}
	rawToken := make([]byte, 32)
	if _, err := rand.Read(rawToken); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create secure link"})
		return
	}
	token := base64.RawURLEncoding.EncodeToString(rawToken)
	now := time.Now().UTC()
	value := store.FileRequest{ID: newID("file-request"), ShareID: share.ID, Path: input.Path, TokenHash: hashFileRequestToken(token), ExpiresAt: now.Add(time.Duration(input.ExpiresInHours) * time.Hour), MaxFiles: input.MaxFiles, MaxBytes: input.MaxBytes}
	if err := s.store.CreateFileRequest(value); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	value.TokenHash = ""
	link := "/request/" + token
	s.recordRequestAudit(r, actor, "file.request.create", value.ID, map[string]any{"shareId": share.ID, "maxFiles": input.MaxFiles, "maxBytes": input.MaxBytes, "expiresAt": value.ExpiresAt})
	s.publishActor(actor, "file.request.created", "info", &model.ResourceRef{Type: "file-request", ID: value.ID}, nil)
	writeJSON(w, http.StatusCreated, map[string]any{"request": value, "url": link})
}

func (s *apiServer) revokeFileRequest(w http.ResponseWriter, r *http.Request, id string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	s.fileRequestMu.Lock()
	err := s.store.RevokeFileRequest(id, time.Now())
	s.fileRequestMu.Unlock()
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file request not found or already revoked"})
		return
	}
	s.recordRequestAudit(r, actor, "file.request.revoke", id, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *apiServer) lookupPublicFileRequest(w http.ResponseWriter, token string) (store.FileRequest, bool) {
	if len(token) < 40 || len(token) > 64 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file request not found"})
		return store.FileRequest{}, false
	}
	value, err := s.store.FileRequestByTokenHash(hashFileRequestToken(token))
	if err != nil || !activeFileRequest(value, time.Now()) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file request not found or expired"})
		return store.FileRequest{}, false
	}
	return value, true
}

func (s *apiServer) publicFileRequest(w http.ResponseWriter, r *http.Request, token string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	value, ok := s.lookupPublicFileRequest(w, token)
	if !ok {
		return
	}
	share, err := s.fileShare(value.ShareID)
	if err != nil {
		writeJSON(w, http.StatusGone, map[string]string{"error": "the requested destination is no longer available"})
		return
	}
	remainingBytes := value.MaxBytes - value.ReceivedBytes
	if remainingBytes < 0 {
		remainingBytes = 0
	}
	writeJSON(w, http.StatusOK, map[string]any{"shareName": share.Name, "expiresAt": value.ExpiresAt, "maxFiles": value.MaxFiles, "remainingFiles": value.MaxFiles - value.ReceivedFiles, "maxBytes": value.MaxBytes, "remainingBytes": remainingBytes})
}

func (s *apiServer) uploadPublicFileRequest(w http.ResponseWriter, r *http.Request, token string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	s.fileRequestMu.Lock()
	defer s.fileRequestMu.Unlock()
	value, ok := s.lookupPublicFileRequest(w, token)
	if !ok {
		return
	}
	if r.ContentLength > maxFileRequestUploadBytes+(1<<20) {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "upload exceeds the 2 GiB per-file limit"})
		return
	}
	remaining := value.MaxBytes - value.ReceivedBytes
	if remaining <= 0 || value.ReceivedFiles >= value.MaxFiles {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "this upload request has reached its limit"})
		return
	}
	if remaining > maxFileRequestUploadBytes {
		remaining = maxFileRequestUploadBytes
	}
	r.Body = http.MaxBytesReader(w, r.Body, remaining+(1<<20))
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
	if header.Size > remaining {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "file is larger than the remaining request allowance"})
		return
	}
	share, err := s.fileShare(value.ShareID)
	if err != nil {
		writeJSON(w, http.StatusGone, map[string]string{"error": "the requested destination is no longer available"})
		return
	}
	name := filepath.Base(strings.ReplaceAll(header.Filename, `\`, "/"))
	name, err = fileops.WriteUpload(share.Path, value.Path, name, file, remaining)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "the file could not be stored; ask the NAS owner to check the destination"})
		return
	}
	target, err := fileops.Resolve(share.Path, filepath.Join(value.Path, name))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "uploaded file path could not be verified"})
		return
	}
	info, err := os.Stat(target)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "the upload could not be verified; ask the NAS owner to check storage"})
		return
	}
	if err := s.store.RecordFileRequestUpload(value.ID, info.Size(), time.Now()); err != nil {
		_ = os.Remove(target)
		writeJSON(w, http.StatusConflict, map[string]string{"error": "this upload request has reached its limit or expired"})
		return
	}
	s.publish("file.request.uploaded", "info", &model.ResourceRef{Type: "share", ID: share.ID}, map[string]any{"requestId": value.ID, "bytes": info.Size()})
	writeJSON(w, http.StatusCreated, map[string]any{"name": name, "sizeBytes": info.Size(), "remainingFiles": value.MaxFiles - value.ReceivedFiles - 1, "remainingBytes": remaining - info.Size()})
}
