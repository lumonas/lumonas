package store

import (
	"fmt"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/backup"
	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/notify"
)

func TestPruneEventsKeepsNewestWindow(t *testing.T) {
	database, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	for index := 0; index < 110; index++ {
		if err := database.SaveEvent(model.Event{ID: "event-" + string(rune(index)), Type: "test", Timestamp: time.Now().UTC().Add(time.Duration(index) * time.Second), Severity: "info", Data: map[string]any{}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.PruneEvents(100); err != nil {
		t.Fatal(err)
	}
	items, err := database.Events(500)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 100 {
		t.Fatalf("expected 100 retained events, got %d", len(items))
	}
}

func TestEventsAfterReplaysInInsertionOrder(t *testing.T) {
	database, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	for _, event := range []model.Event{
		{ID: "event-a", Type: "a", Timestamp: time.Now().UTC(), Severity: "info", Data: map[string]any{}},
		{ID: "event-b", Type: "b", Timestamp: time.Now().UTC(), Severity: "info", Data: map[string]any{}},
		{ID: "event-c", Type: "c", Timestamp: time.Now().UTC(), Severity: "info", Data: map[string]any{}},
	} {
		if err := database.SaveEvent(event); err != nil {
			t.Fatal(err)
		}
	}
	items, err := database.EventsAfter("event-a", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != "event-b" || items[1].ID != "event-c" {
		t.Fatalf("unexpected replay window: %#v", items)
	}
	missing, err := database.EventsAfter("event-missing", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 0 {
		t.Fatalf("missing cursor unexpectedly replayed events: %#v", missing)
	}
}

func TestOperationalRetentionKeepsActiveAndNewestHistory(t *testing.T) {
	database, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()
	for index := 0; index < 105; index++ {
		attempted := now.Add(-time.Duration(index) * time.Minute)
		if err := database.SaveNotificationDelivery(notify.Delivery{ID: fmt.Sprintf("delivery-%03d", index), ChannelID: "channel", EventType: "test", State: "failed", AttemptedAt: attempted}); err != nil {
			t.Fatal(err)
		}
		finished := attempted
		started := attempted.Add(-time.Second)
		if err := database.SaveBackupRun(backup.Run{ID: fmt.Sprintf("run-%03d", index), Trigger: "test", State: "completed", StartedAt: started, FinishedAt: &finished}); err != nil {
			t.Fatal(err)
		}
		if _, err := database.db.Exec(`INSERT INTO storage_operations(operation_id,plan_hash,status,expires_at,plan_json,created_at) VALUES(?,?,?,?,?,?)`, fmt.Sprintf("operation-%03d", index), "hash", "planned", now.Add(-time.Hour).Format(timeFormat), "{}", attempted.Format(timeFormat)); err != nil {
			t.Fatal(err)
		}
		if err := database.RecordNetworkCheckpoint(fmt.Sprintf("checkpoint-%03d", index), "lan", "commit"); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.RecordNetworkCheckpoint("checkpoint-active", "lan", "pending"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.db.Exec(`INSERT INTO storage_operations(operation_id,plan_hash,status,expires_at,plan_json,created_at) VALUES(?,?,?,?,?,?)`, "operation-active", "hash", "planned", now.Add(time.Hour).Format(timeFormat), "{}", now.Add(-time.Hour).Format(timeFormat)); err != nil {
		t.Fatal(err)
	}

	if err := database.PruneNotificationDeliveries(100); err != nil {
		t.Fatal(err)
	}
	if err := database.PruneBackupRuns(100); err != nil {
		t.Fatal(err)
	}
	if err := database.PruneStorageOperations(now, 100); err != nil {
		t.Fatal(err)
	}
	if err := database.PruneNetworkCheckpoints(100); err != nil {
		t.Fatal(err)
	}

	assertCount(t, database, "notification_deliveries", 100)
	assertCount(t, database, "backup_runs", 100)
	assertCount(t, database, "storage_operations", 101)
	assertCount(t, database, "network_checkpoints", 101)
	assertExists(t, database, "storage_operations", "operation-active")
	assertExists(t, database, "network_checkpoints", "checkpoint-active")
}

func TestPruneOperationalHistoryRunsWithDefaultPolicy(t *testing.T) {
	database, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.PruneOperationalHistory(time.Now().UTC()); err != nil {
		t.Fatalf("default operational retention failed: %v", err)
	}
}

func TestPruneNotificationFailuresRemovesOrphans(t *testing.T) {
	database, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.SaveNotificationFailure("missing-channel", "disk.smart.warning", 3, time.Now().UTC().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := database.PruneNotificationFailures(); err != nil {
		t.Fatal(err)
	}
	if _, _, found, err := database.NotificationFailure("missing-channel", "disk.smart.warning"); err != nil || found {
		t.Fatalf("orphan notification failure remained: found=%v err=%v", found, err)
	}
}

func TestPruneOperationalHistoryBoundsCoreHistoryTables(t *testing.T) {
	database, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()
	transaction, err := database.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 10005; index++ {
		created := now.Add(-time.Duration(index) * time.Second).Format(timeFormat)
		if _, err := transaction.Exec(`INSERT INTO events(id,type,timestamp,severity,data_json) VALUES(?,?,?,?,?)`, fmt.Sprintf("event-retention-%05d", index), "retention", created, "info", "{}"); err != nil {
			transaction.Rollback()
			t.Fatal(err)
		}
		if _, err := transaction.Exec(`INSERT INTO audit_log(id,timestamp,actor,action,outcome,metadata_json) VALUES(?,?,?,?,?,?)`, fmt.Sprintf("audit-retention-%05d", index), created, "system", "retention", "recorded", "{}"); err != nil {
			transaction.Rollback()
			t.Fatal(err)
		}
		if _, err := transaction.Exec(`INSERT INTO jobs(id,type,title,state,created_at) VALUES(?,?,?,?,?)`, fmt.Sprintf("job-retention-%05d", index), "retention", "retention", "completed", created); err != nil {
			transaction.Rollback()
			t.Fatal(err)
		}
	}
	if err := transaction.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveCapacitySnapshot(model.CapacitySnapshot{ResourceID: "pool", CapturedAt: now.Add(-181 * 24 * time.Hour), TotalBytes: 100, UsedBytes: 50}); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveCapacitySnapshot(model.CapacitySnapshot{ResourceID: "pool", CapturedAt: now, TotalBytes: 100, UsedBytes: 60}); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveSystemMetricSample(model.SystemMetricSample{CapturedAt: now.Add(-8 * 24 * time.Hour), Metrics: model.SystemMetrics{RAMTotalBytes: 100}}); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveSystemMetricSample(model.SystemMetricSample{CapturedAt: now, Metrics: model.SystemMetrics{RAMTotalBytes: 100}}); err != nil {
		t.Fatal(err)
	}

	if err := database.PruneOperationalHistory(now); err != nil {
		t.Fatal(err)
	}
	for table, want := range map[string]int{"events": 10000, "audit_log": 10000, "jobs": 1000, "capacity_snapshots": 1, "system_metric_samples": 1} {
		var count int
		if err := database.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != want {
			t.Fatalf("unexpected %s count: got %d want %d", table, count, want)
		}
	}
}

func TestOperationalRetentionPrunesExpiredSessionsAndResolvedAlerts(t *testing.T) {
	database, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.EnsureAdmin("admin", "a-long-development-password"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := database.CreateSession("admin", "a-long-development-password", -time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, _, err := database.CreateSession("admin", "a-long-development-password", time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := database.PruneExpiredSessions(time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	assertCount(t, database, "sessions", 1)

	now := time.Now().UTC()
	for index := 0; index < 105; index++ {
		id := fmt.Sprintf("alert-%03d", index)
		alert := model.Alert{ID: id, Severity: "warning", Title: "test", Description: "test", State: "firing", StartedAt: now.Add(-time.Duration(index) * time.Minute)}
		alert.Resource = &model.ResourceRef{Type: "disk", ID: id}
		opened, err := database.OpenGeneratedAlert(alert, "rule-retention")
		if err != nil || !opened {
			t.Fatalf("open alert %s: opened=%v err=%v", id, opened, err)
		}
		if _, err := database.ResolveGeneratedAlerts("rule-retention", id); err != nil {
			t.Fatal(err)
		}
	}
	active := model.Alert{ID: "alert-active", Severity: "critical", Title: "active", Description: "active", State: "firing", StartedAt: now}
	active.Resource = &model.ResourceRef{Type: "disk", ID: "active"}
	if opened, err := database.OpenGeneratedAlert(active, "rule-retention"); err != nil || !opened {
		t.Fatalf("open active alert: opened=%v err=%v", opened, err)
	}
	if err := database.PruneGeneratedAlerts(100); err != nil {
		t.Fatal(err)
	}
	assertCount(t, database, "generated_alerts", 101)
	assertExists(t, database, "generated_alerts", "alert-active")
}

func assertCount(t *testing.T, database *Store, table string, want int) {
	t.Helper()
	var got int
	if err := database.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("%s: expected %d rows, got %d", table, want, got)
	}
}

func assertExists(t *testing.T, database *Store, table, id string) {
	t.Helper()
	var count int
	column := "id"
	if table == "storage_operations" || table == "network_checkpoints" {
		column = "operation_id"
	}
	if err := database.db.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE "+column+"=?", id).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("%s %s was pruned", table, id)
	}
}
