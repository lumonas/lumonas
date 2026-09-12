package main

import (
	"strings"
	"testing"
)

func TestPrivilegedRequestLogAttributesExcludeSensitivePayloads(t *testing.T) {
	request := request{
		Operation:      "filesystem.format",
		OperationID:    "operation-1",
		CorrelationID:  "correlation-1",
		PlanHash:       "plan-1",
		TargetDiskID:   "serial:disk-1",
		RequestedState: map[string]any{"password": "do-not-log"},
		ExpectedIdentity: map[string]string{
			"serial": "serial-number",
		},
	}
	attrs := privilegedRequestAttrs(request)
	joined := strings.Join(func() []string {
		values := make([]string, 0, len(attrs))
		for _, value := range attrs {
			values = append(values, value.(string))
		}
		return values
	}(), " ")
	if !strings.Contains(joined, "operation-1") || !strings.Contains(joined, "plan-1") {
		t.Fatalf("stable request metadata was omitted: %v", attrs)
	}
	if strings.Contains(joined, "do-not-log") || strings.Contains(joined, "serial-number") {
		t.Fatalf("sensitive request payload was logged: %v", attrs)
	}
}
