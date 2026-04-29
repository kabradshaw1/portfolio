package mcp

import (
	"net/http"
	"strings"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/auth"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/jwtctx"
)

// OptionalJWTMiddleware extracts a Bearer token from the Authorization header,
// validates it, and injects the userID + raw JWT into the request context.
// If no token is present or the token is invalid, the request proceeds without
// auth — individual tools enforce their own auth requirements.
func OptionalJWTMiddleware(jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if authHeader := r.Header.Get("Authorization"); authHeader != "" {
				uid, err := auth.ParseBearer(authHeader, jwtSecret)
				if err == nil {
					ctx := WithUserID(r.Context(), uid)
					ctx = jwtctx.WithUserID(ctx, uid)
					ctx = jwtctx.WithJWT(ctx, strings.TrimPrefix(authHeader, "Bearer "))
					r = r.WithContext(ctx)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
