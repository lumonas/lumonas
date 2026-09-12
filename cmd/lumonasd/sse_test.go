package main

import (
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/events"
	"github.com/lumonas/lumonas/internal/model"
)

func TestWriteSSEEventIncludesReplayIDAndSchemaVersion(t *testing.T) {
	response := httptest.NewRecorder()
	server := &apiServer{}
	server.writeSSEEvent(response, response, model.Event{
		ID:        "evt-replay",
		Type:      "job.state_changed",
		Timestamp: time.Now().UTC(),
		Severity:  "info",
		Data:      map[string]any{"state": "running"},
	})

	body := response.Body.String()
	if !strings.Contains(body, "id: evt-replay\n") {
		t.Fatalf("SSE frame has no replay id: %q", body)
	}
	if !strings.Contains(body, `"schemaVersion":`+strconv.Itoa(events.SchemaVersion)) {
		t.Fatalf("SSE frame has no schema version: %q", body)
	}
}
