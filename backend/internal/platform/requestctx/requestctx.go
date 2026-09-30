// Package requestctx carries request-scoped values through context.Context.
//
// Only correlation data lives here. Security principals (tenant, actor) will get their own typed
// accessors during system design (TODO(phase-0)) and are never read from client-supplied input.
package requestctx

import "context"

type requestIDKey struct{}

// WithRequestID returns a copy of ctx carrying the request ID.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestID returns the request ID stored in ctx, or "" if none is set.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}
