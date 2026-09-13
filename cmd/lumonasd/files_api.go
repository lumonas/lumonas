package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	fileops "github.com/lumonas/lumonas/internal/files"
	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/shares"
)

type filePathRequest struct {
	ShareID            string `json:"shareId"`
	Path               string `json:"path"`
	ExpectedGeneration *int64 `json:"expectedGeneration"`
}

type fileJobTask func() (map[string]any, error)

func (s *apiServer) fileShare(id string) (shares.ManagedShare, error) {
	if strings.TrimSpace(id) == "" {
		return shares.ManagedShare{}, errors.New("shareId is required")
	}
	share, err := s.store.ManagedShare(id)
	if err != nil {
		return shares.ManagedShare{}, os.ErrNotExist
	}
	if !share.Enabled {
		return shares.ManagedShare{}, errors.New("share is disabled")
	}
	return share, nil
}

func (s *apiServer) listFiles(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	share, err := s.fileShare(r.URL.Query().Get("share"))
	if err != nil {
		writeFileError(w, err)
		return
	}
	requested := r.URL.Query().Get("path")
	entries, err := fileops.List(share.Path, requested)
	if err != nil {
		writeFileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"shareId": share.ID, "path": pathOrRoot(requested), "entries": entries})
}

func (s *apiServer) searchFiles(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	share, err := s.fileShare(r.URL.Query().Get("share"))
	if err != nil {
		writeFileError(w, err)
		return
	}
	entries, err := fileops.Search(share.Path, r.URL.Query().Get("path"), r.URL.Query().Get("q"))
	if err != nil {
		writeFileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"shareId": share.ID, "entries": entries})
}

func (s *apiServer) fileProperties(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	share, err := s.fileShare(r.URL.Query().Get("share"))
	if err != nil {
		writeFileError(w, err)
		return
	}
	path, info, err := fileops.ResolveEntry(share.Path, r.URL.Query().Get("path"), r.URL.Query().Get("name"))
	if err != nil {
		writeFileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"shareId": share.ID, "path": path, "name": info.Name(), "type": fileEntryType(info), "sizeBytes": fileEntrySize(info, path), "modifiedAt": info.ModTime().UTC()})
}

func (s *apiServer) downloadFile(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	share, err := s.fileShare(r.URL.Query().Get("share"))
	if err != nil {
		writeFileError(w, err)
		return
	}
	path, info, err := fileops.ResolveEntry(share.Path, r.URL.Query().Get("path"), r.URL.Query().Get("name"))
	if err != nil {
		writeFileError(w, err)
		return
	}
	if info.IsDir() {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "directories cannot be downloaded by this endpoint"})
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename=\""+info.Name()+"\"")
	http.ServeFile(w, r, path)
}

func (s *apiServer) makeDirectory(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		filePathRequest
		Name string `json:"name"`
	}
	if !decodeFileJSON(w, r, &input) {
		return
	}
	if !s.expectedIdentityGeneration(w, input.ExpectedGeneration) {
		return
	}
	share, err := s.fileShare(input.ShareID)
	if err == nil {
		err = fileops.MakeDir(share.Path, input.Path, input.Name)
	}
	if err != nil {
		writeFileError(w, err)
		return
	}
	s.recordRequestAudit(r, actor, "file.mkdir", share.ID, map[string]any{"path": input.Path, "name": input.Name})
	s.publish("file.changed", "info", &model.ResourceRef{Type: "share", ID: share.ID}, map[string]any{"operation": "mkdir", "path": input.Path, "name": input.Name})
	writeJSON(w, http.StatusCreated, map[string]bool{"ok": true})
}

func (s *apiServer) renameFile(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		filePathRequest
		OldName string `json:"oldName"`
		NewName string `json:"newName"`
	}
	if !decodeFileJSON(w, r, &input) {
		return
	}
	if !s.expectedIdentityGeneration(w, input.ExpectedGeneration) {
		return
	}
	share, err := s.fileShare(input.ShareID)
	if err == nil {
		err = fileops.Rename(share.Path, input.Path, input.OldName, input.NewName)
	}
	if err != nil {
		writeFileError(w, err)
		return
	}
	s.recordRequestAudit(r, actor, "file.rename", share.ID, map[string]any{"path": input.Path, "oldName": input.OldName, "newName": input.NewName})
	s.publish("file.changed", "info", &model.ResourceRef{Type: "share", ID: share.ID}, map[string]any{"operation": "rename", "path": input.Path, "oldName": input.OldName, "newName": input.NewName})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *apiServer) deleteFiles(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	idempotencyKey, replayed := s.fileJobIdempotency(w, r, "file.delete")
	if replayed {
		return
	}
	var input struct {
		filePathRequest
		Names               []string `json:"names"`
		ConfirmDependencies bool     `json:"confirmDependencies"`
	}
	if !decodeFileJSON(w, r, &input) {
		return
	}
	if !s.expectedIdentityGeneration(w, input.ExpectedGeneration) {
		return
	}
	share, err := s.fileShare(input.ShareID)
	if err != nil {
		writeFileError(w, err)
		return
	}
	if warning := dockerDependencyWarning(share.Path, input.Path, input.Names); warning != "" && !input.ConfirmDependencies {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "explicit confirmation is required before deleting Docker appdata", "dependencyWarning": warning})
		return
	}
	job := s.queueFileJob(actor, requestCorrelationID(r), "delete files", share.ID, func() (map[string]any, error) {
		deleted, err := fileops.Delete(share.Path, share.ID, input.Path, input.Names)
		if err == nil {
			s.publishActor(actor, "file.changed", "info", &model.ResourceRef{Type: "share", ID: share.ID}, map[string]any{"operation": "delete", "path": input.Path, "deleted": deleted})
		}
		return map[string]any{"deleted": deleted}, err
	})
	s.recordRequestAudit(r, actor, "file.delete.queued", share.ID, map[string]any{"jobId": job.ID, "path": input.Path, "count": len(input.Names)})
	s.rememberFileJob("file.delete", idempotencyKey, job.ID)
	writeJSON(w, http.StatusAccepted, map[string]any{"jobId": job.ID, "deleted": 0})
}

func (s *apiServer) transferFiles(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	idempotencyKey, replayed := s.fileJobIdempotency(w, r, "file.transfer")
	if replayed {
		return
	}
	var input struct {
		ShareID            string   `json:"shareId"`
		SourcePath         string   `json:"sourcePath"`
		Names              []string `json:"names"`
		TargetShareID      string   `json:"targetShareId"`
		TargetPath         string   `json:"targetPath"`
		Operation          string   `json:"op"`
		Conflict           string   `json:"conflict"`
		ExpectedGeneration *int64   `json:"expectedGeneration"`
	}
	if !decodeFileJSON(w, r, &input) {
		return
	}
	if !s.expectedIdentityGeneration(w, input.ExpectedGeneration) {
		return
	}
	if input.Operation != "copy" && input.Operation != "move" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "op must be copy or move"})
		return
	}
	source, err := s.fileShare(input.ShareID)
	if err != nil {
		writeFileError(w, err)
		return
	}
	target, err := s.fileShare(input.TargetShareID)
	if err != nil {
		writeFileError(w, err)
		return
	}
	taskInput := fileops.TransferInput{SourceRoot: source.Path, SourcePath: input.SourcePath, Names: input.Names, TargetRoot: target.Path, TargetPath: input.TargetPath, Operation: input.Operation, Conflict: input.Conflict}
	conflicts, err := fileops.Conflicts(taskInput)
	if err != nil {
		writeFileError(w, err)
		return
	}
	if len(conflicts) > 0 && input.Conflict == "" {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "target names already exist", "conflicts": conflicts})
		return
	}
	strategy := "copy"
	if input.Operation == "move" {
		strategy = "rename"
		if filepath.Clean(source.Path) != filepath.Clean(target.Path) {
			strategy = "copy-delete"
		}
	}
	job := s.queueFileJob(actor, requestCorrelationID(r), input.Operation+" files", target.ID, func() (map[string]any, error) {
		count, err := fileops.Transfer(taskInput)
		return map[string]any{"transferred": count, "strategy": strategy}, err
	})
	s.recordRequestAudit(r, actor, "file.transfer.queued", target.ID, map[string]any{"jobId": job.ID, "operation": input.Operation, "strategy": strategy, "count": len(input.Names)})
	s.rememberFileJob("file.transfer", idempotencyKey, job.ID)
	writeJSON(w, http.StatusAccepted, map[string]any{"jobId": job.ID, "transferred": 0, "strategy": strategy})
}

func (s *apiServer) uploadFile(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	idempotencyKey, replayed := s.fileJobIdempotency(w, r, "file.upload")
	if replayed {
		return
	}
	var input struct {
		filePathRequest
		Name      string `json:"name"`
		SizeBytes int64  `json:"sizeBytes"`
	}
	if strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data") {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart upload"})
			return
		}
		input.ShareID, input.Path, input.Name = r.FormValue("shareId"), r.FormValue("path"), r.FormValue("name")
		file, header, err := r.FormFile("file")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file part is required"})
			return
		}
		defer file.Close()
		if input.Name == "" {
			input.Name = header.Filename
		}
		input.SizeBytes = header.Size
		share, err := s.fileShare(input.ShareID)
		if err != nil {
			writeFileError(w, err)
			return
		}
		name, err := fileops.WriteUpload(share.Path, input.Path, input.Name, file, 1<<40)
		if err != nil {
			writeFileError(w, err)
			return
		}
		job := s.queueFileJob(actor, requestCorrelationID(r), "upload file", share.ID, func() (map[string]any, error) { return map[string]any{"name": name, "sizeBytes": input.SizeBytes}, nil })
		s.recordRequestAudit(r, actor, "file.upload.queued", share.ID, map[string]any{"jobId": job.ID, "name": name, "sizeBytes": input.SizeBytes})
		s.rememberFileJob("file.upload", idempotencyKey, job.ID)
		writeJSON(w, http.StatusAccepted, map[string]string{"jobId": job.ID, "name": name})
		return
	}
	if !decodeFileJSON(w, r, &input) {
		return
	}
	if !s.expectedIdentityGeneration(w, input.ExpectedGeneration) {
		return
	}
	share, err := s.fileShare(input.ShareID)
	if err != nil {
		writeFileError(w, err)
		return
	}
	name, err := fileops.Upload(share.Path, input.Path, input.Name, input.SizeBytes)
	if err != nil {
		writeFileError(w, err)
		return
	}
	job := s.queueFileJob(actor, requestCorrelationID(r), "upload file", share.ID, func() (map[string]any, error) {
		return map[string]any{"name": name, "sizeBytes": input.SizeBytes}, nil
	})
	s.recordRequestAudit(r, actor, "file.upload.queued", share.ID, map[string]any{"jobId": job.ID, "name": name, "sizeBytes": input.SizeBytes})
	s.rememberFileJob("file.upload", idempotencyKey, job.ID)
	writeJSON(w, http.StatusAccepted, map[string]string{"jobId": job.ID, "name": name})
}

func (s *apiServer) listRecycleBin(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	share, err := s.fileShare(r.URL.Query().Get("share"))
	if err != nil {
		writeFileError(w, err)
		return
	}
	entries, err := fileops.ListTrash(share.Path, share.ID)
	if err != nil {
		writeFileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (s *apiServer) restoreRecycleBin(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		ID      string `json:"id"`
		ShareID string `json:"shareId"`
	}
	if !decodeFileJSON(w, r, &input) {
		return
	}
	share, err := s.findRecycleShare(input.ShareID, input.ID)
	if err != nil {
		writeFileError(w, err)
		return
	}
	restored, err := fileops.Restore(share.Path, input.ID)
	if err != nil {
		writeFileError(w, err)
		return
	}
	if !restored {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "recycle-bin entry not found"})
		return
	}
	s.recordRequestAudit(r, actor, "file.recycle.restore", share.ID, map[string]any{"id": input.ID})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *apiServer) purgeRecycleBin(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		ID      string `json:"id"`
		ShareID string `json:"shareId"`
	}
	if !decodeFileJSON(w, r, &input) {
		return
	}
	if input.ID != "" {
		share, err := s.findRecycleShare(input.ShareID, input.ID)
		if err != nil {
			writeFileError(w, err)
			return
		}
		removed, err := fileops.Purge(share.Path, input.ID)
		if err != nil {
			writeFileError(w, err)
			return
		}
		if !removed {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "recycle-bin entry not found"})
			return
		}
		s.recordRequestAudit(r, actor, "file.recycle.purge", share.ID, map[string]any{"id": input.ID})
		writeJSON(w, http.StatusOK, map[string]int{"purged": 1})
		return
	}
	share, err := s.fileShare(input.ShareID)
	if err != nil {
		writeFileError(w, err)
		return
	}
	purged, err := fileops.PurgeAll(share.Path, share.ID)
	if err != nil {
		writeFileError(w, err)
		return
	}
	s.recordRequestAudit(r, actor, "file.recycle.purge", share.ID, map[string]any{"purged": purged})
	writeJSON(w, http.StatusOK, map[string]int{"purged": purged})
}

func (s *apiServer) findRecycleShare(shareID, id string) (shares.ManagedShare, error) {
	if shareID != "" {
		share, err := s.fileShare(shareID)
		if err != nil {
			return shares.ManagedShare{}, err
		}
		entries, err := fileops.ListTrash(share.Path, share.ID)
		if err != nil {
			return shares.ManagedShare{}, err
		}
		for _, entry := range entries {
			if entry.ID == id {
				return share, nil
			}
		}
		return shares.ManagedShare{}, os.ErrNotExist
	}
	values, err := s.store.ListManagedShares()
	if err != nil {
		return shares.ManagedShare{}, err
	}
	for _, share := range values {
		if !share.Enabled {
			continue
		}
		entries, listErr := fileops.ListTrash(share.Path, share.ID)
		if listErr != nil {
			continue
		}
		for _, entry := range entries {
			if entry.ID == id {
				return share, nil
			}
		}
	}
	return shares.ManagedShare{}, os.ErrNotExist
}

func (s *apiServer) queueFileJob(actor, correlationID, title, resourceID string, task fileJobTask) model.Job {
	now := time.Now().UTC()
	job := model.Job{ID: newID("job"), CorrelationID: correlationID, Actor: actor, Type: "file.transfer", Title: title, ResourceID: resourceID, State: "queued", Generation: s.currentGeneration(), CreatedAt: now}
	if err := s.store.SaveJob(job); err != nil {
		job.State, job.Error = "failed", err.Error()
		return job
	}
	s.publish("job.state_changed", "info", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job})
	go func(job model.Job) {
		started := time.Now().UTC()
		job.State, job.Stage, job.StartedAt, job.Progress = "running", "Processing filesystem operation", &started, float64Ptr(10)
		_ = s.store.SaveJob(job)
		data, err := task()
		finished := time.Now().UTC()
		job.FinishedAt, job.Progress = &finished, float64Ptr(100)
		if err != nil {
			job.State, job.Stage, job.Error = "failed", "Filesystem operation failed", err.Error()
			s.publish("job.state_changed", "warning", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job})
			_ = s.store.SaveJob(job)
			return
		}
		job.State, job.Stage = "successful", "Filesystem operation completed"
		_ = s.store.SaveJob(job)
		s.publish("job.state_changed", "info", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job, "result": data})
	}(job)
	return job
}

func decodeFileJSON(w http.ResponseWriter, r *http.Request, value any) bool {
	if err := json.NewDecoder(r.Body).Decode(value); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return false
	}
	return true
}

func pathOrRoot(value string) string {
	if strings.TrimSpace(value) == "" {
		return "/"
	}
	return value
}

func writeFileError(w http.ResponseWriter, err error) {
	var conflict *fileops.ConflictError
	switch {
	case errors.As(err, &conflict):
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "conflicts": conflict.Names})
	case errors.Is(err, os.ErrNotExist):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file or directory not found"})
	case errors.Is(err, os.ErrExist):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "file or directory already exists"})
	default:
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
	}
}

func float64Ptr(value float64) *float64 { return &value }

func (s *apiServer) fileJobIdempotency(w http.ResponseWriter, r *http.Request, scope string) (string, bool) {
	key, err := requestIdempotencyKey(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return "", true
	}
	if key == "" {
		return "", false
	}
	if jobID, found := s.store.Meta(idempotencyMetaKey(scope, key)); found {
		if job, jobErr := s.store.Job(jobID); jobErr == nil {
			writeJSON(w, http.StatusOK, map[string]any{"jobId": job.ID, "state": job.State})
			return key, true
		}
	}
	return key, false
}

func (s *apiServer) rememberFileJob(scope, key, jobID string) {
	if key != "" {
		_ = s.store.SetMeta(idempotencyMetaKey(scope, key), jobID)
	}
}

func fileEntryType(info os.FileInfo) string {
	if info.IsDir() {
		return "dir"
	}
	return "file"
}

func fileEntrySize(info os.FileInfo, path string) int64 {
	if !info.IsDir() {
		return info.Size()
	}
	var total int64
	_ = filepath.Walk(path, func(_ string, entry os.FileInfo, err error) error {
		if err == nil && entry != nil && !entry.IsDir() {
			total += entry.Size()
		}
		return nil
	})
	return total
}

func dockerDependencyWarning(root, relative string, names []string) string {
	appdataRoot := envOr("LUMONAS_DOCKER_APPDATA_ROOT", "")
	stackRoot := envOr("LUMONAS_STACK_ROOT", "")
	if appdataRoot == "" && stackRoot == "" {
		return ""
	}
	parent := filepath.Join(root, filepath.Clean("/"+strings.TrimPrefix(relative, "/")))
	for _, name := range names {
		candidate := filepath.Join(parent, name)
		for _, dependencyRoot := range []string{appdataRoot, stackRoot} {
			if dependencyRoot == "" {
				continue
			}
			relative, err := filepath.Rel(filepath.Clean(dependencyRoot), filepath.Clean(candidate))
			if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
				return candidate + " is referenced by Docker application data"
			}
		}
	}
	return ""
}
