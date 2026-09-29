package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/lumonas/lumonas/internal/uploads"
)

func (s *apiServer) uploadSessionManager() uploads.Manager {
	return uploads.Manager{Root: envOr("LUMONAS_UPLOAD_SESSION_DIR", filepath.Join(envOr("LUMONAS_DATA_DIR", "/var/lib/lumonas"), "uploads"))}
}

func (s *apiServer) createResumableUpload(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		ShareID         string     `json:"shareId"`
		Path            string     `json:"path"`
		Name            string     `json:"name"`
		SizeBytes       int64      `json:"sizeBytes"`
		ExpectedSHA256  string     `json:"expectedSha256"`
		ReplaceExisting bool       `json:"replaceExisting"`
		ExpectedSize    int64      `json:"expectedSizeBytes"`
		ExpectedModTime *time.Time `json:"expectedModifiedAt"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	share, err := s.fileShare(input.ShareID)
	if err != nil {
		writeFileError(w, err)
		return
	}
	if !s.workstationShareAllowed(r, share.ID) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "workstation backup token is bound to a different share"})
		return
	}
	options := uploads.CreateOptions{ReplaceExisting: input.ReplaceExisting, ExpectedSize: input.ExpectedSize}
	if input.ExpectedModTime != nil {
		options.ExpectedModTime = input.ExpectedModTime.UTC()
	}
	s.uploadMu.Lock()
	session, err := s.uploadSessionManager().CreateWithOptions(share.ID, share.Path, input.Path, input.Name, input.SizeBytes, input.ExpectedSHA256, options)
	s.uploadMu.Unlock()
	if err != nil {
		writeFileError(w, err)
		return
	}
	s.recordRequestAudit(r, actor, "file.upload.session.created", share.ID, map[string]any{"uploadId": session.ID, "name": session.Name, "sizeBytes": session.SizeBytes})
	writeJSON(w, http.StatusCreated, session)
}

func (s *apiServer) getResumableUpload(w http.ResponseWriter, r *http.Request, id string) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	s.uploadMu.Lock()
	session, err := s.uploadSessionManager().Get(id)
	s.uploadMu.Unlock()
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "upload session not found"})
		return
	}
	if !s.workstationShareAllowed(r, session.ShareID) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "workstation backup token is bound to a different share"})
		return
	}
	if _, err := s.fileShare(session.ShareID); err != nil {
		writeFileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, session)
}

func (s *apiServer) writeResumableUploadChunk(w http.ResponseWriter, r *http.Request, id string) {
	if _, ok := s.identityActor(w, r, true); !ok {
		return
	}
	offset, err := strconv.ParseInt(r.Header.Get("Upload-Offset"), 10, 64)
	if err != nil || offset < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Upload-Offset must be a non-negative byte offset"})
		return
	}
	if r.ContentLength > uploads.MaxChunkBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "upload chunk exceeds the 8 MiB limit"})
		return
	}
	s.uploadMu.Lock()
	manager := s.uploadSessionManager()
	session, err := manager.Get(id)
	if err == nil && !s.workstationShareAllowed(r, session.ShareID) {
		err = errors.New("workstation backup token is bound to a different share")
	}
	if err == nil {
		var sharePath string
		if share, shareErr := s.fileShare(session.ShareID); shareErr != nil {
			err = shareErr
		} else {
			sharePath = share.Path
			if filepath.Clean(sharePath) != filepath.Clean(session.ShareRoot) {
				err = errors.New("share path changed while the upload was in progress")
			}
		}
	}
	var duplicate bool
	if err == nil {
		session, duplicate, err = manager.WriteChunk(id, offset, io.LimitReader(r.Body, uploads.MaxChunkBytes+1))
	}
	s.uploadMu.Unlock()
	if err != nil {
		status := http.StatusUnprocessableEntity
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Upload-Offset", strconv.FormatInt(session.ReceivedBytes, 10))
	status := http.StatusOK
	if !duplicate {
		status = http.StatusNoContent
	}
	w.WriteHeader(status)
}

func (s *apiServer) completeResumableUpload(w http.ResponseWriter, r *http.Request, id string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	s.uploadMu.Lock()
	manager := s.uploadSessionManager()
	session, err := manager.Get(id)
	var name string
	if err == nil && !s.workstationShareAllowed(r, session.ShareID) {
		err = errors.New("workstation backup token is bound to a different share")
	}
	if err == nil {
		share, shareErr := s.fileShare(session.ShareID)
		if shareErr != nil {
			err = shareErr
		} else if filepath.Clean(share.Path) != filepath.Clean(session.ShareRoot) {
			err = errors.New("share path changed while the upload was in progress")
		} else {
			session, name, err = manager.Complete(id)
		}
	}
	s.uploadMu.Unlock()
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	job := s.queueFileJob(actor, requestCorrelationID(r), "upload file", session.ShareID, func() (map[string]any, error) {
		return map[string]any{"name": name, "sizeBytes": session.SizeBytes}, nil
	})
	s.recordRequestAudit(r, actor, "file.upload.completed", session.ShareID, map[string]any{"jobId": job.ID, "uploadId": id, "name": name, "sizeBytes": session.SizeBytes})
	writeJSON(w, http.StatusCreated, map[string]string{"jobId": job.ID, "name": name})
}

func (s *apiServer) cancelResumableUpload(w http.ResponseWriter, r *http.Request, id string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	s.uploadMu.Lock()
	manager := s.uploadSessionManager()
	session, err := manager.Get(id)
	if err == nil && !s.workstationShareAllowed(r, session.ShareID) {
		err = errors.New("workstation backup token is bound to a different share")
	}
	if err == nil {
		err = manager.Cancel(id)
	}
	s.uploadMu.Unlock()
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "upload session not found"})
		return
	}
	s.recordRequestAudit(r, actor, "file.upload.cancelled", id, nil)
	w.WriteHeader(http.StatusNoContent)
}
