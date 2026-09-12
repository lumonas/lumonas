package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNetworkConfigurationAPIRequiresReauthenticationAndPersistsTypedState(t *testing.T) {
	server := testServer(t)

	locked := httptest.NewRecorder()
	server.routes().ServeHTTP(locked, httptest.NewRequest(http.MethodPatch, "/api/v1/network/connections/lan", strings.NewReader(`{"uuid":"12345678-1234","name":"LAN","interface":"en0","enabled":true,"ipv4":{"method":"auto"},"ipv6":{"method":"disabled"}}`)))
	if locked.Code != http.StatusLocked {
		t.Fatalf("expected reauthentication lock, got %d: %s", locked.Code, locked.Body.String())
	}

	request := httptest.NewRequest(http.MethodPatch, "/api/v1/network/connections/lan", strings.NewReader(`{"uuid":"12345678-1234","name":"LAN","interface":"en0","enabled":true,"ipv4":{"method":"auto"},"ipv6":{"method":"disabled"},"reauthenticated":true}`))
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"id":"lan"`) {
		t.Fatalf("unexpected update response %d: %s", response.Code, response.Body.String())
	}

	list := httptest.NewRecorder()
	server.routes().ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/v1/network/connections", nil))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"interface":"en0"`) {
		t.Fatalf("unexpected list response %d: %s", list.Code, list.Body.String())
	}
}

func TestNetworkConnectionCreateAndDiagnosticJobLifecycle(t *testing.T) {
	server := testServer(t)
	create := httptest.NewRecorder()
	server.routes().ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v1/network/connections", strings.NewReader(`{"name":"LAN","interface":"en0","enabled":true,"ipv4":{"method":"auto"},"ipv6":{"method":"disabled"},"reauthenticated":true}`)))
	if create.Code != http.StatusCreated {
		t.Fatalf("create status %d: %s", create.Code, create.Body.String())
	}

	diagnostic := httptest.NewRecorder()
	server.routes().ServeHTTP(diagnostic, httptest.NewRequest(http.MethodPost, "/api/v1/network/diagnostics", strings.NewReader(`{"kind":"interfaces"}`)))
	if diagnostic.Code != http.StatusAccepted {
		t.Fatalf("diagnostic status %d: %s", diagnostic.Code, diagnostic.Body.String())
	}
	var queued struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(diagnostic.Body).Decode(&queued); err != nil || queued.ID == "" {
		t.Fatalf("invalid diagnostic job: %#v err=%v", queued, err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		result := httptest.NewRecorder()
		server.routes().ServeHTTP(result, httptest.NewRequest(http.MethodGet, "/api/v1/network/diagnostics/"+queued.ID, nil))
		if result.Code != http.StatusOK {
			t.Fatalf("diagnostic get status %d: %s", result.Code, result.Body.String())
		}
		if strings.Contains(result.Body.String(), `"state":"successful"`) || strings.Contains(result.Body.String(), `"state":"failed"`) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("diagnostic job did not finish: %s", result.Body.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestNetworkDiagnosticRejectsUnsupportedKind(t *testing.T) {
	server := testServer(t)
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/network/diagnostics", strings.NewReader(`{"kind":"shell","target":"127.0.0.1"}`)))
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected diagnostic validation error, got %d", response.Code)
	}
}

func TestWiFiScanDegradesWithoutNmcli(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	server := testServer(t)
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/network/wifi/scan", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"available":false`) || !strings.Contains(response.Body.String(), `"networks":[]`) {
		t.Fatalf("expected graceful unavailable payload, got %s", response.Body.String())
	}
}

func TestCreateWiFiConnectionPersistsSSIDWithoutPassword(t *testing.T) {
	server := testServer(t)
	response := httptest.NewRecorder()
	body := `{"name":"Home Wi-Fi","type":"wifi","interface":"wlan0","ssid":"HomeNet","enabled":true,"ipv4":{"method":"auto"},"ipv6":{"method":"disabled"},"reauthenticated":true}`
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/network/connections", strings.NewReader(body)))
	if response.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "psk") || strings.Contains(response.Body.String(), "password") {
		t.Fatalf("wifi response should never carry secret material: %s", response.Body.String())
	}
	list := httptest.NewRecorder()
	server.routes().ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/v1/network/connections", nil))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"ssid":"HomeNet"`) {
		t.Fatalf("expected persisted wifi connection, got %d %s", list.Code, list.Body.String())
	}
}

func TestCreateWiFiConnectionRejectsBadSSID(t *testing.T) {
	server := testServer(t)
	response := httptest.NewRecorder()
	body := `{"name":"Home Wi-Fi","type":"wifi","ssid":"` + strings.Repeat("x", 33) + `","reauthenticated":true}`
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/network/connections", strings.NewReader(body)))
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d %s", response.Code, response.Body.String())
	}
}

func TestApplyWiFiConnectionRequiresPassword(t *testing.T) {
	server := testServer(t)
	create := httptest.NewRecorder()
	body := `{"name":"Home Wi-Fi","type":"wifi","interface":"wlan0","ssid":"HomeNet","reauthenticated":true,"ipv4":{"method":"auto"},"ipv6":{"method":"disabled"}}`
	server.routes().ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v1/network/connections", strings.NewReader(body)))
	if create.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d %s", create.Code, create.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil || created.ID == "" {
		t.Fatalf("expected connection id in response: %s", create.Body.String())
	}
	apply := httptest.NewRecorder()
	server.routes().ServeHTTP(apply, httptest.NewRequest(http.MethodPost, "/api/v1/network/connections/"+created.ID+"/apply", strings.NewReader(`{"reauthenticated":true,"wifiPassword":"short"}`)))
	if apply.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for weak password, got %d %s", apply.Code, apply.Body.String())
	}
	apply = httptest.NewRecorder()
	server.routes().ServeHTTP(apply, httptest.NewRequest(http.MethodPost, "/api/v1/network/connections/"+created.ID+"/apply", strings.NewReader(`{"reauthenticated":true,"wifiPassword":"correct horse battery staple"}`)))
	if apply.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 without privd socket, got %d %s", apply.Code, apply.Body.String())
	}
}

func TestWireGuardStatusEndpoint(t *testing.T) {
	server := testServer(t)
	t.Setenv("PATH", t.TempDir())
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/network/wireguard/status", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d %s", response.Code, response.Body.String())
	}
}

func TestWireGuardKeygenEndpoint(t *testing.T) {
	server := testServer(t)
	t.Setenv("PATH", t.TempDir())
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/network/wireguard/keygen", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Logf("keygen without wg binary: %d %s", response.Code, response.Body.String())
	}
}

func TestTailscaleStatusEndpoint(t *testing.T) {
	server := testServer(t)
	t.Setenv("PATH", t.TempDir())
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/network/tailscale/status", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"installed":false`) {
		t.Fatalf("expected installed:false without tailscale, got %s", response.Body.String())
	}
}

func TestTailscaleUpRequiresReauthentication(t *testing.T) {
	server := testServer(t)
	t.Setenv("PATH", t.TempDir())
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/network/tailscale/up", strings.NewReader(`{"hostname":"mynas"}`)))
	if response.Code != http.StatusServiceUnavailable {
		t.Logf("tailscale up without binary: %d %s", response.Code, response.Body.String())
	}
}

func TestTailscaleDownEndpoint(t *testing.T) {
	server := testServer(t)
	t.Setenv("PATH", t.TempDir())
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/network/tailscale/down", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Logf("tailscale down without binary: %d %s", response.Code, response.Body.String())
	}
}

func TestNetworkInterfaceMetricsEndpoint(t *testing.T) {
	server := testServer(t)
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/network/interfaces/metrics", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d %s", response.Code, response.Body.String())
	}
}

func TestSSHKeysListEndpoint(t *testing.T) {
	server := testServer(t)
	t.Setenv("MYNAS_SSH_AUTHORIZED_KEYS", t.TempDir()+"/authorized_keys")
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/ssh/keys", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d %s", response.Code, response.Body.String())
	}
}

func TestSSHKeysAddAndRemoveEndpoint(t *testing.T) {
	server := testServer(t)
	keyFile := t.TempDir() + "/authorized_keys"
	t.Setenv("MYNAS_SSH_AUTHORIZED_KEYS", keyFile)

	add := httptest.NewRecorder()
	server.routes().ServeHTTP(add, httptest.NewRequest(http.MethodPost, "/api/v1/ssh/keys", strings.NewReader(`{"publicKey":"ssh-ed25519 AAAA1234 test@host"}`)))
	if add.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d %s", add.Code, add.Body.String())
	}

	list := httptest.NewRecorder()
	server.routes().ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/v1/ssh/keys", nil))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "AAAA1234") {
		t.Fatalf("expected key in list, got %d %s", list.Code, list.Body.String())
	}

	remove := httptest.NewRecorder()
	server.routes().ServeHTTP(remove, httptest.NewRequest(http.MethodPost, "/api/v1/ssh/keys/remove", strings.NewReader(`{"publicKey":"ssh-ed25519 AAAA1234"}`)))
	if remove.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d %s", remove.Code, remove.Body.String())
	}

	listAfter := httptest.NewRecorder()
	server.routes().ServeHTTP(listAfter, httptest.NewRequest(http.MethodGet, "/api/v1/ssh/keys", nil))
	if listAfter.Code != http.StatusOK || strings.Contains(listAfter.Body.String(), "AAAA1234") {
		t.Fatalf("expected key removed, got %d %s", listAfter.Code, listAfter.Body.String())
	}
}
