package trace

import (
	"context"
	"testing"
)

func TestCorrelationIDRoundTrip(t *testing.T) {
	ctx := WithCorrelationID(context.Background(), "corr-test")
	if got := CorrelationID(ctx); got != "corr-test" {
		t.Fatalf("unexpected correlation id %q", got)
	}
}

func TestNewCorrelationIDHasStablePrefix(t *testing.T) {
	if id := NewCorrelationID(); len(id) < len("corr-")+8 || id[:len("corr-")] != "corr-" {
		t.Fatalf("unexpected generated correlation id %q", id)
	}
}
