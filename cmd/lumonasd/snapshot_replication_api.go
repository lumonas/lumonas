package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/lumonas/lumonas/internal/backup"
	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/privileged"
	"github.com/lumonas/lumonas/internal/storage"
	"github.com/lumonas/lumonas/internal/store"
)

const snapshotReplicationMaxStreamBytes = int64(1 << 40)

var snapshotReplicationIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
var snapshotReplicationHTTPClient = &http.Client{Timeout: 12 * time.Hour, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
var snapshotReplicationReceiveSlots = make(chan struct{}, 2)

type snapshotReplicationActiveOps struct {
	ExportID  string
	ReceiveID string
}

func (s *apiServer) listSnapshotReplicationTasks(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	tasks, err := s.store.SnapshotReplicationTasks(r.URL.Query().Get("peerId"))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.snapshotReplicationMu.Lock()
	for index := range tasks {
		tasks[index].Running = s.snapshotReplicationRunning[tasks[index].ID] != nil
	}
	s.snapshotReplicationMu.Unlock()
	writeJSON(w, http.StatusOK, tasks)
}

func (s *apiServer) createSnapshotReplicationTask(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		PeerID             string `json:"peerId"`
		Name               string `json:"name"`
		SourceShareID      string `json:"sourceShareId"`
		DestinationShareID string `json:"destinationShareId"`
		ReceiveToken       string `json:"receiveToken"`
		ScheduleKind       string `json:"scheduleKind"`
		TimeOfDay          string `json:"timeOfDay"`
		Weekday            string `json:"weekday"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	if decoder.Decode(&input) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	input.PeerID = strings.TrimSpace(input.PeerID)
	input.SourceShareID = strings.TrimSpace(input.SourceShareID)
	input.DestinationShareID = strings.TrimSpace(input.DestinationShareID)
	input.ReceiveToken = strings.TrimSpace(input.ReceiveToken)
	if input.Name == "" || len(input.Name) > 120 || input.PeerID == "" || input.SourceShareID == "" || input.DestinationShareID == "" || !snapshotReplicationIDPattern.MatchString(input.DestinationShareID) || input.ReceiveToken == "" || len(input.ReceiveToken) > 4096 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "name, peer, source share, destination share, and scoped receive token are required"})
		return
	}
	if input.ScheduleKind == "" {
		input.ScheduleKind = "manual"
	}
	if input.ScheduleKind != "manual" && input.ScheduleKind != "daily" && input.ScheduleKind != "weekly" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "schedule must be manual, daily, or weekly"})
		return
	}
	if input.TimeOfDay == "" {
		input.TimeOfDay = "02:00"
	}
	if _, err := time.Parse("15:04", input.TimeOfDay); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "timeOfDay must use HH:MM"})
		return
	}
	if input.Weekday == "" {
		input.Weekday = "sunday"
	}
	if input.ScheduleKind == "weekly" && !validWeekday(input.Weekday) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "weekday must be a full weekday name"})
		return
	}
	if input.ScheduleKind != "weekly" {
		input.Weekday = "sunday"
	}
	if _, _, err := s.store.ReplicationPeerCredentials(input.PeerID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "replication peer not found"})
		return
	}
	source, err := s.store.ManagedShare(input.SourceShareID)
	if err != nil || !source.Enabled {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "source must be an enabled managed share"})
		return
	}
	if _, err = s.store.ManagedShare(input.DestinationShareID); err == nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "destination share must be on the remote peer"})
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "destination share could not be checked"})
		return
	}
	if !strings.HasPrefix(source.Path, "/srv/pools/") && !strings.HasPrefix(source.Path, "/srv/disks/") && !strings.HasPrefix(source.Path, "/srv/lumonas/") {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "source share must be inside managed storage"})
		return
	}
	credentials, err := backup.EncryptCredentials(backup.Credentials{Password: input.ReceiveToken}, []byte(s.recoveryKeyString()))
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "recovery key is required to protect the replication token"})
		return
	}
	task := store.SnapshotReplicationTask{ID: newID("srt"), PeerID: input.PeerID, Name: input.Name, SourceShareID: input.SourceShareID, DestinationShareID: input.DestinationShareID, ScheduleKind: input.ScheduleKind, TimeOfDay: input.TimeOfDay, Weekday: strings.ToLower(input.Weekday)}
	if err := s.store.SaveSnapshotReplicationTask(task, credentials); err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			status = http.StatusConflict
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "replication.snapshot-task.create", task.ID, map[string]any{"peerId": task.PeerID, "sourceShareId": task.SourceShareID, "destinationShareId": task.DestinationShareID, "schedule": task.ScheduleKind})
	writeJSON(w, http.StatusCreated, task)
}

func (s *apiServer) updateSnapshotReplicationTask(w http.ResponseWriter, r *http.Request, id string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	s.snapshotReplicationMu.Lock()
	running := s.snapshotReplicationRunning[id] != nil
	s.snapshotReplicationMu.Unlock()
	if running {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "stop the active run before editing this task"})
		return
	}
	task, ciphertext, err := s.store.SnapshotReplicationTask(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "snapshot replication task not found"})
		return
	}
	var input struct {
		Name         string `json:"name"`
		ScheduleKind string `json:"scheduleKind"`
		TimeOfDay    string `json:"timeOfDay"`
		Weekday      string `json:"weekday"`
		ReceiveToken string `json:"receiveToken"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
	if decoder.Decode(&input) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if strings.TrimSpace(input.Name) != "" {
		task.Name = strings.TrimSpace(input.Name)
	}
	if input.ScheduleKind != "" {
		task.ScheduleKind = input.ScheduleKind
	}
	if input.TimeOfDay != "" {
		task.TimeOfDay = input.TimeOfDay
	}
	if input.Weekday != "" {
		task.Weekday = strings.ToLower(input.Weekday)
	}
	if input.Name != "" && (strings.TrimSpace(input.Name) == "" || len(strings.TrimSpace(input.Name)) > 120) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "task name must contain 1 to 120 characters"})
		return
	}
	if task.ScheduleKind != "manual" && task.ScheduleKind != "daily" && task.ScheduleKind != "weekly" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "schedule must be manual, daily, or weekly"})
		return
	}
	if _, err := time.Parse("15:04", task.TimeOfDay); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "timeOfDay must use HH:MM"})
		return
	}
	if task.ScheduleKind == "weekly" && !validWeekday(task.Weekday) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "weekday must be a full weekday name"})
		return
	}
	if input.ReceiveToken != "" {
		if len(input.ReceiveToken) > 4096 {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "receive token is too long"})
			return
		}
		ciphertext, err = backup.EncryptCredentials(backup.Credentials{Password: strings.TrimSpace(input.ReceiveToken)}, []byte(s.recoveryKeyString()))
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "recovery key is required to protect the replication token"})
			return
		}
	}
	if task.ScheduleKind != "weekly" {
		task.Weekday = "sunday"
	}
	if err := s.store.SaveSnapshotReplicationTask(task, ciphertext); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "snapshot replication task could not be updated"})
		return
	}
	updated, _, err := s.store.SnapshotReplicationTask(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "updated task could not be loaded"})
		return
	}
	s.recordRequestAudit(r, actor, "replication.snapshot-task.update", id, map[string]any{"schedule": updated.ScheduleKind})
	writeJSON(w, http.StatusOK, updated)
}

func (s *apiServer) runSnapshotReplicationTask(w http.ResponseWriter, r *http.Request, id string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	task, _, err := s.store.SnapshotReplicationTask(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "snapshot replication task not found"})
		return
	}
	ctx, ok := s.beginSnapshotReplication(id)
	if !ok {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "this snapshot replication task is already running"})
		return
	}
	run, job, err := s.startSnapshotReplicationRun(task, actor)
	if err != nil {
		s.endSnapshotReplication(id)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not record snapshot replication run"})
		return
	}
	go s.executeSnapshotReplicationRun(ctx, task, run, job)
	s.recordRequestAudit(r, actor, "replication.snapshot-task.run", task.ID, map[string]any{"runId": run.ID, "jobId": job.ID})
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "run": run, "jobId": job.ID})
}

func (s *apiServer) runSnapshotReplication(ctx context.Context, task store.SnapshotReplicationTask, run *store.SnapshotReplicationRun, job *model.Job) (int64, error) {
	if err := s.store.RecordSnapshotReplicationAttempt(task.ID, time.Now().UTC()); err != nil {
		return 0, fmt.Errorf("could not record replication start: %w", err)
	}
	s.updateSnapshotReplicationRun(run, job, "running", "creating source snapshot", 5, 0, "", "")
	peer, _, err := s.store.ReplicationPeerCredentials(task.PeerID)
	if err != nil {
		return 0, fmt.Errorf("replication peer is unavailable: %w", err)
	}
	_, taskCiphertext, err := s.store.SnapshotReplicationTask(task.ID)
	if err != nil {
		return 0, fmt.Errorf("replication task credentials are unavailable: %w", err)
	}
	key := s.recoveryKeyString()
	if key == "" {
		return 0, errors.New("recovery key is unavailable")
	}
	taskCredentials, err := backup.DecryptCredentials(taskCiphertext, []byte(key))
	if err != nil || taskCredentials.Password == "" {
		return 0, errors.New("replication receive credential cannot be decrypted")
	}
	share, err := s.store.ManagedShare(task.SourceShareID)
	if err != nil || !share.Enabled {
		return 0, errors.New("source managed share is unavailable")
	}
	operationID := newID("snap")
	createRequest := privileged.Request{Operation: "snapshot.create", OperationID: operationID, PlanHash: operationID, RequestedState: map[string]any{"kind": "btrfs", "source": share.Path, "label": "replica"}, ExpiresAt: time.Now().UTC().Add(5 * time.Minute), Confirmed: true}
	createCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	created, err := s.executePrivileged(createCtx, createRequest)
	if err != nil {
		return 0, fmt.Errorf("could not create source snapshot: %w", err)
	}
	if !created.OK {
		return 0, fmt.Errorf("could not create source snapshot: %s", created.Error)
	}
	record := snapshotRecordFromResponse(created, storage.SnapshotBtrfs, share.Path, "replica")
	record.Origin = "replication"
	if _, err := s.store.SaveStorageSnapshot(record); err != nil {
		return 0, fmt.Errorf("source snapshot was created but could not be recorded: %w", err)
	}
	run.SnapshotName = record.Name
	s.updateSnapshotReplicationRun(run, job, "running", "exporting snapshot", 25, 0, record.Name, "")
	streamID := newID("stream")
	exportID := newID("export")
	previous, _, taskErr := s.store.SnapshotReplicationTask(task.ID)
	if taskErr != nil {
		return 0, taskErr
	}
	requested := map[string]any{"kind": "btrfs", "source": share.Path, "name": record.Name, "parent": previous.LastSnapshotName}
	exportCtx, exportCancel := context.WithTimeout(ctx, 12*time.Hour)
	defer exportCancel()
	s.setSnapshotReplicationOperation(task.ID, "export", exportID)
	exported, err := s.executePrivileged(exportCtx, privileged.Request{Operation: "snapshot.export", OperationID: exportID, PlanHash: exportID, RequestedState: requested, ExpiresAt: time.Now().UTC().Add(12 * time.Hour), Confirmed: true})
	s.setSnapshotReplicationOperation(task.ID, "export", "")
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			s.cancelPrivilegedSnapshotOperation(exportID, "snapshot.export.cancel")
		}
		return 0, fmt.Errorf("snapshot export failed: %w", err)
	}
	if !exported.OK {
		return 0, fmt.Errorf("snapshot export failed: %s", exported.Error)
	}
	data, ok := exported.Data.(map[string]any)
	if !ok {
		return 0, errors.New("snapshot exporter returned an invalid result")
	}
	bytes, ok := numericInt64(data["bytes"])
	if !ok || bytes <= 0 || bytes > snapshotReplicationMaxStreamBytes {
		return 0, errors.New("snapshot exporter returned an invalid stream size")
	}
	digest, _ := data["sha256"].(string)
	if len(digest) != 64 {
		return 0, errors.New("snapshot exporter returned an invalid checksum")
	}
	defer func() {
		_, _ = s.executePrivileged(context.Background(), privileged.Request{Operation: "snapshot.stream.remove", OperationID: exportID, PlanHash: exportID, ExpiresAt: time.Now().UTC().Add(time.Minute), Confirmed: true})
	}()
	stageRoot := envOr("LUMONAS_REPLICATION_DIR", "/var/lib/lumonas/replication")
	streamPath := filepath.Join(stageRoot, "out-"+exportID+".stream")
	stream, err := os.Open(streamPath)
	if err != nil {
		return 0, fmt.Errorf("could not open exported snapshot stream: %w", err)
	}
	defer stream.Close()
	info, err := stream.Stat()
	if err != nil || info.Size() != bytes {
		return 0, errors.New("snapshot stream size changed after export")
	}
	peerURL, err := validateReplicationURL(peer.URL)
	if err != nil {
		return 0, errors.New("replication peer URL is invalid")
	}
	endpoint := peerURL + "/api/v1/replication/snapshots/" + url.PathEscape(task.DestinationShareID) + "/receive"
	request, err := http.NewRequestWithContext(exportCtx, http.MethodPut, endpoint, io.LimitReader(stream, snapshotReplicationMaxStreamBytes+1))
	if err != nil {
		return 0, err
	}
	request.ContentLength = bytes
	request.Header.Set("Authorization", "Bearer "+taskCredentials.Password)
	request.Header.Set("Content-Type", "application/octet-stream")
	request.Header.Set("X-LumoNAS-Snapshot-Name", record.Name)
	request.Header.Set("X-LumoNAS-Snapshot-Parent", previous.LastSnapshotName)
	request.Header.Set("X-LumoNAS-Snapshot-SHA256", digest)
	request.Header.Set("X-LumoNAS-Snapshot-Stream", streamID)
	receiveOperationID := newID("receive")
	s.setSnapshotReplicationOperation(task.ID, "receive", receiveOperationID)
	request.Header.Set("X-LumoNAS-Snapshot-Operation", receiveOperationID)
	lastProgress := 0.0
	request.Body = &replicationProgressReader{reader: io.NopCloser(io.LimitReader(stream, snapshotReplicationMaxStreamBytes+1)), onProgress: func(sent int64) {
		progress := 25 + float64(sent)/float64(bytes)*70
		if progress-lastProgress >= 5 || sent == bytes {
			lastProgress = progress
			s.updateSnapshotReplicationRun(run, job, "running", "transferring snapshot", progress, sent, record.Name, "")
		}
	}}
	s.updateSnapshotReplicationRun(run, job, "running", "connecting to destination", 30, 0, record.Name, "")
	response, err := snapshotReplicationHTTPClient.Do(request)
	s.setSnapshotReplicationOperation(task.ID, "receive", "")
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(exportCtx.Err(), context.Canceled) {
			s.cancelRemoteSnapshotReceive(task, taskCredentials.Password, peer.URL, receiveOperationID)
		}
		return 0, fmt.Errorf("remote snapshot receive failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		payload, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return 0, fmt.Errorf("remote snapshot receive returned %s: %s", response.Status, strings.TrimSpace(string(payload)))
	}
	if err := s.store.RecordSnapshotReplicationResult(task.ID, record.Name, ""); err != nil {
		return bytes, fmt.Errorf("snapshot was received but task state could not be saved: %w", err)
	}
	return bytes, nil
}

func (s *apiServer) beginSnapshotReplication(id string) (context.Context, bool) {
	s.snapshotReplicationMu.Lock()
	defer s.snapshotReplicationMu.Unlock()
	if s.snapshotReplicationRunning == nil {
		s.snapshotReplicationRunning = make(map[string]context.CancelFunc)
	}
	if s.snapshotReplicationRunning[id] != nil {
		return nil, false
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.snapshotReplicationRunning[id] = cancel
	return ctx, true
}
func (s *apiServer) endSnapshotReplication(id string) {
	s.snapshotReplicationMu.Lock()
	if cancel := s.snapshotReplicationRunning[id]; cancel != nil {
		cancel()
	}
	delete(s.snapshotReplicationRunning, id)
	s.snapshotReplicationMu.Unlock()
}

func (s *apiServer) setSnapshotReplicationOperation(taskID, kind, operationID string) {
	s.snapshotReplicationMu.Lock()
	defer s.snapshotReplicationMu.Unlock()
	if s.snapshotReplicationOps == nil {
		s.snapshotReplicationOps = make(map[string]snapshotReplicationActiveOps)
	}
	active := s.snapshotReplicationOps[taskID]
	if kind == "export" {
		active.ExportID = operationID
	} else {
		active.ReceiveID = operationID
	}
	s.snapshotReplicationOps[taskID] = active
}

func (s *apiServer) cancelPrivilegedSnapshotOperation(operationID, operation string) {
	if !snapshotReplicationIDPattern.MatchString(operationID) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = s.executePrivileged(ctx, privileged.Request{Operation: operation, OperationID: operationID, PlanHash: operationID, ExpiresAt: time.Now().UTC().Add(5 * time.Second), Confirmed: true})
}

func (s *apiServer) cancelRemoteSnapshotReceive(task store.SnapshotReplicationTask, token, peerURL, operationID string) {
	if !snapshotReplicationIDPattern.MatchString(operationID) {
		return
	}
	base, err := validateReplicationURL(peerURL)
	if err != nil {
		return
	}
	endpoint := base + "/api/v1/replication/snapshots/" + url.PathEscape(task.DestinationShareID) + "/cancel"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("X-LumoNAS-Snapshot-Operation", operationID)
	response, err := snapshotReplicationHTTPClient.Do(request)
	if err == nil {
		response.Body.Close()
	}
}

func (s *apiServer) startSnapshotReplicationRun(task store.SnapshotReplicationTask, actor string) (store.SnapshotReplicationRun, model.Job, error) {
	now := time.Now().UTC()
	job := model.Job{ID: newID("job"), Type: "replication.snapshot", Title: "Replicate snapshot: " + task.Name, ResourceID: task.ID, Actor: actor, State: "running", Progress: replicationFloat(0), Stage: "queued", CreatedAt: now, StartedAt: &now}
	run := store.SnapshotReplicationRun{ID: newID("run"), TaskID: task.ID, JobID: job.ID, State: "running", Stage: "queued", Progress: 0, CreatedAt: now, StartedAt: &now}
	if err := s.store.SaveJob(job); err != nil {
		return run, job, err
	}
	if err := s.store.CreateSnapshotReplicationRun(run); err != nil {
		job.State = "failed"
		job.Error = "Could not create replication run history"
		finished := time.Now().UTC()
		job.FinishedAt = &finished
		_ = s.store.SaveJob(job)
		return run, job, err
	}
	s.publish("job.state_changed", "info", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job, "runId": run.ID})
	return run, job, nil
}

func (s *apiServer) updateSnapshotReplicationRun(run *store.SnapshotReplicationRun, job *model.Job, state, stage string, progress float64, bytes int64, snapshotName, errorText string) {
	run.State, run.Stage, run.Progress, run.Bytes, run.SnapshotName, run.Error = state, stage, progress, bytes, snapshotName, errorText
	_ = s.store.UpdateSnapshotReplicationRun(*run)
	job.State, job.Stage, job.Progress, job.Error = state, stage, replicationFloat(progress), errorText
	if state != "running" {
		finished := time.Now().UTC()
		run.FinishedAt = &finished
		job.FinishedAt = &finished
		_ = s.store.UpdateSnapshotReplicationRun(*run)
	}
	_ = s.store.SaveJob(*job)
	severity := "info"
	if state == "failed" {
		severity = "critical"
	} else if state == "cancelled" {
		severity = "warning"
	}
	s.publish("job.state_changed", severity, &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job, "runId": run.ID, "bytes": bytes})
}

func (s *apiServer) executeSnapshotReplicationRun(ctx context.Context, task store.SnapshotReplicationTask, run store.SnapshotReplicationRun, job model.Job) {
	defer s.endSnapshotReplication(task.ID)
	runCtx, cancel := context.WithTimeout(ctx, 12*time.Hour)
	defer cancel()
	bytes, err := s.runSnapshotReplication(runCtx, task, &run, &job)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(runCtx.Err(), context.Canceled) {
			s.updateSnapshotReplicationRun(&run, &job, "cancelled", "cancelled", run.Progress, run.Bytes, run.SnapshotName, "Replication cancelled")
		} else {
			_ = s.store.RecordSnapshotReplicationResult(task.ID, "", err.Error())
			s.updateSnapshotReplicationRun(&run, &job, "failed", "failed", run.Progress, run.Bytes, run.SnapshotName, err.Error())
		}
		s.publish("replication.snapshot-task.failed", "critical", &model.ResourceRef{Type: "replication-task", ID: task.ID}, map[string]any{"runId": run.ID, "error": err.Error()})
		return
	}
	s.updateSnapshotReplicationRun(&run, &job, "successful", "completed", 100, bytes, run.SnapshotName, "")
	s.publish("replication.snapshot-task.completed", "info", &model.ResourceRef{Type: "replication-task", ID: task.ID}, map[string]any{"runId": run.ID, "snapshot": run.SnapshotName, "bytes": bytes})
}

func (s *apiServer) snapshotReplicationRunHistory(w http.ResponseWriter, r *http.Request, id string) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	if _, _, err := s.store.SnapshotReplicationTask(id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "snapshot replication task not found"})
		return
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 200 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be between 1 and 200"})
			return
		}
		limit = parsed
	}
	runs, err := s.store.SnapshotReplicationRuns(id, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (s *apiServer) cancelSnapshotReplicationTask(w http.ResponseWriter, r *http.Request, id string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	s.snapshotReplicationMu.Lock()
	cancel := s.snapshotReplicationRunning[id]
	active := s.snapshotReplicationOps[id]
	s.snapshotReplicationMu.Unlock()
	if cancel == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "no active run for this snapshot replication task"})
		return
	}
	cancel()
	if active.ExportID != "" {
		s.cancelPrivilegedSnapshotOperation(active.ExportID, "snapshot.export.cancel")
	}
	if active.ReceiveID != "" {
		task, encrypted, loadErr := s.store.SnapshotReplicationTask(id)
		if loadErr == nil && s.recoveryKeyString() != "" {
			credentials, decryptErr := backup.DecryptCredentials(encrypted, []byte(s.recoveryKeyString()))
			peer, _, peerErr := s.store.ReplicationPeerCredentials(task.PeerID)
			if decryptErr == nil && peerErr == nil {
				s.cancelRemoteSnapshotReceive(task, credentials.Password, peer.URL, active.ReceiveID)
			}
		}
	}
	s.recordRequestAudit(r, actor, "replication.snapshot-task.cancel", id, nil)
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "cancellation_requested"})
}

func replicationFloat(value float64) *float64 { return &value }

type replicationProgressReader struct {
	reader     io.ReadCloser
	sent       int64
	onProgress func(int64)
}

func (reader *replicationProgressReader) Close() error { return reader.reader.Close() }

func (reader *replicationProgressReader) Read(buffer []byte) (int, error) {
	count, err := reader.reader.Read(buffer)
	reader.sent += int64(count)
	if reader.onProgress != nil {
		reader.onProgress(reader.sent)
	}
	return count, err
}

func (s *apiServer) runDueSnapshotReplicationTasks() {
	now := time.Now()
	if s.clock != nil {
		now = s.clock()
	}
	tasks, err := s.store.SnapshotReplicationTasks("")
	if err != nil {
		return
	}
	for _, task := range tasks {
		if task.ScheduleKind != "daily" && task.ScheduleKind != "weekly" {
			continue
		}
		if task.TimeOfDay != now.Format("15:04") {
			continue
		}
		if task.ScheduleKind == "weekly" && !strings.EqualFold(task.Weekday, now.Weekday().String()) {
			continue
		}
		if task.LastAttemptAt != nil && task.LastAttemptAt.In(now.Location()).Format("2006-01-02 15:04") == now.Format("2006-01-02 15:04") {
			continue
		}
		ctx, started := s.beginSnapshotReplication(task.ID)
		if !started {
			continue
		}
		go func(task store.SnapshotReplicationTask) {
			run, job, runErr := s.startSnapshotReplicationRun(task, "system:schedule")
			if runErr != nil {
				s.endSnapshotReplication(task.ID)
				return
			}
			s.executeSnapshotReplicationRun(ctx, task, run, job)
		}(task)
	}
}

func (s *apiServer) deleteSnapshotReplicationTask(w http.ResponseWriter, r *http.Request, id string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	s.snapshotReplicationMu.Lock()
	running := s.snapshotReplicationRunning[id] != nil
	s.snapshotReplicationMu.Unlock()
	if running {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "a running snapshot replication task cannot be deleted"})
		return
	}
	if err := s.store.DeleteSnapshotReplicationTask(id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "snapshot replication task not found"})
		return
	}
	s.recordRequestAudit(r, actor, "replication.snapshot-task.delete", id, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *apiServer) receiveSnapshotReplication(w http.ResponseWriter, r *http.Request, shareID string) {
	if !snapshotReplicationIDPattern.MatchString(shareID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "destination share ID is invalid"})
		return
	}
	if _, ok := s.identityActor(w, r, true); !ok {
		return
	}
	share, err := s.store.ManagedShare(shareID)
	if err != nil || !share.Enabled {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "destination managed share is unavailable"})
		return
	}
	name := r.Header.Get("X-LumoNAS-Snapshot-Name")
	parent := r.Header.Get("X-LumoNAS-Snapshot-Parent")
	digest := strings.ToLower(strings.TrimSpace(r.Header.Get("X-LumoNAS-Snapshot-SHA256")))
	streamID := strings.TrimSpace(r.Header.Get("X-LumoNAS-Snapshot-Stream"))
	opID := strings.TrimSpace(r.Header.Get("X-LumoNAS-Snapshot-Operation"))
	if storage.ValidateSnapshotName(name) != nil || (parent != "" && storage.ValidateSnapshotName(parent) != nil) || len(digest) != 64 || !snapshotReplicationIDPattern.MatchString(streamID) || !snapshotReplicationIDPattern.MatchString(opID) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "snapshot stream metadata is invalid"})
		return
	}
	for _, char := range digest {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')) {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "snapshot checksum is invalid"})
			return
		}
	}
	root := envOr("LUMONAS_REPLICATION_INCOMING_DIR", "/var/lib/lumonas/replication-incoming")
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "incoming snapshot storage is unsafe"})
		return
	}
	info, err := os.Lstat(root)
	resolved, resolveErr := filepath.EvalSymlinks(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 || resolveErr != nil || resolved != root {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "incoming snapshot storage is unsafe"})
		return
	}
	if r.ContentLength <= 0 || r.ContentLength > snapshotReplicationMaxStreamBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "snapshot stream must declare a positive size within the 1 TiB limit"})
		return
	}
	var filesystem syscall.Statfs_t
	if err := syscall.Statfs(root, &filesystem); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "incoming storage capacity could not be checked"})
		return
	}
	available := filesystem.Bavail * uint64(filesystem.Bsize)
	const incomingReserveBytes = uint64(512 << 20)
	if available <= incomingReserveBytes || uint64(r.ContentLength) > available-incomingReserveBytes {
		writeJSON(w, http.StatusInsufficientStorage, map[string]string{"error": "not enough free space to receive this snapshot while preserving a 512 MiB system reserve"})
		return
	}
	select {
	case snapshotReplicationReceiveSlots <- struct{}{}:
		defer func() { <-snapshotReplicationReceiveSlots }()
	default:
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "snapshot receiver is busy with other streams"})
		return
	}
	streamPath := filepath.Join(root, streamID+".stream")
	temporary, err := os.OpenFile(streamPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "snapshot stream ID is already in use"})
		return
	}
	defer os.Remove(streamPath)
	hash := sha256.New()
	limited := http.MaxBytesReader(w, r.Body, snapshotReplicationMaxStreamBytes)
	written, copyErr := io.Copy(io.MultiWriter(temporary, hash), limited)
	closeErr := temporary.Close()
	if copyErr != nil || closeErr != nil || written <= 0 || written != r.ContentLength || hex.EncodeToString(hash.Sum(nil)) != digest {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "snapshot stream size or checksum did not match"})
		return
	}
	s.snapshotReplicationMu.Lock()
	if s.snapshotReceiveOwners == nil {
		s.snapshotReceiveOwners = make(map[string]string)
	}
	s.snapshotReceiveOwners[opID] = shareID
	s.snapshotReplicationMu.Unlock()
	defer func() {
		s.snapshotReplicationMu.Lock()
		delete(s.snapshotReceiveOwners, opID)
		s.snapshotReplicationMu.Unlock()
	}()
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Hour)
	defer cancel()
	result, err := s.executePrivileged(ctx, privileged.Request{Operation: "snapshot.receive", OperationID: opID, PlanHash: opID, RequestedState: map[string]any{"kind": "btrfs", "target": share.Path, "name": name, "parent": parent, "streamId": streamID}, ExpiresAt: time.Now().UTC().Add(12 * time.Hour), Confirmed: true})
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "snapshot receiver unavailable"})
		return
	}
	if !result.OK {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": result.Error})
		return
	}
	_, _ = s.store.SaveStorageSnapshot(store.StorageSnapshotRecord{Kind: "btrfs", Source: share.Path, Name: name, Origin: "replication", CreatedAt: time.Now().UTC()})
	s.publish("storage.snapshot.received", "info", nil, map[string]any{"shareId": share.ID, "name": name, "bytes": written})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "snapshot": name, "bytes": written})
}

func (s *apiServer) cancelSnapshotReceive(w http.ResponseWriter, r *http.Request, shareID string) {
	if !snapshotReplicationIDPattern.MatchString(shareID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "destination share ID is invalid"})
		return
	}
	if _, ok := s.identityActor(w, r, true); !ok {
		return
	}
	opID := strings.TrimSpace(r.Header.Get("X-LumoNAS-Snapshot-Operation"))
	if !snapshotReplicationIDPattern.MatchString(opID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "snapshot operation ID is invalid"})
		return
	}
	s.snapshotReplicationMu.Lock()
	owner := s.snapshotReceiveOwners[opID]
	s.snapshotReplicationMu.Unlock()
	if owner != shareID {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "no active receive operation for this share"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	result, err := s.executePrivileged(ctx, privileged.Request{Operation: "snapshot.receive.cancel", OperationID: opID, PlanHash: opID, ExpiresAt: time.Now().UTC().Add(5 * time.Second), Confirmed: true})
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "snapshot receiver could not be cancelled"})
		return
	}
	if !result.OK {
		writeJSON(w, http.StatusConflict, map[string]string{"error": result.Error})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "cancellation_requested"})
}

func numericInt64(value any) (int64, bool) {
	switch number := value.(type) {
	case int64:
		return number, true
	case int:
		return int64(number), true
	case float64:
		return int64(number), number == float64(int64(number))
	default:
		return 0, false
	}
}
func validWeekday(value string) bool {
	switch strings.ToLower(value) {
	case "monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday":
		return true
	}
	return false
}
