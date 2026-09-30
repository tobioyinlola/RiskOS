package httpx

import "context"

type rawBodyKey struct{}

// WithRawBody returns ctx carrying the request body exactly as received. The contract validator
// sets it, so handlers that must store the original payload do not read the body twice.
func WithRawBody(ctx context.Context, b []byte) context.Context {
	return context.WithValue(ctx, rawBodyKey{}, b)
}

// RawBody returns the body stored by WithRawBody.
func RawBody(ctx context.Context) ([]byte, bool) {
	b, ok := ctx.Value(rawBodyKey{}).([]byte)
	return b, ok
}
