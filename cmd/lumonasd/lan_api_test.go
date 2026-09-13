package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lumonas/lumonas/internal/network"
	"github.com/lumonas/lumonas/internal/privileged"
	"github.com/lumonas/lumonas/internal/store"
)

func lanTestServer(t *testing.T) (*apiServer, *[]privileged.Request) {
	t.Helper()
	server := testServer(t)
	requests := &[]privileged.Request{}
	server.brokerExec = func(_ context.Context, request privileged.Request) error {
		*requests = append(*requests, request)
		return nil
	}
	return server, requests
}

func TestLanScanPersistsObservations(t *testing.T) {
	server, _ := lanTestServer(t)
	server.lanScanImpl = func(context.Context) ([]network.LanHost, error) {
		return []network.LanHost{
			{MAC: "aa:bb:cc:dd:ee:01", IP: "192.168.1.10", Interface: "eth0", State: "reachable"},
			{MAC: "aa:bb:cc:dd:ee:02", IP: "192.168.1.11", Interface: "eth0", State: "stale"},
		}, nil
	}

	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/network/lan/scan", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var hosts []store.LanHostRecord
	if err := json.NewDecoder(response.Body).Decode(&hosts); err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 2 {
		t.Fatalf("unexpected scan result: %#v", hosts)
	}
}

func TestLanScanPreservesOperatorHostname(t *testing.T) {
	server, _ := lanTestServer(t)
	if err := server.store.UpsertLanHosts([]store.LanHostRecord{{MAC: "aa:bb:cc:dd:ee:01", Interface: "eth0", IP: "192.168.1.10"}}); err != nil {
		t.Fatal(err)
	}
	if err := server.store.RenameLanHost("aa:bb:cc:dd:ee:01", "eth0", "media-player"); err != nil {
		t.Fatal(err)
	}
	server.lanScanImpl = func(context.Context) ([]network.LanHost, error) {
		return []network.LanHost{{MAC: "aa:bb:cc:dd:ee:01", IP: "192.168.1.99", Interface: "eth0", State: "reachable"}}, nil
	}
	if _, err := server.lanScan(context.Background()); err != nil {
		t.Fatal(err)
	}
	hosts, err := server.store.LanHosts()
	if err != nil || len(hosts) != 1 {
		t.Fatalf("unexpected hosts %#v err=%v", hosts, err)
	}
	if hosts[0].Hostname != "media-player" {
		t.Fatalf("operator hostname lost: %#v", hosts[0])
	}
	if hosts[0].IP != "192.168.1.99" {
		t.Fatalf("observation not refreshed: %#v", hosts[0])
	}
}

func TestLanHostsListsInventory(t *testing.T) {
	server, _ := lanTestServer(t)
	if err := server.store.UpsertLanHosts([]store.LanHostRecord{{MAC: "aa:bb:cc:dd:ee:01", Interface: "eth0", IP: "192.168.1.10"}}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/network/lan/hosts", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var hosts []store.LanHostRecord
	if err := json.NewDecoder(response.Body).Decode(&hosts); err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 || hosts[0].MAC != "aa:bb:cc:dd:ee:01" {
		t.Fatalf("unexpected inventory: %#v", hosts)
	}
}

func TestLanWakeSendsBrokerRequest(t *testing.T) {
	server, requests := lanTestServer(t)
	if err := server.store.UpsertLanHosts([]store.LanHostRecord{{MAC: "aa:bb:cc:dd:ee:ff", Interface: "eth0", IP: "192.168.1.10"}}); err != nil {
		t.Fatal(err)
	}
	body := `{"mac":"AA:BB:CC:DD:EE:FF","interface":"eth0"}`
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/network/lan/hosts/wake", strings.NewReader(body)))
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	if len(*requests) != 1 {
		t.Fatalf("broker was not called: %#v", *requests)
	}
	request := (*requests)[0]
	if request.Operation != "network.wol.wake" || !request.Confirmed || request.OperationID == "" {
		t.Fatalf("unexpected broker request: %#v", request)
	}

	bad := httptest.NewRecorder()
	server.routes().ServeHTTP(bad, httptest.NewRequest(http.MethodPost, "/api/v1/network/lan/hosts/wake", strings.NewReader(`{"mac":"nope","interface":"eth0"}`)))
	if bad.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for invalid MAC, got %d", bad.Code)
	}
	unknown := httptest.NewRecorder()
	server.routes().ServeHTTP(unknown, httptest.NewRequest(http.MethodPost, "/api/v1/network/lan/hosts/wake", strings.NewReader(`{"mac":"aa:bb:cc:dd:ee:01","interface":"eth0"}`)))
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for undiscovered MAC, got %d", unknown.Code)
	}
}

func TestLanRenameStoresAlias(t *testing.T) {
	server, _ := lanTestServer(t)
	if err := server.store.UpsertLanHosts([]store.LanHostRecord{{MAC: "aa:bb:cc:dd:ee:01", Interface: "eth0", IP: "192.168.1.10"}}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/network/lan/hosts/rename", strings.NewReader(`{"mac":"aa:bb:cc:dd:ee:01","interface":"eth0","hostname":"nas-backup"}`)))
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	hosts, _ := server.store.LanHosts()
	if hosts[0].Hostname != "nas-backup" {
		t.Fatalf("hostname not stored: %#v", hosts[0])
	}

	bad := httptest.NewRecorder()
	server.routes().ServeHTTP(bad, httptest.NewRequest(http.MethodPost, "/api/v1/network/lan/hosts/rename", strings.NewReader(`{"mac":"aa:bb:cc:dd:ee:01","interface":"eth0","hostname":"has space"}`)))
	if bad.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for invalid hostname, got %d", bad.Code)
	}
	unknown := httptest.NewRecorder()
	server.routes().ServeHTTP(unknown, httptest.NewRequest(http.MethodPost, "/api/v1/network/lan/hosts/rename", strings.NewReader(`{"mac":"aa:bb:cc:dd:ee:02","interface":"eth0","hostname":"other"}`)))
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for undiscovered host, got %d", unknown.Code)
	}
}
