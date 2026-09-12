package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lumonas/lumonas/internal/monitoring"
)

func TestNotificationChannelAPIStoresSecretsWithoutReturningThem(t *testing.T) {
	server := testServer(t)
	t.Setenv("MYNAS_RECOVERY_KEY", "notification-key")
	request := httptest.NewRequest(http.MethodPost, "/api/v1/notification-channels", strings.NewReader(`{"type":"telegram","label":"Operations","target":"telegram://ops","enabled":true,"credentials":{"token":"super-secret"}}`))
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "super-secret") || !strings.Contains(response.Body.String(), `"configured":true`) {
		t.Fatalf("unexpected save response %d: %s", response.Code, response.Body.String())
	}
	list := httptest.NewRecorder()
	server.routes().ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/v1/notification-channels", nil))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "Operations") || strings.Contains(list.Body.String(), "super-secret") {
		t.Fatalf("unexpected channel list %d: %s", list.Code, list.Body.String())
	}
}

func TestNotificationRuleAPIValidatesRequiredFields(t *testing.T) {
	server := testServer(t)
	invalid := httptest.NewRecorder()
	server.routes().ServeHTTP(invalid, httptest.NewRequest(http.MethodPost, "/api/v1/notification-rules", strings.NewReader(`{"severity":"warning"}`)))
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected rule validation, got %d: %s", invalid.Code, invalid.Body.String())
	}
}

func TestNotificationRoutingHonorsSeverityAndCategoryRoutes(t *testing.T) {
	server := testServer(t)
	if err := server.store.SaveAlertRule(monitoring.AlertRule{ID: "rule-disk", Name: "Disk temperature", Condition: "disk temperature is high", Severity: "warning", Routes: []string{"channel-ntfy"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if routes := server.notificationRoutes("disk.temperature.changed", "critical"); !routes["channel-ntfy"] || routes["channel-slack"] {
		t.Fatalf("unexpected disk routes: %#v", routes)
	}
	if routes := server.notificationRoutes("docker.container.unhealthy", "critical"); routes["channel-ntfy"] {
		t.Fatalf("disk route leaked into Docker event: %#v", routes)
	}
}

func TestNotificationChannelSendTestUsesEncryptedChannelCredentials(t *testing.T) {
	server := testServer(t)
	t.Setenv("MYNAS_RECOVERY_KEY", "notification-test-key")
	var received bool
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = r.Method == http.MethodPost
		w.WriteHeader(http.StatusNoContent)
	}))
	defer provider.Close()

	save := httptest.NewRecorder()
	server.routes().ServeHTTP(save, httptest.NewRequest(http.MethodPost, "/api/v1/notification-channels", strings.NewReader(`{"id":"channel-test","type":"webhook","label":"Test","target":"`+provider.URL+`","enabled":true,"credentials":{"token":"not-returned"}}`)))
	if save.Code != http.StatusOK {
		t.Fatalf("channel save failed: %d %s", save.Code, save.Body.String())
	}
	test := httptest.NewRecorder()
	server.routes().ServeHTTP(test, httptest.NewRequest(http.MethodPost, "/api/v1/notification-channels/channel-test/test", strings.NewReader(`{"title":"hello","body":"world"}`)))
	if test.Code != http.StatusOK || !strings.Contains(test.Body.String(), `"sent":true`) || !received {
		t.Fatalf("channel test failed: %d %s received=%v", test.Code, test.Body.String(), received)
	}
}
