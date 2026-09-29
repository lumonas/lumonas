package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/monitoring"
	"github.com/lumonas/lumonas/internal/privileged"
	"github.com/lumonas/lumonas/internal/shares"
)

func TestShareRelocationPreviewIsVerifiedAndDoesNotCreateDestination(t *testing.T) {
	server := testServer(t)
	source := t.TempDir()
	storageRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "important.txt"), []byte("important data"), 0o640); err != nil {
		t.Fatal(err)
	}
	share, err := server.store.CreateManagedShare(shares.ManagedShare{ID: "relocate-share", Name: "Documents", Path: source, Enabled: true, Protocols: []shares.Protocol{{Name: "smb"}}})
	if err != nil {
		t.Fatal(err)
	}
	server.shareStorageResourcesFunc = func(_ context.Context) ([]shareStorageResource, error) {
		return []shareStorageResource{{ID: storageRoot, Path: storageRoot, Label: "Pool", Kind: "pool"}}, nil
	}
	preview, err := server.buildShareRelocationPreview(context.Background(), share.ID, shareRelocationTarget{ResourceID: storageRoot, RelativePath: "family/Documents"})
	if err != nil {
		t.Fatal(err)
	}
	if preview.FileCount != 1 || preview.Bytes != int64(len("important data")) || !preview.RetainsSource || !preview.RequiresDowntime || preview.PlanHash == "" {
		t.Fatalf("unexpected relocation preview: %#v", preview)
	}
	if _, err := os.Stat(preview.DestinationPath); !os.IsNotExist(err) {
		t.Fatalf("preview created its destination: %v", err)
	}
	if _, err := os.Stat(filepath.Join(source, "important.txt")); err != nil {
		t.Fatalf("preview changed the original share: %v", err)
	}
}

func TestShareRelocationRejectsUnsafeOrOccupiedDestination(t *testing.T) {
	server := testServer(t)
	source := t.TempDir()
	storageRoot := t.TempDir()
	share, err := server.store.CreateManagedShare(shares.ManagedShare{ID: "relocate-share", Name: "Documents", Path: source, Enabled: true, Protocols: []shares.Protocol{{Name: "smb"}}})
	if err != nil {
		t.Fatal(err)
	}
	server.shareStorageResourcesFunc = func(_ context.Context) ([]shareStorageResource, error) {
		return []shareStorageResource{{ID: storageRoot, Path: storageRoot, Label: "Pool", Kind: "pool"}}, nil
	}
	for _, relative := range []string{"../outside", "/etc/passwd", "nested/../../escape"} {
		if _, err := server.buildShareRelocationPreview(context.Background(), share.ID, shareRelocationTarget{ResourceID: storageRoot, RelativePath: relative}); err == nil {
			t.Fatalf("unsafe destination %q was accepted", relative)
		}
	}
	occupied := filepath.Join(storageRoot, "occupied")
	if err := os.Mkdir(occupied, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(occupied, "keep.txt"), []byte("keep"), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := server.buildShareRelocationPreview(context.Background(), share.ID, shareRelocationTarget{ResourceID: storageRoot, RelativePath: "occupied"}); err == nil {
		t.Fatal("non-empty destination was accepted")
	}
}

func TestShareRelocationCopiesVerifiesSwitchesPathAndRetainsSource(t *testing.T) {
	server := testServer(t)
	source := t.TempDir()
	storageRoot := t.TempDir()
	t.Setenv("LUMONAS_SAMBA_CONFIG", filepath.Join(t.TempDir(), "generated", "smb.conf"))
	if err := os.WriteFile(filepath.Join(source, "notes.txt"), []byte("move me safely"), 0o640); err != nil {
		t.Fatal(err)
	}
	share, err := server.store.CreateManagedShare(shares.ManagedShare{ID: "relocate-share", Name: "Documents", Path: source, Enabled: true, Protocols: []shares.Protocol{{Name: "smb"}}})
	if err != nil {
		t.Fatal(err)
	}
	server.shareStorageResourcesFunc = func(_ context.Context) ([]shareStorageResource, error) {
		return []shareStorageResource{{ID: storageRoot, Path: storageRoot, Label: "Pool", Kind: "pool"}}, nil
	}
	server.brokerExecWithResponse = func(_ context.Context, request privileged.Request) (privileged.Response, error) {
		if request.Operation == "samba.status.read" {
			return privileged.Response{OK: true, Data: map[string]any{"sessions": map[string]any{}, "tcons": map[string]any{}, "open_files": map[string]any{}}}, nil
		}
		return privileged.Response{OK: true}, nil
	}
	preview, err := server.buildShareRelocationPreview(context.Background(), share.ID, shareRelocationTarget{ResourceID: storageRoot, RelativePath: "family/Documents"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	job := model.Job{ID: "relocation-job", Type: "share.relocate", ResourceID: share.ID, State: "queued", CreatedAt: now}
	if err := server.store.SaveJob(job); err != nil {
		t.Fatal(err)
	}
	server.runShareRelocation(context.Background(), func() {}, job, preview)
	updated, err := server.store.ManagedShare(share.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Path != preview.DestinationPath || !updated.Enabled {
		t.Fatalf("share was not switched back on at destination: %#v", updated)
	}
	if got, err := os.ReadFile(filepath.Join(preview.DestinationPath, "notes.txt")); err != nil || string(got) != "move me safely" {
		t.Fatalf("destination was not copied and verified: %q err=%v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(source, "notes.txt")); err != nil || string(got) != "move me safely" {
		t.Fatalf("original data was not retained: %q err=%v", got, err)
	}
	completed, err := server.store.Job(job.ID)
	if err != nil || completed.State != "successful" {
		encoded, _ := json.Marshal(completed)
		t.Fatalf("relocation job did not complete: %s err=%v", encoded, err)
	}
}

func TestShareRelocationCanBeScheduledAndRunsThroughScheduleDispatcher(t *testing.T) {
	server := testServer(t)
	source := t.TempDir()
	storageRoot := t.TempDir()
	t.Setenv("LUMONAS_SAMBA_CONFIG", filepath.Join(t.TempDir(), "generated", "smb.conf"))
	if err := os.WriteFile(filepath.Join(source, "schedule.txt"), []byte("scheduled move"), 0o640); err != nil {
		t.Fatal(err)
	}
	share, err := server.store.CreateManagedShare(shares.ManagedShare{ID: "scheduled-share", Name: "Scheduled", Path: source, Enabled: true, Protocols: []shares.Protocol{{Name: "smb"}}})
	if err != nil {
		t.Fatal(err)
	}
	server.shareStorageResourcesFunc = func(_ context.Context) ([]shareStorageResource, error) {
		return []shareStorageResource{{ID: storageRoot, Path: storageRoot, Label: "Pool", Kind: "pool"}}, nil
	}
	server.brokerExecWithResponse = func(_ context.Context, request privileged.Request) (privileged.Response, error) {
		if request.Operation == "samba.status.read" {
			return privileged.Response{OK: true, Data: map[string]any{"sessions": map[string]any{}, "tcons": map[string]any{}, "open_files": map[string]any{}}}, nil
		}
		return privileged.Response{OK: true}, nil
	}
	target := shareRelocationTarget{ResourceID: storageRoot, RelativePath: "Scheduled"}
	preview, err := server.buildShareRelocationPreview(context.Background(), share.ID, target)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"target": target, "planHash": preview.PlanHash, "confirmed": true, "scheduleKind": "daily", "timeOfDay": "02:30"})
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/shares/"+share.ID+"/relocation", strings.NewReader(string(body))))
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), `"jobType":"share.relocate"`) {
		t.Fatalf("scheduled relocation was not saved: %d %s", response.Code, response.Body.String())
	}
	schedules, err := server.store.JobSchedules(time.Now())
	if err != nil || len(schedules) == 0 {
		t.Fatalf("scheduled relocation missing: %#v err=%v", schedules, err)
	}
	var schedule monitoring.Schedule
	for _, item := range schedules {
		if item.JobType == "share.relocate" {
			schedule = item
			break
		}
	}
	if schedule.ID == "" || schedule.RelocationShareID != share.ID || schedule.RelocationResourceID != storageRoot || schedule.TimeOfDay != "02:30" {
		t.Fatalf("scheduled relocation fields were not persisted: %#v", schedule)
	}
	server.launchScheduleJob(schedule)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		updated, getErr := server.store.ManagedShare(share.ID)
		if getErr == nil && updated.Path == preview.DestinationPath {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	updated, _ := server.store.ManagedShare(share.ID)
	t.Fatalf("scheduled relocation dispatcher did not move share: %#v", updated)
}
