package main

import (
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/quotas"
)

func TestQuotaAlertsOpenEscalateAndResolve(t *testing.T) {
	server := testServer(t)
	policy := quotas.Policy{ID: "quota-share-1", TargetType: "share", TargetID: "share-documents", LimitBytes: 100 << 20, WarningPercent: 80}
	server.evaluateQuotaAlerts([]quotas.Status{{Policy: policy, UsedBytes: 85 << 20, Percent: 85, State: "warning", MeasuredAt: time.Now().UTC().Format(time.RFC3339)}})
	alerts, err := server.store.GeneratedAlerts()
	if err != nil || len(alerts) != 1 || alerts[0].Severity != "warning" || alerts[0].Resource == nil || alerts[0].Resource.ID != policy.ID {
		t.Fatalf("quota warning alert not created: alerts=%#v err=%v", alerts, err)
	}
	server.evaluateQuotaAlerts([]quotas.Status{{Policy: policy, UsedBytes: 101 << 20, Percent: 101, State: "over", MeasuredAt: time.Now().UTC().Format(time.RFC3339)}})
	alerts, err = server.store.GeneratedAlerts()
	if err != nil || len(alerts) != 1 || alerts[0].Severity != "critical" {
		t.Fatalf("quota alert did not escalate: alerts=%#v err=%v", alerts, err)
	}
	history, err := server.store.GeneratedAlertHistory(10)
	if err != nil || len(history) != 1 || history[0].Severity != "warning" {
		t.Fatalf("warning alert was not retained in history during escalation: alerts=%#v err=%v", history, err)
	}
	server.evaluateQuotaAlerts([]quotas.Status{{Policy: policy, UsedBytes: 20 << 20, Percent: 20, State: "healthy", MeasuredAt: time.Now().UTC().Format(time.RFC3339)}})
	alerts, err = server.store.GeneratedAlerts()
	if err != nil || len(alerts) != 0 {
		t.Fatalf("quota alert did not resolve after usage recovered: alerts=%#v err=%v", alerts, err)
	}
	history, err = server.store.GeneratedAlertHistory(10)
	if err != nil || len(history) != 2 {
		t.Fatalf("resolved quota alert was not retained in history: alerts=%#v err=%v", history, err)
	}
}

func TestQuotaAlertsResolvePoliciesThatWereRemoved(t *testing.T) {
	server := testServer(t)
	policy := quotas.Policy{ID: "removed-quota", TargetType: "share", TargetID: "share-documents", LimitBytes: 100 << 20, WarningPercent: 80}
	server.evaluateQuotaAlerts([]quotas.Status{{Policy: policy, UsedBytes: 90 << 20, Percent: 90, State: "warning"}})
	server.evaluateQuotaAlerts(nil)
	alerts, err := server.store.GeneratedAlerts()
	if err != nil || len(alerts) != 0 {
		t.Fatalf("removed policy left a firing alert: alerts=%#v err=%v", alerts, err)
	}
}
