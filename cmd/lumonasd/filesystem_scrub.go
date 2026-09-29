package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/privileged"
	"github.com/lumonas/lumonas/internal/storage"
)

func (s *apiServer) runFilesystemScrubJob(job model.Job, kind storage.SnapshotKind, source string) {
	ctx, cancel := context.WithCancel(context.Background())
	s.runningJobMu.Lock()
	if s.runningJobCancels == nil {
		s.runningJobCancels = make(map[string]context.CancelFunc)
	}
	s.runningJobCancels[job.ID] = cancel
	s.runningJobMu.Unlock()
	defer func() {
		cancel()
		s.runningJobMu.Lock()
		delete(s.runningJobCancels, job.ID)
		s.runningJobMu.Unlock()
	}()

	now := time.Now().UTC()
	progress := 1.0
	started, err := s.store.StartQueuedJob(job.ID, "Starting filesystem scrub", progress, now)
	if err != nil || !started {
		return
	}
	job.State, job.Stage, job.StartedAt, job.Progress = "running", "Starting filesystem scrub", &now, &progress
	s.publish("job.state_changed", "info", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job})
	requested := map[string]any{"filesystemKind": string(kind), "filesystemSource": source}
	if _, err := s.executeFilesystemScrubOperation(ctx, "filesystem.scrub.start", job, requested); err != nil {
		s.finishFilesystemScrubJob(job, "failed", "Could not start filesystem scrub", 0, err)
		return
	}
	job.Stage = "Checking filesystem integrity"
	_ = s.store.SaveJob(job)
	s.publish("job.state_changed", "info", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job})

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		result, statusErr := s.executeFilesystemScrubOperation(ctx, "filesystem.scrub.status", job, requested)
		if statusErr != nil {
			if ctx.Err() != nil {
				s.cancelFilesystemScrub(job, kind, source)
				s.finishFilesystemScrubJob(job, "cancelled", "Filesystem scrub cancelled", progress, nil)
				return
			}
			s.finishFilesystemScrubJob(job, "failed", "Could not read filesystem scrub status", progress, statusErr)
			return
		}
		status, parseErr := decodeFilesystemScrubStatus(result.Data)
		if parseErr != nil {
			s.finishFilesystemScrubJob(job, "failed", "Filesystem scrub status was invalid", progress, parseErr)
			return
		}
		progress = status.Progress
		if progress < 0 {
			progress = 0
		}
		if progress > 100 {
			progress = 100
		}
		job.Progress = &progress
		job.Stage = status.Message
		_ = s.store.SaveJob(job)
		s.publish("job.state_changed", "info", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job, "scrub": status})
		if !status.Running {
			if status.Cancelled {
				s.finishFilesystemScrubJob(job, "cancelled", status.Message, 100, nil)
				return
			}
			if status.Errors {
				s.finishFilesystemScrubJob(job, "failed", status.Message, 100, errors.New("filesystem scrub reported data integrity errors"))
				return
			}
			s.finishFilesystemScrubJob(job, "successful", status.Message, 100, nil)
			return
		}
		select {
		case <-ctx.Done():
			s.cancelFilesystemScrub(job, kind, source)
			s.finishFilesystemScrubJob(job, "cancelled", "Filesystem scrub cancelled", progress, nil)
			return
		case <-ticker.C:
		}
	}
}

func decodeFilesystemScrubStatus(value any) (storage.ScrubStatus, error) {
	switch status := value.(type) {
	case storage.ScrubStatus:
		return status, nil
	case map[string]any:
		result := storage.ScrubStatus{}
		var ok bool
		if result.Running, ok = status["running"].(bool); !ok {
			return result, errors.New("running state is missing")
		}
		if number, ok := status["progress"].(float64); ok {
			result.Progress = number
		}
		if message, ok := status["message"].(string); ok {
			result.Message = message
		}
		return result, nil
	default:
		return storage.ScrubStatus{}, fmt.Errorf("unexpected scrub status %T", value)
	}
}

func (s *apiServer) executeFilesystemScrubOperation(ctx context.Context, operation string, job model.Job, requested map[string]any) (privileged.Response, error) {
	operationID := newID("scrub-op")
	request := privileged.Request{
		Operation: operation, OperationID: operationID, CorrelationID: job.CorrelationID,
		PlanHash: operationID, RequestedState: requested,
		ExpiresAt: time.Now().UTC().Add(2 * time.Minute), Confirmed: true,
	}
	return s.executePrivileged(ctx, request)
}

func (s *apiServer) cancelFilesystemScrub(job model.Job, kind storage.SnapshotKind, source string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _ = s.executeFilesystemScrubOperation(ctx, "filesystem.scrub.cancel", job, map[string]any{"filesystemKind": string(kind), "filesystemSource": source})
}

func (s *apiServer) finishFilesystemScrubJob(job model.Job, state, stage string, value float64, runErr error) {
	finished := time.Now().UTC()
	job.State, job.Stage, job.FinishedAt = state, stage, &finished
	job.Progress = &value
	if runErr != nil {
		job.Error = runErr.Error()
	}
	_ = s.store.SaveJob(job)
	severity := "info"
	if state == "failed" {
		severity = "critical"
	} else if state == "cancelled" {
		severity = "attention"
	}
	s.publish("job.state_changed", severity, &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job})
	event := "filesystem.scrub." + state
	s.publish(event, severity, &model.ResourceRef{Type: "filesystem", ID: job.ResourceID}, map[string]any{"jobId": job.ID, "error": job.Error, "stage": stage})
}
