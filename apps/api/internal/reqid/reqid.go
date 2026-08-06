// Package reqid shares the request correlation ID across HTTP middleware,
// service transactions and audit events.
package reqid

import "context"

type contextKey struct{}

// WithValue returns a context carrying the request correlation ID.
func WithValue(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, contextKey{}, id)
}

// FromContext returns the request correlation ID, if any.
func FromContext(ctx context.Context) string {
	id, _ := ctx.Value(contextKey{}).(string)
	return id
}
