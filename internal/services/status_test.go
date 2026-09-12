package services

import (
	"context"
	"testing"
)

func TestCollectReturnsEveryRequestedService(t *testing.T) {
	values := Collect(context.Background(), []string{"service-that-does-not-exist.service"})
	if len(values) != 1 || values[0].Name == "" || values[0].Active {
		t.Fatalf("unexpected service status %#v", values)
	}
}
