package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/monitoring"
	"github.com/lumonas/lumonas/internal/notify"
)

type notificationRoundTripper func(*http.Request) (*http.Response, error)

func (roundTrip notificationRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func TestNotificationChannelAPIStoresSecretsWithoutReturningThem(t *testing.T) {
	server := testServer(t)
	t.Setenv("LUMONAS_RECOVERY_KEY", "notification-key")
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
	t.Setenv("LUMONAS_RECOVERY_KEY", "notification-test-key")
	var received bool
	server.notificationClient = &http.Client{Transport: notificationRoundTripper(func(r *http.Request) (*http.Response, error) {
		received = r.Method == http.MethodPost
		return &http.Response{StatusCode: http.StatusNoContent, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	})}

	save := httptest.NewRecorder()
	server.routes().ServeHTTP(save, httptest.NewRequest(http.MethodPost, "/api/v1/notification-channels", strings.NewReader(`{"id":"channel-test","type":"webhook","label":"Test","target":"https://provider.test/webhook","enabled":true,"credentials":{"token":"not-returned"}}`)))
	if save.Code != http.StatusOK {
		t.Fatalf("channel save failed: %d %s", save.Code, save.Body.String())
	}
	test := httptest.NewRecorder()
	server.routes().ServeHTTP(test, httptest.NewRequest(http.MethodPost, "/api/v1/notification-channels/channel-test/test", strings.NewReader(`{"title":"hello","body":"world"}`)))
	if test.Code != http.StatusOK || !strings.Contains(test.Body.String(), `"sent":true`) || !received {
		t.Fatalf("channel test failed: %d %s received=%v", test.Code, test.Body.String(), received)
	}
}

func TestNotificationDeliveryListIsAuthenticatedAndRedacted(t *testing.T) {
	server := testServer(t)
	if err := server.store.SaveNotificationDelivery(notify.Delivery{ID: "delivery-2", ChannelID: "channel-1", EventType: "disk.smart.warning", State: "failed", AttemptedAt: time.Now().UTC(), Error: "provider returned HTTP 503"}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/notification-deliveries", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"state":"failed"`) {
		t.Fatalf("unexpected delivery response: %d %s", response.Code, response.Body.String())
	}
}

func TestNotificationFailuresAreTemporarilySuppressedAndRecover(t *testing.T) {
	server := testServer(t)
	for range 3 {
		server.recordNotificationDeliveryFailure("channel-1", "disk.smart.warning", true)
	}
	if server.notificationDeliveryAllowed("channel-1", "disk.smart.warning") {
		t.Fatal("repeated notification failures were not suppressed")
	}
	server.recordNotificationDeliveryFailure("channel-1", "disk.smart.warning", false)
	if !server.notificationDeliveryAllowed("channel-1", "disk.smart.warning") {
		t.Fatal("successful notification did not clear suppression")
	}
}

func TestNotificationSuppressionStartsFreshAfterExpiry(t *testing.T) {
	server := testServer(t)
	key := "channel-1\x00disk.smart.warning"
	server.notificationFailures = map[string]notificationFailureState{
		key: {Failures: 3, SuppressedUntil: time.Now().UTC().Add(-time.Second)},
	}
	if !server.notificationDeliveryAllowed("channel-1", "disk.smart.warning") {
		t.Fatal("expired notification suppression did not clear")
	}
	server.recordNotificationDeliveryFailure("channel-1", "disk.smart.warning", true)
	if server.notificationFailures[key].Failures != 1 {
		t.Fatalf("expected fresh failure window, got %#v", server.notificationFailures[key])
	}
}
