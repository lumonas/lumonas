package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSystemLogsAppliesAllowListedServiceAndSearchFilters(t *testing.T) {
	server := testServer(t)
	previous := runSystemJournal
	t.Cleanup(func() { runSystemJournal = previous })
	var command string
	var args []string
	runSystemJournal = func(_ context.Context, name string, values ...string) ([]byte, error) {
		command = name
		args = append([]string(nil), values...)
		return []byte("{\"__REALTIME_TIMESTAMP\":\"1780000000000000\",\"PRIORITY\":\"3\",\"_SYSTEMD_UNIT\":\"lumonasd.service\",\"MESSAGE\":\"share quota warning\"}\n{\"__REALTIME_TIMESTAMP\":\"1780000001000000\",\"PRIORITY\":\"6\",\"_SYSTEMD_UNIT\":\"lumonasd.service\",\"MESSAGE\":\"routine status\"}\n"), nil
	}
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/system/logs?unit=lumonasd.service&since=1%20hour%20ago&q=quota&limit=50", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "share quota warning") || strings.Contains(response.Body.String(), "routine status") {
		t.Fatalf("log search filter failed: %d %s", response.Code, response.Body.String())
	}
	joined := strings.Join(args, " ")
	if command != "journalctl" || !strings.Contains(joined, "--unit=lumonasd.service") || !strings.Contains(joined, "--since=1 hour ago") || !strings.Contains(joined, "--lines=50") {
		t.Fatalf("journal filters were not passed as separate safe arguments: %s %v", command, args)
	}
}

func TestSystemLogsRejectsUnsafeOrExcessiveFilters(t *testing.T) {
	server := testServer(t)
	for _, target := range []string{
		"/api/v1/system/logs?unit=attacker.service",
		"/api/v1/system/logs?since=%0A--output=cat",
		"/api/v1/system/logs?q=" + strings.Repeat("x", 257),
	} {
		response := httptest.NewRecorder()
		server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
		if response.Code < 400 || response.Code >= 500 {
			t.Fatalf("unsafe log query accepted: %s -> %d %s", target, response.Code, response.Body.String())
		}
	}
}

func TestSystemLogsCanFilterSMBShareAuditIdentifier(t *testing.T) {
	server := testServer(t)
	previous := runSystemJournal
	t.Cleanup(func() { runSystemJournal = previous })
	var args []string
	runSystemJournal = func(_ context.Context, name string, values ...string) ([]byte, error) {
		args = append([]string(nil), values...)
		return []byte("{\"__REALTIME_TIMESTAMP\":\"1780000000000000\",\"PRIORITY\":\"5\",\"SYSLOG_IDENTIFIER\":\"smbd_audit\",\"MESSAGE\":\"alice|192.0.2.10|renameat|ok|docs|old.txt|new.txt\"}\n"), nil
	}
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/system/logs?source=smb-audit", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "renameat") || !strings.Contains(response.Body.String(), "smbd_audit") {
		t.Fatalf("SMB audit filter failed: %d %s", response.Code, response.Body.String())
	}
	if !strings.Contains(strings.Join(args, " "), "--identifier=smbd_audit") {
		t.Fatalf("SMB audit identifier missing from journal arguments: %v", args)
	}
}
