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
	"path/filepath"
	"strings"
	"time"

	"github.com/lumonas/lumonas/internal/foldersync"
	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/monitoring"
	"github.com/lumonas/lumonas/internal/privileged"
)

type shareRelocationTarget struct {
	ResourceID   string `json:"resourceId"`
	RelativePath string `json:"relativePath"`
}

type shareRelocationPreview struct {
	ShareID          string          `json:"shareId"`
	ShareName        string          `json:"shareName"`
	SourcePath       string          `json:"sourcePath"`
	DestinationRoot  string          `json:"destinationRoot"`
	RelativePath     string          `json:"relativePath"`
	DestinationPath  string          `json:"destinationPath"`
	FileCount        int             `json:"fileCount"`
	Bytes            int64           `json:"bytes"`
	Plan             foldersync.Plan `json:"plan"`
	PlanHash         string          `json:"planHash"`
	RetainsSource    bool            `json:"retainsSource"`
	RequiresDowntime bool            `json:"requiresDowntime"`
}

func (s *apiServer) previewShareRelocation(w http.ResponseWriter, r *http.Request, id string) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	var target shareRelocationTarget
	if json.NewDecoder(r.Body).Decode(&target) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	preview, err := s.buildShareRelocationPreview(r.Context(), id, target)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (s *apiServer) buildShareRelocationPreview(ctx context.Context, id string, target shareRelocationTarget) (shareRelocationPreview, error) {
	share, err := s.store.ManagedShare(id)
	if err != nil {
		return shareRelocationPreview{}, errors.New("managed share not found")
	}
	resources, err := s.shareStorageResources(ctx)
	if err != nil {
		return shareRelocationPreview{}, errors.New("storage discovery unavailable")
	}
	var resource *shareStorageResource
	for index := range resources {
		if resources[index].ID == filepath.Clean(target.ResourceID) {
			resource = &resources[index]
			break
		}
	}
	if resource == nil {
		return shareRelocationPreview{}, errors.New("destination must be a currently mounted managed storage location")
	}
	relative, err := cleanRelocationPath(target.RelativePath, share.Name)
	if err != nil {
		return shareRelocationPreview{}, err
	}
	destination := filepath.Join(resource.Path, relative)
	if err := relocationPathIsSafe(resource.Path, relative); err != nil {
		return shareRelocationPreview{}, err
	}
	source, err := filepath.EvalSymlinks(share.Path)
	if err != nil {
		return shareRelocationPreview{}, errors.New("current share location is unavailable")
	}
	if source == destination {
		return shareRelocationPreview{}, errors.New("destination is already the current share location")
	}
	_, rootErr := filepath.EvalSymlinks(resource.Path)
	if rootErr != nil {
		return shareRelocationPreview{}, errors.New("destination storage location is unavailable")
	}
	if relocationPathWithin(source, destination) || relocationPathWithin(destination, source) {
		return shareRelocationPreview{}, errors.New("source and destination must be separate, non-overlapping directories")
	}
	sourceEntries, err := foldersync.Scan(source, true)
	if err != nil {
		return shareRelocationPreview{}, fmt.Errorf("could not scan current share: %w", err)
	}
	destinationEntries := map[string]foldersync.Entry{}
	if info, statErr := os.Lstat(destination); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return shareRelocationPreview{}, errors.New("destination exists and is not a regular directory")
		}
		destinationEntries, err = foldersync.Scan(destination, true)
		if err != nil {
			return shareRelocationPreview{}, fmt.Errorf("could not scan destination: %w", err)
		}
		if len(destinationEntries) != 0 {
			return shareRelocationPreview{}, errors.New("destination must be empty so unrelated files cannot be overwritten")
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return shareRelocationPreview{}, statErr
	}
	plan := foldersync.BuildPlan(sourceEntries, destinationEntries, false, true)
	encoded, _ := json.Marshal(struct {
		ShareID     string
		Source      string
		Destination string
		Generation  int64
		Plan        foldersync.Plan
	}{id, source, destination, s.currentGeneration(), plan})
	digest := sha256.Sum256(encoded)
	return shareRelocationPreview{ShareID: id, ShareName: share.Name, SourcePath: source, DestinationRoot: resource.Path, RelativePath: relative, DestinationPath: destination, FileCount: plan.Files, Bytes: plan.Bytes, Plan: plan, PlanHash: hex.EncodeToString(digest[:]), RetainsSource: true, RequiresDowntime: share.Enabled}, nil
}

func cleanRelocationPath(value, defaultName string) (string, error) {
	value = strings.TrimSpace(value)
	if filepath.IsAbs(filepath.FromSlash(value)) {
		return "", errors.New("destination path must be relative to the selected storage location")
	}
	if strings.ContainsAny(value, "\\\x00\r\n") {
		return "", errors.New("destination path contains invalid characters")
	}
	if value == "" || value == "." {
		value = defaultName
	}
	clean := filepath.Clean(filepath.FromSlash(value))
	if clean == "" || clean == "." || clean == ".." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean != filepath.FromSlash(value) || strings.ContainsAny(clean, "\x00\r\n") {
		return "", errors.New("destination path must stay inside the selected storage location")
	}
	return clean, nil
}

func relocationPathIsSafe(root, relative string) error {
	root = filepath.Clean(root)
	parts := strings.Split(relative, string(filepath.Separator))
	current := root
	for _, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("destination path cannot contain symlinks")
		}
		if current != filepath.Join(root, relative) && !info.IsDir() {
			return errors.New("destination path contains a non-directory component")
		}
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return errors.New("selected storage location is unavailable")
	}
	parent := filepath.Dir(filepath.Join(root, relative))
	for {
		if _, statErr := os.Lstat(parent); statErr == nil {
			break
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return statErr
		}
		next := filepath.Dir(parent)
		if next == parent {
			return errors.New("destination parent is unavailable")
		}
		parent = next
	}
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return err
	}
	if !relocationPathWithin(resolvedRoot, resolvedParent) {
		return errors.New("destination path escapes the selected storage location")
	}
	return nil
}

func relocationPathWithin(root, candidate string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func (s *apiServer) startShareRelocation(w http.ResponseWriter, r *http.Request, id string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		Target       shareRelocationTarget `json:"target"`
		PlanHash     string                `json:"planHash"`
		Confirmed    bool                  `json:"confirmed"`
		ScheduleKind string                `json:"scheduleKind,omitempty"`
		TimeOfDay    string                `json:"timeOfDay,omitempty"`
		Weekday      string                `json:"weekday,omitempty"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if !input.Confirmed || input.PlanHash == "" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "review and confirm the current relocation preview"})
		return
	}
	preview, err := s.buildShareRelocationPreview(r.Context(), id, input.Target)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	if preview.PlanHash != input.PlanHash {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "share or destination changed; create a fresh preview"})
		return
	}
	if input.ScheduleKind != "" && input.ScheduleKind != "manual" {
		if input.TimeOfDay == "" {
			input.TimeOfDay = "02:00"
		}
		schedule := monitoring.Schedule{ID: newID("schedule"), Name: "Move " + preview.ShareName, JobType: "share.relocate", Kind: input.ScheduleKind, TimeOfDay: input.TimeOfDay, Weekday: strings.ToLower(input.Weekday), Enabled: true, RelocationShareID: id, RelocationResourceID: input.Target.ResourceID, RelocationRelativePath: preview.RelativePath}
		if err := validateScheduledShareRelocation(s, r.Context(), schedule); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
			return
		}
		next := monitoring.NextOccurrence(schedule, time.Now())
		schedule.NextDueAt = &next
		if err := s.store.SaveJobSchedule(schedule); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
			return
		}
		s.recordRequestAudit(r, actor, "share.relocation.schedule", id, map[string]any{"scheduleId": schedule.ID, "destination": preview.DestinationPath})
		s.advanceGeneration("share.relocation.schedule")
		saved, err := s.store.JobSchedule(schedule.ID, time.Now())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"schedule": saved, "preview": preview})
		return
	}
	job, err := s.queueShareRelocation(actor, requestCorrelationID(r), preview)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "share.relocation.start", id, map[string]any{"jobId": job.ID, "destination": preview.DestinationPath})
	writeJSON(w, http.StatusAccepted, map[string]any{"jobId": job.ID, "state": "queued", "preview": preview})
}

func validateScheduledShareRelocation(s *apiServer, ctx context.Context, schedule monitoring.Schedule) error {
	if schedule.JobType != "share.relocate" {
		return nil
	}
	if err := schedule.Validate(); err != nil {
		return err
	}
	if !schedule.Enabled {
		return nil
	}
	_, err := s.buildShareRelocationPreview(ctx, schedule.RelocationShareID, shareRelocationTarget{ResourceID: schedule.RelocationResourceID, RelativePath: schedule.RelocationRelativePath})
	return err
}

func (s *apiServer) queueShareRelocation(actor, correlationID string, preview shareRelocationPreview) (model.Job, error) {
	if err := s.assertNoSMBUsersForShare(context.Background(), preview.ShareName); err != nil {
		return model.Job{}, err
	}
	s.shareRelocationMu.Lock()
	if s.shareRelocations[preview.ShareID] != nil {
		s.shareRelocationMu.Unlock()
		return model.Job{}, errors.New("this share already has a relocation in progress")
	}
	if s.shareRelocations == nil {
		s.shareRelocations = map[string]context.CancelFunc{}
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.shareRelocations[preview.ShareID] = cancel
	s.shareRelocationMu.Unlock()
	now := time.Now().UTC()
	job := model.Job{ID: newID("job"), CorrelationID: correlationID, Actor: actor, Type: "share.relocate", Title: "Move share: " + preview.ShareName, ResourceID: preview.ShareID, State: "queued", Progress: folderSyncFloatPointer(0), CreatedAt: now}
	if err := s.admitJob(&job); err != nil {
		cancel()
		s.shareRelocationMu.Lock()
		delete(s.shareRelocations, preview.ShareID)
		s.shareRelocationMu.Unlock()
		return model.Job{}, err
	}
	s.runningJobMu.Lock()
	if s.runningJobCancels == nil {
		s.runningJobCancels = map[string]context.CancelFunc{}
	}
	s.runningJobCancels[job.ID] = cancel
	s.runningJobMu.Unlock()
	go s.runShareRelocation(ctx, cancel, job, preview)
	return job, nil
}

func (s *apiServer) assertNoSMBUsersForShare(ctx context.Context, shareName string) error {
	operationID := newID("smb-status")
	result, err := s.executePrivileged(ctx, privileged.Request{Operation: "samba.status.read", OperationID: operationID, PlanHash: operationID, ExpiresAt: time.Now().UTC().Add(10 * time.Second), Confirmed: true})
	if err != nil || !result.OK {
		return errors.New("could not confirm SMB clients are disconnected; retry when Samba status is available")
	}
	encoded, err := json.Marshal(result.Data)
	if err != nil {
		return errors.New("could not read SMB client status")
	}
	var raw struct {
		TCons map[string]struct {
			Service string `json:"service"`
		} `json:"tcons"`
		OpenFiles map[string]struct {
			ServicePath string `json:"service_path"`
		} `json:"open_files"`
	}
	if json.Unmarshal(encoded, &raw) != nil {
		return errors.New("could not decode SMB client status")
	}
	for _, tcon := range raw.TCons {
		if strings.EqualFold(tcon.Service, shareName) {
			return errors.New("disconnect SMB clients from this share before moving it")
		}
	}
	for _, file := range raw.OpenFiles {
		if strings.EqualFold(file.ServicePath, shareName) {
			return errors.New("close open SMB files on this share before moving it")
		}
	}
	return nil
}

func (s *apiServer) runShareRelocation(ctx context.Context, cancel context.CancelFunc, job model.Job, preview shareRelocationPreview) {
	defer func() {
		cancel()
		s.runningJobMu.Lock()
		delete(s.runningJobCancels, job.ID)
		s.runningJobMu.Unlock()
		s.shareRelocationMu.Lock()
		delete(s.shareRelocations, preview.ShareID)
		s.shareRelocationMu.Unlock()
	}()
	queued, err := s.store.Job(job.ID)
	if err != nil || queued.State != "queued" {
		return
	}
	freshPreview, previewErr := s.buildShareRelocationPreview(ctx, preview.ShareID, shareRelocationTarget{ResourceID: preview.DestinationRoot, RelativePath: preview.RelativePath})
	if previewErr == nil && freshPreview.PlanHash != preview.PlanHash {
		previewErr = errors.New("share or destination changed after confirmation; the original share remains in place")
	}
	if previewErr != nil {
		job.State, job.Stage, job.Error = "failed", "Relocation preview became stale", previewErr.Error()
		finished := time.Now().UTC()
		job.FinishedAt = &finished
		_ = s.store.SaveJob(job)
		return
	}
	preview = freshPreview
	started := time.Now().UTC()
	job.State, job.StartedAt, job.Stage = "running", &started, "Pausing share access"
	_ = s.store.SaveJob(job)
	s.publish("job.state_changed", "info", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job})

	share, err := s.store.ManagedShare(preview.ShareID)
	if err == nil {
		err = s.assertNoSMBUsersForShare(ctx, share.Name)
	}
	wasEnabled := share.Enabled
	paused := false
	if err == nil && wasEnabled {
		share.Enabled = false
		_, err = s.persistManagedShareUpdate(ctx, share)
		paused = err == nil
	}
	restoreSource := func() {
		if paused {
			share.Enabled = wasEnabled
			_, _ = s.persistManagedShareUpdate(context.Background(), share)
		}
	}
	if err == nil {
		_, err = ensureShareDirectory(preview.DestinationRoot, preview.RelativePath)
	}
	if err == nil {
		err = foldersync.Apply(ctx, preview.SourcePath, preview.DestinationPath, preview.Plan, func(done int, bytes int64) {
			progress := 0.0
			if preview.FileCount > 0 {
				progress = float64(done) * 90 / float64(preview.FileCount)
			}
			job.Progress, job.Stage = &progress, fmt.Sprintf("Copied %d of %d files (%d bytes)", done, preview.FileCount, bytes)
			_ = s.store.SaveJob(job)
			s.publish("job.progress", "info", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job})
		})
	}
	if err == nil {
		err = verifyRelocatedShare(preview.SourcePath, preview.DestinationPath)
	}
	if err == nil {
		share.Path = preview.DestinationPath
		share.Enabled = wasEnabled
		_, err = s.persistManagedShareUpdate(context.Background(), share)
		if err == nil {
			s.advanceGeneration("share.relocation")
			s.publishActor(job.Actor, "share.relocation.completed", "info", &model.ResourceRef{Type: "share", ID: share.ID}, map[string]any{"oldPath": preview.SourcePath, "newPath": preview.DestinationPath, "sourceRetained": true})
		}
	}
	if err != nil {
		restoreSource()
		if errors.Is(err, context.Canceled) {
			job.State, job.Stage = "cancelled", "Relocation cancelled; original share remains in place"
		} else {
			job.State, job.Stage, job.Error = "failed", "Relocation failed; original share remains in place", err.Error()
		}
	} else {
		job.State, job.Stage, job.Progress = "successful", "Share moved and destination verified; original data retained", folderSyncFloatPointer(100)
	}
	finished := time.Now().UTC()
	job.FinishedAt = &finished
	_ = s.store.SaveJob(job)
	s.publish("job.state_changed", mapSeverity(err), &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job, "sourceRetained": true})
}

func verifyRelocatedShare(sourcePath, destinationPath string) error {
	source, err := foldersync.Scan(sourcePath, true)
	if err != nil {
		return fmt.Errorf("could not verify source after copy: %w", err)
	}
	destination, err := foldersync.Scan(destinationPath, true)
	if err != nil {
		return fmt.Errorf("could not verify destination after copy: %w", err)
	}
	if len(source) != len(destination) {
		return errors.New("source changed during relocation; the original share remains in place")
	}
	for name, entry := range source {
		other, ok := destination[name]
		if !ok || entry.Size != other.Size || entry.Hash != other.Hash {
			return fmt.Errorf("destination verification failed for %s", name)
		}
	}
	return nil
}
