package main

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/virtualization"
)

func (s *apiServer) virtualizationStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	writeJSON(w, http.StatusOK, s.virtualizationService.Status(r.Context()))
}

func (s *apiServer) virtualMachines(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	domains, err := s.virtualizationService.Domains(ctx)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "VM inventory is unavailable: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, domains)
}

func (s *apiServer) virtualMachineRecoverables(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	recoverables, err := s.virtualizationService.RecoverableDefinitions(ctx)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "saved VM recovery inventory is unavailable: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, recoverables)
}

func (s *apiServer) virtualMachineRestoreDefinition(w http.ResponseWriter, r *http.Request, name string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	domain, err := s.virtualizationService.RestoreDefinition(ctx, name)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "virtualization.vm.definition.restore", name, map[string]any{"state": domain.State})
	s.publishActor(actor, "virtualization.vm.definition.restored", "info", &model.ResourceRef{Type: "virtual-machine", ID: name}, map[string]any{"state": domain.State})
	writeJSON(w, http.StatusCreated, domain)
}

func (s *apiServer) virtualizationMedia(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	media, err := s.virtualizationService.Media()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, media)
}

func (s *apiServer) virtualizationUploadMedia(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, virtualization.MaxISOUploadBytes+(1<<20))
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid ISO upload or upload exceeds 8 GiB"})
		return
	}
	if r.MultipartForm != nil {
		defer func() { _ = r.MultipartForm.RemoveAll() }()
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ISO file is required"})
		return
	}
	defer file.Close()
	if header.Size > virtualization.MaxISOUploadBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "ISO uploads are limited to 8 GiB"})
		return
	}
	media, err := s.virtualizationService.StoreISO(filepath.Base(header.Filename), file)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "virtualization.iso.upload", media.Name, map[string]any{"size_bytes": media.SizeBytes})
	s.publishActor(actor, "virtualization.iso.uploaded", "info", nil, map[string]any{"name": media.Name, "sizeBytes": media.SizeBytes})
	writeJSON(w, http.StatusCreated, media)
}

func (s *apiServer) virtualMachineCreate(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input virtualization.CreateInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024)).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid VM request"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	created, err := s.virtualizationService.Create(ctx, input)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "virtualization.vm.create", created.Name, map[string]any{"vcpus": input.VCPUs, "memory_mib": input.MemoryMiB, "disk_gib": input.DiskGiB, "iso": input.ISO})
	s.publishActor(actor, "virtualization.vm.created", "info", &model.ResourceRef{Type: "virtual-machine", ID: created.Name}, map[string]any{"state": created.State, "iso": created.ISO})
	writeJSON(w, http.StatusCreated, created)
}

func (s *apiServer) virtualMachineDelete(w http.ResponseWriter, r *http.Request, name string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		ConfirmName string `json:"confirmName"`
		DeleteDisk  bool   `json:"deleteDisk"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8*1024)).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid VM deletion confirmation"})
		return
	}
	if input.ConfirmName != name {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "type the VM name exactly to confirm deletion"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	result, err := s.virtualizationService.Delete(ctx, name, input.DeleteDisk)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	action := "virtualization.vm.delete"
	if input.DeleteDisk {
		action = "virtualization.vm.delete_with_disk"
	}
	s.recordRequestAudit(r, actor, action, name, map[string]any{"diskPath": result.DiskPath, "diskRemoved": result.DiskRemoved})
	s.publishActor(actor, "virtualization.vm.deleted", "warning", &model.ResourceRef{Type: "virtual-machine", ID: name}, map[string]any{"diskRemoved": result.DiskRemoved, "diskPath": result.DiskPath})
	writeJSON(w, http.StatusOK, result)
}

func (s *apiServer) virtualMachineSnapshots(w http.ResponseWriter, r *http.Request, name string) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	snapshots, err := s.virtualizationService.Snapshots(ctx, name)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, snapshots)
}

func (s *apiServer) virtualMachineSnapshotCreate(w http.ResponseWriter, r *http.Request, name string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8*1024)).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid snapshot request"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	snapshot, err := s.virtualizationService.CreateSnapshot(ctx, name, input.Name)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "virtualization.snapshot.create", name, map[string]any{"snapshot": snapshot.Name})
	s.publishActor(actor, "virtualization.snapshot.created", "info", &model.ResourceRef{Type: "virtual-machine", ID: name}, map[string]any{"snapshot": snapshot.Name})
	writeJSON(w, http.StatusCreated, snapshot)
}

func (s *apiServer) virtualMachineSnapshotRevert(w http.ResponseWriter, r *http.Request, name, snapshotName string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	if err := s.virtualizationService.RevertSnapshot(ctx, name, snapshotName); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "virtualization.snapshot.revert", name, map[string]any{"snapshot": snapshotName})
	s.publishActor(actor, "virtualization.snapshot.reverted", "warning", &model.ResourceRef{Type: "virtual-machine", ID: name}, map[string]any{"snapshot": snapshotName})
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "reverting"})
}

func (s *apiServer) virtualMachineSnapshotDelete(w http.ResponseWriter, r *http.Request, name, snapshotName string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := s.virtualizationService.DeleteSnapshot(ctx, name, snapshotName); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "virtualization.snapshot.delete", name, map[string]any{"snapshot": snapshotName})
	s.publishActor(actor, "virtualization.snapshot.deleted", "warning", &model.ResourceRef{Type: "virtual-machine", ID: name}, map[string]any{"snapshot": snapshotName})
	w.WriteHeader(http.StatusNoContent)
}

func (s *apiServer) virtualMachineAction(w http.ResponseWriter, r *http.Request, name string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8*1024)).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	domain, err := s.virtualizationService.Action(ctx, name, input.Action)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "virtualization.vm."+input.Action, domain.Name, nil)
	s.publishActor(actor, "virtualization.vm.action", "info", &model.ResourceRef{Type: "virtual-machine", ID: domain.Name}, map[string]any{"action": input.Action, "state": domain.State})
	writeJSON(w, http.StatusAccepted, domain)
}

func (s *apiServer) virtualMachineConsole(w http.ResponseWriter, r *http.Request, name string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		cursor, err := strconv.ParseInt(r.URL.Query().Get("cursor"), 10, 64)
		if r.URL.Query().Get("cursor") == "" {
			cursor, err = 0, nil
		}
		if err != nil || cursor < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "console cursor is invalid"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		if err := s.virtualizationService.OpenConsole(ctx, name); err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		state, err := s.virtualizationService.ReadConsole(name, cursor)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
			return
		}
		if cursor == 0 {
			s.recordRequestAudit(r, actor, "virtualization.console.open", name, nil)
		}
		writeJSON(w, http.StatusOK, state)
	case http.MethodPost:
		var input struct {
			Data string `json:"data"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, virtualization.MaxConsoleInputBytes+1024)).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid console input"})
			return
		}
		if err := s.virtualizationService.WriteConsole(name, []byte(input.Data)); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "sent"})
	case http.MethodDelete:
		if err := s.virtualizationService.CloseConsole(name); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		s.recordRequestAudit(r, actor, "virtualization.console.close", name, nil)
		w.WriteHeader(http.StatusNoContent)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}
