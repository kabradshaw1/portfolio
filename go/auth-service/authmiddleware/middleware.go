package authmiddleware

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	jwtlib "github.com/golang-jwt/jwt/v5"
	pb "github.com/kabradshaw1/portfolio/go/auth-service/pb/auth/v1"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
	"google.golang.org/grpc"
)

// AuthChecker is the subset of the generated AuthServiceClient we need.
type AuthChecker interface {
	CheckToken(ctx context.Context, in *pb.CheckTokenRequest, opts ...grpc.CallOption) (*pb.CheckTokenResponse, error)
}

type cacheEntry struct {
	resp      *pb.CheckTokenResponse
	expiresAt time.Time
}

type options struct {
	cacheTTL  time.Duration
	skipPaths map[string]bool
}

// Option configures the middleware.
type Option func(*options)

// WithCacheTTL sets how long denylist responses are cached per token.
func WithCacheTTL(d time.Duration) Option {
	return func(o *options) { o.cacheTTL = d }
}

// WithSkipPaths sets paths that bypass auth entirely.
func WithSkipPaths(paths ...string) Option {
	return func(o *options) {
		for _, p := range paths {
			o.skipPaths[p] = true
		}
	}
}

// New creates Gin middleware that validates JWTs locally and checks the
// auth-service denylist via gRPC, caching results.
func New(jwtSecret string, checker AuthChecker, opts ...Option) gin.HandlerFunc {
	cfg := &options{
		cacheTTL:  30 * time.Second,
		skipPaths: make(map[string]bool),
	}
	for _, o := range opts {
		o(cfg)
	}

	var mu sync.RWMutex
	cache := make(map[string]cacheEntry)

	return func(c *gin.Context) {
		if cfg.skipPaths[c.Request.URL.Path] {
			c.Next()
			return
		}

		tokenStr := extractToken(c)
		if tokenStr == "" {
			_ = c.Error(apperror.Unauthorized("MISSING_AUTH", "missing authorization"))
			c.Abort()
			return
		}

		// Fast path: validate signature + expiry locally.
		claims := jwtlib.MapClaims{}
		_, err := jwtlib.ParseWithClaims(tokenStr, claims, func(t *jwtlib.Token) (any, error) {
			if _, ok := t.Method.(*jwtlib.SigningMethodHMAC); !ok {
				return nil, jwtlib.ErrSignatureInvalid
			}
			return []byte(jwtSecret), nil
		})
		if err != nil {
			_ = c.Error(apperror.Unauthorized("INVALID_TOKEN", "invalid or expired token"))
			c.Abort()
			return
		}

		sub, ok := claims["sub"].(string)
		if !ok || sub == "" {
			_ = c.Error(apperror.Unauthorized("INVALID_TOKEN", "invalid token claims"))
			c.Abort()
			return
		}

		// Check denylist with cache.
		if resp, hit := getCached(&mu, cache, tokenStr); hit {
			if !resp.Valid {
				_ = c.Error(apperror.Unauthorized("TOKEN_REVOKED", resp.Reason))
				c.Abort()
				return
			}
		} else {
			resp, err := checker.CheckToken(c.Request.Context(), &pb.CheckTokenRequest{Token: tokenStr})
			if err == nil {
				putCached(&mu, cache, tokenStr, resp, cfg.cacheTTL)
				if !resp.Valid {
					_ = c.Error(apperror.Unauthorized("TOKEN_REVOKED", resp.Reason))
					c.Abort()
					return
				}
			}
			// On gRPC error, fail open -- local validation already passed.
		}

		c.Set("userId", sub)
		c.Next()
	}
}

func extractToken(c *gin.Context) string {
	if h := c.GetHeader("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	if cookie, err := c.Cookie("access_token"); err == nil && cookie != "" {
		return cookie
	}
	return ""
}

func getCached(mu *sync.RWMutex, cache map[string]cacheEntry, key string) (*pb.CheckTokenResponse, bool) {
	mu.RLock()
	defer mu.RUnlock()
	entry, ok := cache[key]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.resp, true
}

func putCached(mu *sync.RWMutex, cache map[string]cacheEntry, key string, resp *pb.CheckTokenResponse, ttl time.Duration) {
	mu.Lock()
	defer mu.Unlock()
	cache[key] = cacheEntry{resp: resp, expiresAt: time.Now().Add(ttl)}
}
