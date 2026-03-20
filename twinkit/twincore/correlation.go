package twincore

import (
	"context"

	chimw "github.com/go-chi/chi/v5/middleware"
)

type correlationIDKey struct{}

// CorrelationIDFromContext returns the correlation ID from the request context.
// Falls back to chi's RequestID if no explicit correlation ID was set.
func CorrelationIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(correlationIDKey{}).(string); ok && id != "" {
		return id
	}
	// Fall back to chi's RequestID (set by chimw.RequestID middleware).
	return chimw.GetReqID(ctx)
}

// ContextWithCorrelationID returns a new context with the given correlation ID.
func ContextWithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, correlationIDKey{}, id)
}
