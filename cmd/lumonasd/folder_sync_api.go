package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/backup"
	"github.com/lumonas/lumonas/internal/foldersync"
	"github.com/lumonas/lumonas/internal/model"
)

func (s *apiServer) folderSyncRoutes(w http.ResponseWriter, r *http.Request, endpoint string) bool {
	if endpoint == "/folder-sync/tasks" && r.Method == http.MethodGet {
		if _, ok := s.identityActor(w, r, false); !ok {
			return true
		}
		values, err := s.store.FolderSyncTasks()
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
		} else {
			writeJSON(w, 200, values)
		}
		return true
	}
	if endpoint == "/folder-sync/tasks" && r.Method == http.MethodPost {
		s.saveFolderSyncTask(w, r, "")
		return true
	}
	if !strings.HasPrefix(endpoint, "/folder-sync/tasks/") {
		return false
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(endpoint, "/folder-sync/tasks/"), "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return false
	}
	id := parts[0]
	if len(parts) == 1 && r.Method == http.MethodPut {
		s.saveFolderSyncTask(w, r, id)
		return true
	}
	if len(parts) == 1 && r.Method == http.MethodDelete {
		s.deleteFolderSyncTask(w, r, id)
		return true
	}
	if len(parts) == 2 && parts[1] == "preview" && r.Method == http.MethodPost {
		s.previewFolderSync(w, r, id)
		return true
	}
	if len(parts) == 2 && parts[1] == "run" && r.Method == http.MethodPost {
		s.startFolderSync(w, r, id)
		return true
	}
	if len(parts) == 2 && parts[1] == "runs" && r.Method == http.MethodGet {
		if _, ok := s.identityActor(w, r, false); !ok {
			return true
		}
		runs, err := s.store.FolderSyncRuns(id, 100)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
		} else {
			writeJSON(w, 200, runs)
		}
		return true
	}
	if len(parts) == 3 && parts[1] == "runs" && parts[2] == "cancel" && r.Method == http.MethodPost {
		s.cancelFolderSync(w, r, id)
		return true
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "folder sync endpoint not found"})
	return true
}

func (s *apiServer) saveFolderSyncTask(w http.ResponseWriter, r *http.Request, id string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var task foldersync.Task
	if json.NewDecoder(r.Body).Decode(&task) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
		return
	}
	if id == "" {
		id = newID("sync")
	}
	task.ID = id
	if task.ScheduleKind == "" {
		task.ScheduleKind = "manual"
	}
	if task.Mode == "" {
		task.Mode = "copy"
	}
	if task.Direction == "" {
		task.Direction = "one-way"
	}
	if err := task.Validate(); err != nil {
		writeJSON(w, 422, map[string]string{"error": err.Error()})
		return
	}
	if previous, previousErr := s.store.FolderSyncTask(id); previousErr == nil {
		if previous.Direction != task.Direction || previous.Source != task.Source || previous.Destination != task.Destination || previous.DeepCheck != task.DeepCheck {
			if err := s.store.DeleteFolderSyncBaseline(id); err != nil {
				writeJSON(w, 500, map[string]string{"error": "could not reset sync history for the updated task"})
				return
			}
		}
	}
	if task.Source.Kind == "destination" {
		if _, _, err := s.syncDestination(task.Source.DestinationID); err != nil {
			writeJSON(w, 422, map[string]string{"error": "source: " + err.Error()})
			return
		}
	} else if _, err := s.resolveSyncEndpoint(task.Source); err != nil {
		writeJSON(w, 422, map[string]string{"error": "source: " + err.Error()})
		return
	}
	if task.OnUSBAttach && task.Destination.Kind == "mount" {
		entries, mountErr := s.store.MountEntries()
		configured := false
		for _, entry := range entries {
			if entry.Enabled && entry.Kind == "disk" && entry.MountPath == task.Destination.MountPath {
				configured = true
				break
			}
		}
		if mountErr != nil || !configured {
			writeJSON(w, 422, map[string]string{"error": "destination: USB destination must be a configured managed disk mount"})
			return
		}
	} else if task.Destination.Kind != "destination" {
		if _, err := s.resolveSyncEndpoint(task.Destination); err != nil {
			writeJSON(w, 422, map[string]string{"error": "destination: " + err.Error()})
			return
		}
	}
	if task.Destination.Kind == "destination" {
		if _, _, err := s.syncDestination(task.Destination.DestinationID); err != nil {
			writeJSON(w, 422, map[string]string{"error": err.Error()})
			return
		}
	}
	if task.ScheduleKind == "manual" {
		task.Enabled = task.OnUSBAttach
	}
	if err := s.store.SaveFolderSyncTask(task); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "folder_sync.task.save", id, map[string]any{"mode": task.Mode})
	s.publishActor(actor, "folder_sync.task.updated", "info", &model.ResourceRef{Type: "folder-sync-task", ID: id}, nil)
	saved, _ := s.store.FolderSyncTask(id)
	writeJSON(w, 200, saved)
}

func (s *apiServer) deleteFolderSyncTask(w http.ResponseWriter, r *http.Request, id string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	s.folderSyncMu.Lock()
	running := s.folderSyncRunning[id] != nil
	s.folderSyncMu.Unlock()
	if running {
		writeJSON(w, 409, map[string]string{"error": "task has an active run"})
		return
	}
	if err := s.store.DeleteFolderSyncTask(id); err != nil {
		writeJSON(w, 404, map[string]string{"error": "sync task not found"})
		return
	}
	s.recordRequestAudit(r, actor, "folder_sync.task.delete", id, nil)
	w.WriteHeader(204)
}

func (s *apiServer) previewFolderSync(w http.ResponseWriter, r *http.Request, id string) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	task, err := s.store.FolderSyncTask(id)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "sync task not found"})
		return
	}
	plan, hash, err := s.folderSyncPlan(r.Context(), task)
	if err != nil {
		writeJSON(w, 422, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"plan": plan, "planHash": hash, "requiresConfirmation": task.Mode == "mirror" || task.Direction == "two-way"})
}

func (s *apiServer) folderSyncPlan(ctx context.Context, task foldersync.Task) (foldersync.Plan, string, error) {
	if task.Direction == "two-way" {
		leftRoot, err := s.resolveSyncEndpoint(task.Source)
		if err != nil {
			return foldersync.Plan{}, "", err
		}
		rightRoot, err := s.resolveSyncEndpoint(task.Destination)
		if err != nil {
			return foldersync.Plan{}, "", err
		}
		if err := foldersync.CheckRoots(leftRoot, rightRoot); err != nil {
			return foldersync.Plan{}, "", err
		}
		left, err := foldersync.ScanFiltered(leftRoot, task.DeepCheck, task.IgnorePatterns)
		if err != nil {
			return foldersync.Plan{}, "", err
		}
		right, err := foldersync.ScanFiltered(rightRoot, task.DeepCheck, task.IgnorePatterns)
		if err != nil {
			return foldersync.Plan{}, "", err
		}
		baseline, initialized, err := s.store.FolderSyncBaseline(task.ID)
		if err != nil {
			return foldersync.Plan{}, "", err
		}
		plan := foldersync.BuildTwoWayPlan(left, right, baseline, initialized, task.DeepCheck)
		encoded, _ := json.Marshal(struct {
			Task foldersync.Task
			Plan foldersync.Plan
		}{task, plan})
		digest := sha256.Sum256(encoded)
		return plan, hex.EncodeToString(digest[:]), nil
	}
	var source map[string]foldersync.Entry
	var err error
	if task.Source.Kind == "destination" {
		remote, credentials, resolveErr := s.syncDestination(task.Source.DestinationID)
		if resolveErr != nil {
			return foldersync.Plan{}, "", resolveErr
		}
		source, err = remoteSyncManifest(ctx, remote, credentials, task.Source.Prefix, task.DeepCheck, backup.ListRemote, backup.Download)
		if err != nil {
			return foldersync.Plan{}, "", err
		}
	} else {
		src, resolveErr := s.resolveSyncEndpoint(task.Source)
		if resolveErr != nil {
			return foldersync.Plan{}, "", resolveErr
		}
		source, err = foldersync.ScanFiltered(src, task.DeepCheck, task.IgnorePatterns)
		if err != nil {
			return foldersync.Plan{}, "", err
		}
	}
	source = foldersync.FilterEntries(source, task.IgnorePatterns)
	destination := map[string]foldersync.Entry{}
	if task.Destination.Kind == "destination" {
		remote, credentials, resolveErr := s.syncDestination(task.Destination.DestinationID)
		if resolveErr != nil {
			return foldersync.Plan{}, "", resolveErr
		}
		// Mirror plans must reflect the current remote prefix. A saved local
		// manifest can miss files added or changed directly at the destination.
		destination, err = remoteSyncManifest(ctx, remote, credentials, task.Destination.Prefix, task.DeepCheck, backup.ListRemote, backup.Download)
		if err != nil {
			return foldersync.Plan{}, "", err
		}
	} else {
		dst, resolveErr := s.resolveSyncEndpoint(task.Destination)
		if resolveErr != nil {
			return foldersync.Plan{}, "", resolveErr
		}
		destination, err = foldersync.ScanFiltered(dst, task.DeepCheck, task.IgnorePatterns)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return foldersync.Plan{}, "", err
		}
	}
	destination = foldersync.FilterEntries(destination, task.IgnorePatterns)
	plan := foldersync.BuildPlan(source, destination, task.Mode == "mirror", task.DeepCheck)
	encoded, _ := json.Marshal(struct {
		Task foldersync.Task
		Plan foldersync.Plan
	}{task, plan})
	digest := sha256.Sum256(encoded)
	return plan, hex.EncodeToString(digest[:]), nil
}

type remoteListFunc func(context.Context, backup.Destination, backup.Credentials, string) ([]backup.RemoteFile, error)
type remoteDownloadFunc func(context.Context, backup.Destination, backup.Credentials, string, string) error

func remoteSyncManifest(ctx context.Context, remote backup.Destination, credentials backup.Credentials, prefix string, deep bool, list remoteListFunc, download remoteDownloadFunc) (map[string]foldersync.Entry, error) {
	files, err := list(ctx, remote, credentials, prefix)
	if err != nil {
		return nil, err
	}
	result := make(map[string]foldersync.Entry, len(files))
	for _, file := range files {
		name := filepath.ToSlash(file.Path)
		if name == "" || name == "." || path.IsAbs(name) || path.Clean(name) != name || name == ".." || strings.HasPrefix(name, "../") || strings.ContainsAny(name, "\\\x00\r\n") {
			return nil, fmt.Errorf("remote listing contains an unsafe path")
		}
		entry := foldersync.Entry{Path: name, Size: file.Size, ModTime: file.ModTime}
		if deep {
			temporary, createErr := os.CreateTemp("", ".lumonas-sync-preview-*")
			if createErr != nil {
				return nil, createErr
			}
			temporaryPath := temporary.Name()
			if closeErr := temporary.Close(); closeErr != nil {
				_ = os.Remove(temporaryPath)
				return nil, closeErr
			}
			object := path.Join(prefix, name)
			if downloadErr := download(ctx, remote, credentials, object, temporaryPath); downloadErr != nil {
				_ = os.Remove(temporaryPath)
				return nil, fmt.Errorf("deep check %s: %w", name, downloadErr)
			}
			digest, size, hashErr := backup.SHA256File(temporaryPath)
			_ = os.Remove(temporaryPath)
			if hashErr != nil {
				return nil, hashErr
			}
			if size != file.Size {
				return nil, fmt.Errorf("remote file size changed while previewing %s", name)
			}
			entry.Hash = digest
		}
		result[name] = entry
	}
	return result, nil
}

func (s *apiServer) startFolderSync(w http.ResponseWriter, r *http.Request, id string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	task, err := s.store.FolderSyncTask(id)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "sync task not found"})
		return
	}
	var input struct {
		PlanHash      string `json:"planHash"`
		ConfirmMirror bool   `json:"confirmMirror"`
		ConfirmTwoWay bool   `json:"confirmTwoWay"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
		return
	}
	plan, hash, err := s.folderSyncPlan(r.Context(), task)
	if err != nil {
		writeJSON(w, 422, map[string]string{"error": err.Error()})
		return
	}
	if input.PlanHash != "" && input.PlanHash != hash {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "sync endpoints changed after preview; create a fresh preview"})
		return
	}
	if task.Mode == "mirror" && (!input.ConfirmMirror || input.PlanHash == "" || input.PlanHash != hash) {
		writeJSON(w, 409, map[string]string{"error": "mirror run requires confirmation of the current preview"})
		return
	}
	if task.Direction == "two-way" && (!input.ConfirmTwoWay || input.PlanHash == "" || input.PlanHash != hash) {
		writeJSON(w, 409, map[string]string{"error": "two-way run requires confirmation of the current preview"})
		return
	}
	s.folderSyncMu.Lock()
	if s.folderSyncRunning == nil {
		s.folderSyncRunning = map[string]context.CancelFunc{}
	}
	if s.folderSyncRunning[id] != nil {
		s.folderSyncMu.Unlock()
		writeJSON(w, 409, map[string]string{"error": "a run for this task is already active"})
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.folderSyncRunning[id] = cancel
	s.folderSyncMu.Unlock()
	now := time.Now().UTC()
	run := foldersync.Run{ID: newID("sync-run"), TaskID: id, State: "running", Trigger: "manual", StartedAt: now, Plan: plan, Files: plan.Files, Bytes: plan.Bytes, Deleted: plan.Deletes, Conflicts: plan.Conflicts}
	if err := s.store.SaveFolderSyncRun(run); err != nil {
		cancel()
		s.folderSyncMu.Lock()
		delete(s.folderSyncRunning, id)
		s.folderSyncMu.Unlock()
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	job := model.Job{ID: run.ID, Actor: actor, Type: "folder-sync.run", Title: "Folder sync: " + task.Name, ResourceID: id, State: "running", Progress: folderSyncFloatPointer(0), CreatedAt: now, StartedAt: &now}
	_ = s.store.SaveJob(job)
	src := ""
	if task.Source.Kind != "destination" {
		src, _ = s.resolveSyncEndpoint(task.Source)
	}
	dst := ""
	if task.Destination.Kind != "destination" {
		dst, _ = s.resolveSyncEndpoint(task.Destination)
	}
	go func() {
		err := s.applyFolderSync(ctx, task, src, dst, plan, func(done int, bytes int64) {
			progress := 0.0
			if plan.Files > 0 {
				progress = float64(done) / float64(plan.Files) * 100
			}
			job.Progress = &progress
			job.Stage = fmt.Sprintf("Copied %d files (%d bytes)", done, bytes)
			_ = s.store.SaveJob(job)
			s.publish("job.progress", "info", &model.ResourceRef{Type: "job", ID: run.ID}, map[string]any{"job": job})
		})
		if err == nil && task.Direction == "two-way" {
			err = s.commitFolderSyncBaseline(task, src, dst)
		}
		finished := time.Now().UTC()
		run.FinishedAt = &finished
		if err != nil {
			if errors.Is(err, context.Canceled) {
				run.State = "cancelled"
				job.State = "cancelled"
			} else {
				run.State = "failed"
				run.Error = err.Error()
				job.State = "failed"
				job.Error = err.Error()
			}
		} else {
			run.State = "successful"
			job.State = "completed"
		}
		job.FinishedAt = &finished
		if run.State == "successful" {
			job.Progress = folderSyncFloatPointer(100)
		}
		_ = s.store.UpdateFolderSyncRun(run)
		_ = s.store.SaveJob(job)
		s.folderSyncMu.Lock()
		delete(s.folderSyncRunning, id)
		s.folderSyncMu.Unlock()
		s.publish("folder_sync.run.finished", mapSeverity(err), &model.ResourceRef{Type: "folder-sync-run", ID: run.ID}, map[string]any{"run": run})
	}()
	s.recordRequestAudit(r, actor, "folder_sync.run.start", id, map[string]any{"runId": run.ID, "mirror": task.Mode == "mirror", "deletions": plan.Deletes})
	writeJSON(w, 202, run)
}

func (s *apiServer) cancelFolderSync(w http.ResponseWriter, r *http.Request, id string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	s.folderSyncMu.Lock()
	cancel := s.folderSyncRunning[id]
	s.folderSyncMu.Unlock()
	if cancel == nil {
		writeJSON(w, 409, map[string]string{"error": "no active run for this task"})
		return
	}
	cancel()
	s.recordRequestAudit(r, actor, "folder_sync.run.cancel", id, nil)
	writeJSON(w, 202, map[string]string{"status": "cancellation_requested"})
}

func (s *apiServer) runDueFolderSyncTasks() {
	tasks, err := s.store.FolderSyncTasks()
	if err != nil {
		return
	}
	now := time.Now()
	if s.clock != nil {
		now = s.clock()
	}
	usbMounts := s.connectedUSBManagedMounts()
	attached := map[string]bool{}
	if s.folderSyncUSBState == nil {
		// Establish a baseline at startup. A disk already connected during boot
		// does not count as a fresh attach event.
		s.folderSyncUSBState = usbMounts
	} else {
		for mountPath, mounted := range usbMounts {
			if mounted && !s.folderSyncUSBState[mountPath] {
				attached[mountPath] = true
			}
		}
		s.folderSyncUSBState = usbMounts
	}
	for _, task := range tasks {
		if !task.Enabled {
			continue
		}
		dueBySchedule := (task.ScheduleKind == "daily" || task.ScheduleKind == "weekly") && task.TimeOfDay == now.Format("15:04")
		if dueBySchedule && task.ScheduleKind == "weekly" && !strings.EqualFold(task.Weekday, now.Weekday().String()) {
			dueBySchedule = false
		}
		trigger := ""
		if dueBySchedule {
			trigger = "schedule"
		} else if task.OnUSBAttach && task.Destination.Kind == "mount" && attached[task.Destination.MountPath] {
			trigger = "usb.attach"
		}
		if trigger == "" {
			continue
		}
		if task.LastRunAt != nil && task.LastRunAt.In(now.Location()).Format("2006-01-02 15:04") == now.Format("2006-01-02 15:04") {
			continue
		}
		s.folderSyncMu.Lock()
		active := s.folderSyncRunning[task.ID] != nil
		s.folderSyncMu.Unlock()
		if active {
			continue
		}
		planCtx, planCancel := context.WithTimeout(context.Background(), 2*time.Minute)
		plan, _, planErr := s.folderSyncPlan(planCtx, task)
		planCancel()
		if planErr != nil {
			continue
		}
		src, srcErr := "", error(nil)
		if task.Source.Kind != "destination" {
			src, srcErr = s.resolveSyncEndpoint(task.Source)
		}
		dst, dstErr := "", error(nil)
		if task.Destination.Kind != "destination" {
			dst, dstErr = s.resolveSyncEndpoint(task.Destination)
		}
		if srcErr != nil || dstErr != nil {
			continue
		}
		ctx, cancel := context.WithCancel(context.Background())
		s.folderSyncMu.Lock()
		if s.folderSyncRunning == nil {
			s.folderSyncRunning = map[string]context.CancelFunc{}
		}
		if s.folderSyncRunning[task.ID] != nil {
			s.folderSyncMu.Unlock()
			cancel()
			continue
		}
		s.folderSyncRunning[task.ID] = cancel
		s.folderSyncMu.Unlock()
		started := now.UTC()
		run := foldersync.Run{ID: newID("sync-run"), TaskID: task.ID, State: "running", Trigger: trigger, StartedAt: started, Plan: plan, Files: plan.Files, Bytes: plan.Bytes, Deleted: plan.Deletes, Conflicts: plan.Conflicts}
		if s.store.SaveFolderSyncRun(run) != nil {
			s.folderSyncMu.Lock()
			delete(s.folderSyncRunning, task.ID)
			s.folderSyncMu.Unlock()
			cancel()
			continue
		}
		task.LastRunAt = &started
		_ = s.store.SaveFolderSyncTask(task)
		job := model.Job{ID: run.ID, Type: "folder-sync.run", Title: "Scheduled folder sync: " + task.Name, ResourceID: task.ID, State: "running", Progress: folderSyncFloatPointer(0), CreatedAt: started, StartedAt: &started}
		_ = s.store.SaveJob(job)
		go func(task foldersync.Task, run foldersync.Run, job model.Job, ctx context.Context, src, dst string) {
			err := s.applyFolderSync(ctx, task, src, dst, run.Plan, func(done int, bytes int64) {
				progress := 0.0
				if run.Plan.Files > 0 {
					progress = float64(done) / float64(run.Plan.Files) * 100
				}
				job.Progress = &progress
				job.Stage = fmt.Sprintf("Copied %d files (%d bytes)", done, bytes)
				_ = s.store.SaveJob(job)
				s.publish("job.progress", "info", &model.ResourceRef{Type: "job", ID: run.ID}, map[string]any{"job": job})
			})
			if err == nil && task.Direction == "two-way" {
				err = s.commitFolderSyncBaseline(task, src, dst)
			}
			finished := time.Now().UTC()
			run.FinishedAt = &finished
			if err != nil {
				run.State = "failed"
				run.Error = err.Error()
				if errors.Is(err, context.Canceled) {
					run.State = "cancelled"
					run.Error = ""
					job.State = "cancelled"
				} else {
					job.State = "failed"
					job.Error = err.Error()
				}
			} else {
				run.State = "successful"
				job.State = "completed"
			}
			job.FinishedAt = &finished
			if run.State == "successful" {
				job.Progress = folderSyncFloatPointer(100)
			}
			_ = s.store.UpdateFolderSyncRun(run)
			_ = s.store.SaveJob(job)
			s.folderSyncMu.Lock()
			delete(s.folderSyncRunning, task.ID)
			s.folderSyncMu.Unlock()
			s.publish("folder_sync.run.finished", mapSeverity(err), &model.ResourceRef{Type: "folder-sync-run", ID: run.ID}, map[string]any{"run": run})
		}(task, run, job, ctx, src, dst)
	}
}

func (s *apiServer) connectedUSBManagedMounts() map[string]bool {
	connected := map[string]bool{}
	if s.diskFunc == nil {
		return connected
	}
	disks, err := s.diskFunc()
	if err != nil {
		return connected
	}
	usb := map[string]bool{}
	for _, disk := range disks {
		if strings.EqualFold(disk.Interface, "usb") && disk.Mounted {
			usb[disk.ID] = true
		}
	}
	entries, err := s.store.MountEntries()
	if err != nil {
		return connected
	}
	for _, entry := range entries {
		if entry.Enabled && entry.Kind == "disk" && usb[entry.TargetID] && managedMountIsActive(entry.MountPath) {
			connected[entry.MountPath] = true
		}
	}
	return connected
}

func managedMountIsActive(target string) bool {
	target = filepath.Clean(target)
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		mountPath := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`).Replace(fields[4])
		if filepath.Clean(mountPath) == target {
			return true
		}
	}
	return false
}

func (s *apiServer) resolveSyncEndpoint(endpoint foldersync.Endpoint) (string, error) {
	var root string
	switch endpoint.Kind {
	case "share":
		share, err := s.store.ManagedShare(endpoint.ShareID)
		if err != nil {
			return "", errors.New("managed share not found")
		}
		root = share.Path
	case "mount":
		entries, err := s.store.MountEntries()
		if err != nil {
			return "", err
		}
		allowed := false
		for _, entry := range entries {
			if entry.Enabled && entry.MountPath == endpoint.MountPath {
				allowed = true
				break
			}
		}
		if !allowed {
			return "", errors.New("mount is not configured and enabled")
		}
		if !managedMountIsActive(endpoint.MountPath) {
			return "", errors.New("managed mount is not currently mounted; refusing to use the underlying system directory")
		}
		root = endpoint.MountPath
	default:
		return "", errors.New("remote endpoint is not supported by this transfer runtime")
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", errors.New("managed endpoint is unavailable")
	}
	if endpoint.Path != "" {
		candidate := filepath.Join(resolved, filepath.FromSlash(endpoint.Path))
		clean, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			return "", err
		}
		rel, err := filepath.Rel(resolved, clean)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", errors.New("endpoint path escapes managed root")
		}
		resolved = clean
	}
	return resolved, nil
}

func mapSeverity(err error) string {
	if err != nil {
		return "warning"
	}
	return "info"
}

func folderSyncFloatPointer(value float64) *float64 { return &value }

func (s *apiServer) syncDestination(id string) (backup.Destination, backup.Credentials, error) {
	key := s.recoveryKeyString()
	if key == "" {
		return backup.Destination{}, backup.Credentials{}, errors.New("configure the recovery key before using remote backup destinations")
	}
	destination, credentials, err := s.store.BackupDestination(id, []byte(key))
	if err != nil {
		return backup.Destination{}, backup.Credentials{}, errors.New("configured backup destination or credentials are unavailable")
	}
	if destination.Type != backup.DestinationSFTP && destination.Type != backup.DestinationS3 {
		return backup.Destination{}, backup.Credentials{}, errors.New("folder sync destinations must use SFTP or S3")
	}
	if !destination.Enabled {
		return backup.Destination{}, backup.Credentials{}, errors.New("backup destination is disabled")
	}
	return destination, credentials, nil
}

func (s *apiServer) applyFolderSync(ctx context.Context, task foldersync.Task, sourceRoot, destinationRoot string, plan foldersync.Plan, progress func(int, int64)) error {
	if task.Direction == "two-way" {
		return foldersync.ApplyTwoWay(ctx, sourceRoot, destinationRoot, plan, progress)
	}
	if task.Source.Kind == "destination" {
		return s.applyRemotePull(ctx, task, destinationRoot, plan, progress)
	}
	if task.Destination.Kind != "destination" {
		return foldersync.Apply(ctx, sourceRoot, destinationRoot, plan, progress)
	}
	destination, credentials, err := s.syncDestination(task.Destination.DestinationID)
	if err != nil {
		return err
	}
	completed := 0
	var transferred int64
	for _, change := range plan.Changes {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		object := path.Join(task.Destination.Prefix, filepath.ToSlash(change.Path))
		if change.Action == "delete" {
			if task.Mode != "mirror" || !task.MirrorApproved {
				return errors.New("remote deletion is not approved")
			}
			if err := backup.Delete(ctx, destination, credentials, object); err != nil {
				return fmt.Errorf("delete remote file %s: %w", change.Path, err)
			}
			continue
		}
		source := filepath.Join(sourceRoot, filepath.FromSlash(change.Path))
		info, err := os.Lstat(source)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("sync source %s is not a regular file", change.Path)
		}
		digest, size, err := backup.SHA256File(source)
		if err != nil {
			return err
		}
		if err := backup.UploadAndVerifyContext(ctx, destination, credentials, source, object, digest, size); err != nil {
			return fmt.Errorf("transfer %s: %w", change.Path, err)
		}
		completed++
		transferred += size
		if progress != nil {
			progress(completed, transferred)
		}
	}
	return nil
}

func (s *apiServer) commitFolderSyncBaseline(task foldersync.Task, leftRoot, rightRoot string) error {
	left, err := foldersync.ScanFiltered(leftRoot, task.DeepCheck, task.IgnorePatterns)
	if err != nil {
		return err
	}
	right, err := foldersync.ScanFiltered(rightRoot, task.DeepCheck, task.IgnorePatterns)
	if err != nil {
		return err
	}
	if len(left) != len(right) {
		return errors.New("sync endpoints changed during transfer; baseline was not advanced")
	}
	for name, entry := range left {
		other, ok := right[name]
		if !ok || entry.Size != other.Size || !entry.ModTime.Equal(other.ModTime) || (task.DeepCheck && !strings.EqualFold(entry.Hash, other.Hash)) {
			return fmt.Errorf("sync endpoints diverged at %s; baseline was not advanced", name)
		}
	}
	return s.store.SaveFolderSyncBaseline(task.ID, left)
}

func (s *apiServer) applyRemotePull(ctx context.Context, task foldersync.Task, destinationRoot string, plan foldersync.Plan, progress func(int, int64)) error {
	destination, credentials, err := s.syncDestination(task.Source.DestinationID)
	if err != nil {
		return err
	}
	return foldersync.ApplyPull(ctx, destinationRoot, plan, func(ctx context.Context, relativePath, target string) error {
		object := path.Join(task.Source.Prefix, filepath.ToSlash(relativePath))
		return backup.Download(ctx, destination, credentials, object, target)
	}, progress)
}
