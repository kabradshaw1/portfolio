package jwtctx

import "context"

type userIDKey struct{}

// WithUserID returns ctx with id attached for downstream consumers.
func WithUserID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, userIDKey{}, id)
}

// UserID returns the user id attached to ctx, or empty string if none.
func UserID(ctx context.Context) string {
	v, _ := ctx.Value(userIDKey{}).(string)
	return v
}
