package authmiddleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	jwtlib "github.com/golang-jwt/jwt/v5"
	pb "github.com/kabradshaw1/portfolio/go/auth-service/pb/auth/v1"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
	"google.golang.org/grpc"
)

func init() { gin.SetMode(gin.TestMode) }

type fakeAuthClient struct {
	resp *pb.CheckTokenResponse
	err  error
}

func (f *fakeAuthClient) CheckToken(_ context.Context, _ *pb.CheckTokenRequest, _ ...grpc.CallOption) (*pb.CheckTokenResponse, error) {
	return f.resp, f.err
}

func signToken(t *testing.T, secret, userID string) string {
	t.Helper()
	claims := jwtlib.MapClaims{
		"sub": userID,
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	tok := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims)
	s, err := tok.SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func newTestRouter(mw gin.HandlerFunc) *gin.Engine {
	r := gin.New()
	r.Use(apperror.ErrorHandler())
	r.Use(mw)
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"userId": c.GetString("userId")})
	})
	return r
}

func TestValidToken_Allowed(t *testing.T) {
	secret := "test-secret"
	token := signToken(t, secret, "user-1")
	client := &fakeAuthClient{resp: &pb.CheckTokenResponse{Valid: true, UserId: "user-1"}}
	mw := New(secret, client)
	r := newTestRouter(mw)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMissingToken_Returns401(t *testing.T) {
	client := &fakeAuthClient{}
	mw := New("secret", client)
	r := newTestRouter(mw)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 401 {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestRevokedToken_Returns401(t *testing.T) {
	secret := "test-secret"
	token := signToken(t, secret, "user-2")
	client := &fakeAuthClient{resp: &pb.CheckTokenResponse{Valid: false, Reason: "revoked"}}
	mw := New(secret, client)
	r := newTestRouter(mw)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 401 {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCookieToken_Allowed(t *testing.T) {
	secret := "test-secret"
	token := signToken(t, secret, "user-3")
	client := &fakeAuthClient{resp: &pb.CheckTokenResponse{Valid: true, UserId: "user-3"}}
	mw := New(secret, client)
	r := newTestRouter(mw)

	req := httptest.NewRequest("GET", "/test", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: token})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCacheHit_SkipsDenylistCall(t *testing.T) {
	secret := "test-secret"
	token := signToken(t, secret, "user-4")

	callCount := 0
	client := &fakeAuthClient{resp: &pb.CheckTokenResponse{Valid: true, UserId: "user-4"}}
	countingClient := &countingAuthClient{inner: client, count: &callCount}
	mw := New(secret, countingClient, WithCacheTTL(5*time.Second))
	r := newTestRouter(mw)

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("request %d: expected 200, got %d", i, w.Code)
		}
	}

	if callCount != 1 {
		t.Fatalf("expected 1 denylist call (cached), got %d", callCount)
	}
}

func TestSkipPaths(t *testing.T) {
	client := &fakeAuthClient{}
	mw := New("secret", client, WithSkipPaths("/health", "/metrics"))

	r := gin.New()
	r.Use(mw)
	r.GET("/health", func(c *gin.Context) { c.String(200, "ok") })

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200 for skipped path, got %d", w.Code)
	}
}

type countingAuthClient struct {
	inner AuthChecker
	count *int
}

func (c *countingAuthClient) CheckToken(ctx context.Context, req *pb.CheckTokenRequest, opts ...grpc.CallOption) (*pb.CheckTokenResponse, error) {
	*c.count++
	return c.inner.CheckToken(ctx, req, opts...)
}
