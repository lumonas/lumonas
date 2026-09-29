package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lumonas/lumonas/internal/store"
)

func TestReplicationPeerHealthCheckAndOverview(t *testing.T) {
	server := testServer(t)
	previous := replicationProbeClient
	replicationProbeClient = func() *http.Client { return &http.Client{Transport: replicationTestTransport{}} }
	t.Cleanup(func() { replicationProbeClient = previous })
	if !probeReplicationPeer(context.Background(), "https://remote.example") {
		t.Fatal("healthy remote NAS was reported offline")
	}
	if probeReplicationPeer(context.Background(), "http://127.0.0.1:1234") {
		t.Fatal("non-HTTPS peer was treated as healthy")
	}
	if err := server.store.SaveReplicationPeer(store.ReplicationPeer{ID: "peer-test", Name: "Remote NAS", URL: "https://remote.example"}, []byte("encrypted")); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/replication/peers", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("overview status %d: %s", response.Code, response.Body.String())
	}
	var peers []store.ReplicationPeer
	if err := json.NewDecoder(response.Body).Decode(&peers); err != nil {
		t.Fatal(err)
	}
	if len(peers) != 1 || peers[0].RemoteStatus != "online" {
		t.Fatalf("remote status missing: %#v", peers)
	}
}

func TestReplicationFleetProbeUsesNarrowBearerAndReadsVersion(t *testing.T) {
	previous := replicationProbeClient
	replicationProbeClient = func() *http.Client { return &http.Client{Transport: fleetStatusTestTransport{}} }
	t.Cleanup(func() { replicationProbeClient = previous })
	status, version, health, checkedAt := probeReplicationPeerDetails(context.Background(), "https://remote.example", "fleet-token")
	if status != "online" || version != "2.4.1" || health != "healthy" || checkedAt == nil {
		t.Fatalf("fleet version status was not read: status=%q version=%q health=%q checked=%v", status, version, health, checkedAt)
	}
}

type replicationTestTransport struct{}

func (replicationTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Path != "/healthz" {
		return nil, io.ErrUnexpectedEOF
	}
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"status":"ok"}`)), Request: request}, nil
}

type fleetStatusTestTransport struct{}

func (fleetStatusTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Path != "/api/v1/fleet/status" || request.Header.Get("Authorization") != "Bearer fleet-token" {
		return nil, io.ErrUnexpectedEOF
	}
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"status":"ok","version":"2.4.1","health":"healthy"}`)), Request: request}, nil
}
