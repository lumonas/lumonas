package trace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

type contextKey struct{}

func WithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, contextKey{}, id)
}

func CorrelationID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(contextKey{}).(string)
	return id
}

func NewCorrelationID() string {
	var bytes [8]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("corr-%d", time.Now().UnixNano())
	}
	return "corr-" + hex.EncodeToString(bytes[:])
}
