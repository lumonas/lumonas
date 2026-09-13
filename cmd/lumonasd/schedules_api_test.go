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
	"github.com/lumonas/lumonas/internal/privileged"
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
	server.routes().ServeHTTP(snapshot, httptest.NewRequest(http.MethodPatch, "/api/v1/schedules/sched-snapshot", strings.NewReader(`{"snapshotKind":"btrfs","snapshotSource":"/srv/pools/media","snapshotLabel":"nightly","snapshotKeep":14}`)))
	if snapshot.Code != http.StatusOK || !strings.Contains(snapshot.Body.String(), `"snapshotKeep":14`) || !strings.Contains(snapshot.Body.String(), `"snapshotSource":"/srv/pools/media"`) {
		t.Fatalf("expected snapshot policy update, got %d: %s", snapshot.Code, snapshot.Body.String())
	}
	invalidSnapshot := httptest.NewRecorder()
	server.routes().ServeHTTP(invalidSnapshot, httptest.NewRequest(http.MethodPatch, "/api/v1/schedules/sched-snapshot", strings.NewReader(`{"snapshotSource":"relative"}`)))
	if invalidSnapshot.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected invalid snapshot source rejection, got %d: %s", invalidSnapshot.Code, invalidSnapshot.Body.String())
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
		if readErr == nil && len(records) == 1 && records[0].Origin == "scheduled" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	records, err := server.store.StorageSnapshots(schedule.SnapshotSource, 10)
	t.Fatalf("scheduled snapshot was not persisted with origin: %#v err=%v", records, err)
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
