// Package jwtctx provides context helpers for propagating a bearer token
// through the request context. It has no dependencies on other internal
// packages, so both internal/http and internal/tools can import it without
// creating an import cycle.
package jwtctx

import "context"

type ctxKey int

const jwtKey ctxKey = iota

// WithJWT returns a new context that carries the user's bearer token.
func WithJWT(ctx context.Context, jwt string) context.Context {
	return context.WithValue(ctx, jwtKey, jwt)
}

// FromContext returns the bearer token attached by WithJWT, or "".
func FromContext(ctx context.Context) string {
	v, _ := ctx.Value(jwtKey).(string)
	return v
}
