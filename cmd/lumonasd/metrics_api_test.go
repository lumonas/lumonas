package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

func TestMetricsHistoryValidatesBoundsAndReturnsSamples(t *testing.T) {
	server := testServer(t)
	if err := server.store.SaveSystemMetricSample(model.SystemMetricSample{CapturedAt: time.Now().UTC(), Metrics: model.SystemMetrics{CPUPercent: 42}}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.metricsHistory(response, httptest.NewRequest(http.MethodGet, "/api/v1/system/metrics/history?hours=24&limit=10", nil))
	if response.Code != http.StatusOK || response.Body.Len() == 0 {
		t.Fatalf("unexpected metrics history response: status=%d body=%s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	server.metricsHistory(response, httptest.NewRequest(http.MethodGet, "/api/v1/system/metrics/history?hours=0", nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid hours rejection, got %d", response.Code)
	}
}
