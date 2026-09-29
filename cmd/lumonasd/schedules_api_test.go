package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/monitoring"
	"github.com/lumonas/lumonas/internal/privileged"
	"github.com/lumonas/lumonas/internal/store"
)

func TestSchedulesEndpointsRoundTrip(t *testing.T) {
	server := testServer(t)
	list := httptest.NewRecorder()
	server.routes().ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/v1/schedules", nil))
	if list.Code != http.StatusOK {
		t.Fatalf("expected schedules, got %d: %s", list.Code, list.Body.String())
	}
	var schedules []struct {
		ID       string `json:"id"`
		JobType  string `json:"jobType"`
		Kind     string `json:"kind"`
		Schedule string `json:"schedule"`
		Next     string `json:"next"`
		Enabled  bool   `json:"enabled"`
	}
	if err := json.NewDecoder(list.Body).Decode(&schedules); err != nil || len(schedules) != 6 {
		t.Fatalf("unexpected schedules: %#v err=%v", schedules, err)
	}
	disable := httptest.NewRecorder()
	server.routes().ServeHTTP(disable, httptest.NewRequest(http.MethodPatch, "/api/v1/schedules/sched-sync", strings.NewReader(`{"enabled":false}`)))
	if disable.Code != http.StatusOK || strings.Contains(disable.Body.String(), `"enabled":true`) {
		t.Fatalf("unexpected disable response %d: %s", disable.Code, disable.Body.String())
	}
	recolor := httptest.NewRecorder()
	server.routes().ServeHTTP(recolor, httptest.NewRequest(http.MethodPatch, "/api/v1/schedules/sched-sync", strings.NewReader(`{"timeOfDay":"04:15"}`)))
	if recolor.Code != http.StatusOK || !strings.Contains(recolor.Body.String(), "Daily at 04:15") {
		t.Fatalf("unexpected recolor response %d: %s", recolor.Code, recolor.Body.String())
	}
	var updated struct {
		NextDueAt *string `json:"nextDueAt"`
	}
	if err := json.NewDecoder(recolor.Body).Decode(&updated); err != nil || updated.NextDueAt == nil {
		t.Fatalf("expected re-armed next due time: %#v err=%v", updated, err)
	}
	event := httptest.NewRecorder()
	server.routes().ServeHTTP(event, httptest.NewRequest(http.MethodPatch, "/api/v1/schedules/sched-config", strings.NewReader(`{"enabled":false}`)))
	if event.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected event schedule edit to be rejected, got %d", event.Code)
	}
	missing := httptest.NewRecorder()
	server.routes().ServeHTTP(missing, httptest.NewRequest(http.MethodPatch, "/api/v1/schedules/sched-none", strings.NewReader(`{"enabled":false}`)))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("expected missing schedule 404, got %d", missing.Code)
	}
	reread := httptest.NewRecorder()
	server.routes().ServeHTTP(reread, httptest.NewRequest(http.MethodGet, "/api/v1/schedules", nil))
	if reread.Code != http.StatusOK || !strings.Contains(reread.Body.String(), `"enabled":false`) {
		t.Fatalf("expected persisted disable, got %d: %s", reread.Code, reread.Body.String())
	}
	snapshot := httptest.NewRecorder()
	server.routes().ServeHTTP(snapshot, httptest.NewRequest(http.MethodPatch, "/api/v1/schedules/sched-snapshot", strings.NewReader(`{"snapshotKind":"btrfs","snapshotSource":"/srv/pools/media","snapshotLabel":"nightly","snapshotKeep":14,"snapshotLockDays":90}`)))
	if snapshot.Code != http.StatusOK || !strings.Contains(snapshot.Body.String(), `"snapshotKeep":14`) || !strings.Contains(snapshot.Body.String(), `"snapshotLockDays":90`) || !strings.Contains(snapshot.Body.String(), `"snapshotSource":"/srv/pools/media"`) {
		t.Fatalf("expected snapshot policy update, got %d: %s", snapshot.Code, snapshot.Body.String())
	}
	invalidSnapshot := httptest.NewRecorder()
	server.routes().ServeHTTP(invalidSnapshot, httptest.NewRequest(http.MethodPatch, "/api/v1/schedules/sched-snapshot", strings.NewReader(`{"snapshotSource":"relative"}`)))
	if invalidSnapshot.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected invalid snapshot source rejection, got %d: %s", invalidSnapshot.Code, invalidSnapshot.Body.String())
	}
}

func TestSnapshotPoliciesCanBeCreatedAndDeleted(t *testing.T) {
	server := testServer(t)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/schedules", strings.NewReader(`{"name":"Documents snapshots","kind":"weekly","weekday":"wednesday","timeOfDay":"02:45","snapshotKind":"btrfs","snapshotSource":"/srv/pools/documents","snapshotKeep":30,"snapshotLockDays":90,"enabled":true}`))
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, request)
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), `"snapshotKeep":30`) || !strings.Contains(response.Body.String(), `"snapshotLockDays":90`) || !strings.Contains(response.Body.String(), `"snapshotSource":"/srv/pools/documents"`) {
		t.Fatalf("unexpected snapshot policy create: %d %s", response.Code, response.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil || !strings.HasPrefix(created.ID, "schedule-") {
		t.Fatalf("unexpected generated schedule id %#v: %v", created, err)
	}
	list := httptest.NewRecorder()
	server.routes().ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/v1/schedules", nil))
	if !strings.Contains(list.Body.String(), created.ID) {
		t.Fatalf("created policy missing from list: %s", list.Body.String())
	}
	remove := httptest.NewRecorder()
	server.routes().ServeHTTP(remove, httptest.NewRequest(http.MethodDelete, "/api/v1/schedules/"+created.ID, nil))
	if remove.Code != http.StatusNoContent {
		t.Fatalf("expected policy delete, got %d %s", remove.Code, remove.Body.String())
	}
	invalid := httptest.NewRecorder()
	server.routes().ServeHTTP(invalid, httptest.NewRequest(http.MethodPost, "/api/v1/schedules", strings.NewReader(`{"name":"Unsafe","kind":"daily","timeOfDay":"02:00","snapshotKind":"btrfs","snapshotSource":"/etc","snapshotKeep":3}`)))
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected managed path validation, got %d %s", invalid.Code, invalid.Body.String())
	}
	invalidLock := httptest.NewRecorder()
	server.routes().ServeHTTP(invalidLock, httptest.NewRequest(http.MethodPost, "/api/v1/schedules", strings.NewReader(`{"name":"Unsafe lock","kind":"daily","timeOfDay":"02:00","snapshotKind":"btrfs","snapshotSource":"/srv/pools/media","snapshotKeep":3,"snapshotLockDays":3651}`)))
	if invalidLock.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected excessive snapshot lock to be rejected, got %d %s", invalidLock.Code, invalidLock.Body.String())
	}
}

func TestFilesystemScrubSchedulesAreValidatedAndPersisted(t *testing.T) {
	server := testServer(t)
	create := httptest.NewRecorder()
	server.routes().ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v1/schedules", strings.NewReader(`{"jobType":"filesystem.scrub","name":"Weekly pool check","kind":"weekly","weekday":"sunday","timeOfDay":"03:00","filesystemKind":"btrfs","filesystemSource":"/srv/pools/media","enabled":true}`)))
	if create.Code != http.StatusCreated || !strings.Contains(create.Body.String(), `"jobType":"filesystem.scrub"`) || !strings.Contains(create.Body.String(), `"filesystemSource":"/srv/pools/media"`) {
		t.Fatalf("filesystem scrub schedule was not created: %d %s", create.Code, create.Body.String())
	}
	invalid := httptest.NewRecorder()
	server.routes().ServeHTTP(invalid, httptest.NewRequest(http.MethodPost, "/api/v1/schedules", strings.NewReader(`{"jobType":"filesystem.scrub","name":"Unsafe check","kind":"daily","timeOfDay":"03:00","filesystemKind":"btrfs","filesystemSource":"/etc","enabled":true}`)))
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unmanaged filesystem scrub target was accepted: %d %s", invalid.Code, invalid.Body.String())
	}
	var created monitoring.Schedule
	if err := json.NewDecoder(create.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	updated := httptest.NewRecorder()
	server.routes().ServeHTTP(updated, httptest.NewRequest(http.MethodPatch, "/api/v1/schedules/"+created.ID, strings.NewReader(`{"filesystemKind":"zfs","filesystemSource":"tank/media"}`)))
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), `"filesystemKind":"zfs"`) {
		t.Fatalf("filesystem scrub schedule update failed: %d %s", updated.Code, updated.Body.String())
	}
}

func TestRunDueSchedulesFiresBackupSchedule(t *testing.T) {
	server := testServer(t)
	if err := server.store.SetMeta("nas_uuid", "nas-test"); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Minute)
	schedule, err := server.store.JobSchedule("sched-backup", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	schedule.NextDueAt = &past
	if err := server.store.SaveJobSchedule(schedule); err != nil {
		t.Fatal(err)
	}
	server.runDueSchedules()
	fired, err := server.store.JobSchedule("sched-backup", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if fired.NextDueAt == nil || !fired.NextDueAt.After(time.Now()) {
		t.Fatalf("expected next due in the future, got %v", fired.NextDueAt)
	}
	if fired.LastStartedAt == nil {
		t.Fatal("expected last started to be recorded")
	}
}

func TestRunDueSchedulesFiresSnapshotScheduleAndPersistsOrigin(t *testing.T) {
	server := testServer(t)
	requests := make(chan privileged.Request, 4)
	server.brokerExec = func(_ context.Context, request privileged.Request) error {
		requests <- request
		return nil
	}
	schedule, err := server.store.JobSchedule("sched-snapshot", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	schedule.Enabled = true
	schedule.SnapshotLockDays = 30
	past := time.Now().Add(-time.Minute)
	schedule.NextDueAt = &past
	if err := server.store.SaveJobSchedule(schedule); err != nil {
		t.Fatal(err)
	}
	server.runDueSchedules()
	fired, err := server.store.JobSchedule("sched-snapshot", time.Now())
	if err != nil || fired.LastStartedAt == nil {
		t.Fatalf("snapshot schedule did not fire: %#v err=%v", fired, err)
	}
	var request privileged.Request
	select {
	case request = <-requests:
	case <-time.After(2 * time.Second):
		t.Fatal("scheduled snapshot did not reach the privileged broker")
	}
	if request.Operation != "snapshot.create" || request.OperationID == "" {
		t.Fatalf("unexpected scheduled snapshot broker request: %#v", request)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		records, readErr := server.store.StorageSnapshots(schedule.SnapshotSource, 10)
		if readErr == nil && len(records) == 1 && records[0].Origin == "scheduled" && records[0].ProtectedUntil != nil && records[0].ProtectedUntil.After(time.Now()) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	records, err := server.store.StorageSnapshots(schedule.SnapshotSource, 10)
	t.Fatalf("scheduled snapshot was not persisted with origin: %#v err=%v", records, err)
}

func TestScheduledSnapshotRetentionSkipsLockedSnapshots(t *testing.T) {
	server := testServer(t)
	lockedUntil := time.Now().UTC().Add(24 * time.Hour)
	locked, err := server.store.SaveStorageSnapshot(store.StorageSnapshotRecord{Kind: "btrfs", Source: "/srv/pools/media", Name: "locked-old", Origin: "scheduled", CreatedAt: time.Now().Add(-48 * time.Hour), ProtectedUntil: &lockedUntil})
	if err != nil {
		t.Fatal(err)
	}
	newest, err := server.store.SaveStorageSnapshot(store.StorageSnapshotRecord{Kind: "btrfs", Source: "/srv/pools/media", Name: "newest", Origin: "scheduled", CreatedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	called := false
	server.brokerExec = func(context.Context, privileged.Request) error { called = true; return nil }
	if err := server.pruneScheduledSnapshots(monitoring.Schedule{ID: "schedule-lock", JobType: "snapshot.create", SnapshotKind: "btrfs", SnapshotSource: "/srv/pools/media", SnapshotKeep: 1}, newest); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("retention cleanup attempted to delete a locked snapshot")
	}
	if _, found, err := server.store.StorageSnapshot(locked.ID); err != nil || !found {
		t.Fatalf("locked snapshot did not survive cleanup: found=%v err=%v", found, err)
	}
}

func TestRunDueSchedulesSkipsWhenJobActive(t *testing.T) {
	server := testServer(t)
	if err := server.store.SetMeta("nas_uuid", "nas-test"); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Minute)
	schedule, err := server.store.JobSchedule("sched-sync", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	schedule.NextDueAt = &past
	if err := server.store.SaveJobSchedule(schedule); err != nil {
		t.Fatal(err)
	}
	active := model.Job{ID: "job-active", Type: "snapraid.sync", Title: "snapraid sync", State: "running", CreatedAt: time.Now().UTC()}
	if err := server.store.SaveJob(active); err != nil {
		t.Fatal(err)
	}
	server.runDueSchedules()
	rearmed, err := server.store.JobSchedule("sched-sync", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if rearmed.NextDueAt == nil || !rearmed.NextDueAt.After(time.Now()) {
		t.Fatalf("expected schedule re-armed to the future, got %v", rearmed.NextDueAt)
	}
	if rearmed.LastStartedAt != nil {
		t.Fatal("expected skipped schedule to not record a start")
	}
}

func TestScheduledPowerRunsOnlyOncePerMinute(t *testing.T) {
	server := testServer(t)
	server.runtimeStateFunc = func() map[string]any { return nil }
	fixed := time.Date(2026, time.January, 5, 3, 15, 0, 0, time.Local)
	server.clock = func() time.Time { return fixed }
	var requests []privileged.Request
	server.brokerExec = func(_ context.Context, request privileged.Request) error {
		requests = append(requests, request)
		return nil
	}
	settings := `{"updates":{},"runtime":{},"power":{"maintenanceMode":false,"wol":[],"ups":[],"schedule":{"enabled":true,"action":"shutdown","time":"03:15","days":"Daily"}},"security":{}}`
	if err := server.store.SetMeta(settingsMetaKey, settings); err != nil {
		t.Fatal(err)
	}
	server.runScheduledPower()
	server.runScheduledPower()
	if len(requests) != 1 || requests[0].Operation != "power.shutdown" || requests[0].OperationID == "" {
		t.Fatalf("unexpected scheduled power requests: %#v", requests)
	}
}
