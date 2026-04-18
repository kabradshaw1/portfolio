package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kabradshaw1/portfolio/go/auth-service/internal/google"
	"github.com/kabradshaw1/portfolio/go/auth-service/internal/handler"
	"github.com/kabradshaw1/portfolio/go/auth-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/auth-service/internal/service"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

// --- mock repo ---

type mockUserRepo struct {
	users map[string]*model.User
}

func newMockUserRepo() *mockUserRepo {
	return &mockUserRepo{users: make(map[string]*model.User)}
}

func (m *mockUserRepo) Create(_ context.Context, email, passwordHash, name string) (*model.User, error) {
	if _, exists := m.users[email]; exists {
		return nil, fmt.Errorf("email already registered")
	}
	u := &model.User{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: &passwordHash,
		Name:         name,
		CreatedAt:    time.Now(),
	}
	m.users[email] = u
	return u, nil
}

func (m *mockUserRepo) FindByEmail(_ context.Context, email string) (*model.User, error) {
	u, ok := m.users[email]
	if !ok {
		return nil, fmt.Errorf("user not found")
	}
	return u, nil
}

func (m *mockUserRepo) FindByID(_ context.Context, id string) (*model.User, error) {
	for _, u := range m.users {
		if u.ID.String() == id {
			return u, nil
		}
	}
	return nil, fmt.Errorf("user not found")
}

func (m *mockUserRepo) UpsertGoogleUser(_ context.Context, _ string, _ string, _ string) (*model.User, error) {
	return nil, fmt.Errorf("not implemented")
}

// testRouter returns a Gin engine with ErrorHandler middleware for tests.
func testRouter() *gin.Engine {
	r := gin.New()
	r.Use(apperror.ErrorHandler())
	return r
}

// defaultCookieCfg returns a CookieConfig suitable for tests (non-secure, no domain).
func defaultCookieCfg() handler.CookieConfig {
	return handler.CookieConfig{
		Secure:   false,
		Domain:   "",
		SameSite: http.SameSiteLaxMode,
	}
}

// hasCookie checks whether the response contains a Set-Cookie header for the given name.
func hasCookie(w *httptest.ResponseRecorder, name string) bool {
	for _, h := range w.Result().Cookies() {
		if h.Name == name {
			return true
		}
	}
	return false
}

// --- tests ---

func TestRegisterHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := newMockUserRepo()
	svc := service.NewAuthService(repo, "test-secret", 900000, 604800000)
	h := handler.NewAuthHandler(svc, nil, service.NewTokenDenylist(nil), 15*time.Minute, 7*24*time.Hour, defaultCookieCfg())

	router := testRouter()
	router.POST("/auth/register", h.Register)

	body, _ := json.Marshal(model.RegisterRequest{
		Email:    "alice@example.com",
		Password: "password123456", // 14 chars, meets min=12 requirement
		Name:     "Alice",
	})

	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp["email"] != "alice@example.com" {
		t.Errorf("expected email alice@example.com, got %v", resp["email"])
	}
	// tokens must be in cookies, not in body
	if _, ok := resp["accessToken"]; ok {
		t.Error("accessToken must not appear in response body")
	}
	if !hasCookie(w, "access_token") {
		t.Error("expected access_token cookie to be set")
	}
	if !hasCookie(w, "refresh_token") {
		t.Error("expected refresh_token cookie to be set")
	}
}

func TestRegisterHandler_InvalidEmail(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := newMockUserRepo()
	svc := service.NewAuthService(repo, "test-secret", 900000, 604800000)
	h := handler.NewAuthHandler(svc, nil, service.NewTokenDenylist(nil), 15*time.Minute, 7*24*time.Hour, defaultCookieCfg())

	router := testRouter()
	router.POST("/auth/register", h.Register)

	body, _ := json.Marshal(map[string]string{
		"email":    "not-an-email",
		"password": "password123456", // 14 chars, meets min=12 requirement
		"name":     "Bob",
	})

	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", w.Code, w.Body.String())
	}
}

// --- fake auth service for Google handler tests ---

type fakeAuthService struct {
	googleFn func(ctx context.Context, email, name, avatarURL string) (*model.AuthResponse, error)
}

func (f *fakeAuthService) Register(_ context.Context, _, _, _ string) (*model.AuthResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeAuthService) Login(_ context.Context, _, _ string) (*model.AuthResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeAuthService) Refresh(_ context.Context, _ string) (*model.AuthResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeAuthService) AuthenticateGoogleUser(ctx context.Context, email, name, avatarURL string) (*model.AuthResponse, error) {
	if f.googleFn != nil {
		return f.googleFn(ctx, email, name, avatarURL)
	}
	return nil, errors.New("not implemented")
}

// --- fake Google client ---

type fakeGoogleClient struct {
	info *google.UserInfo
	err  error
}

func (f *fakeGoogleClient) ExchangeCode(_ context.Context, _, _ string) (*google.UserInfo, error) {
	return f.info, f.err
}

// --- Google handler tests ---

func TestGoogleLogin_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &fakeAuthService{
		googleFn: func(ctx context.Context, email, name, avatarURL string) (*model.AuthResponse, error) {
			if email != "a@example.com" || name != "A" || avatarURL != "http://pic" {
				t.Errorf("svc args: %q %q %q", email, name, avatarURL)
			}
			return &model.AuthResponse{
				AccessToken:  "access",
				RefreshToken: "refresh",
				UserID:       "uid-1",
				Email:        email,
				Name:         name,
				AvatarURL:    avatarURL,
			}, nil
		},
	}
	gc := &fakeGoogleClient{info: &google.UserInfo{Email: "a@example.com", Name: "A", Picture: "http://pic"}}
	h := handler.NewAuthHandler(svc, gc, service.NewTokenDenylist(nil), 15*time.Minute, 7*24*time.Hour, defaultCookieCfg())

	router := testRouter()
	router.POST("/auth/google", h.GoogleLogin)

	body := strings.NewReader(`{"code":"abc","redirectUri":"http://localhost:3000/go/login"}`)
	req := httptest.NewRequest(http.MethodPost, "/auth/google", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["email"] != "a@example.com" {
		t.Errorf("unexpected email in response: %v", resp["email"])
	}
	// tokens must be in cookies, not in body
	if _, ok := resp["accessToken"]; ok {
		t.Error("accessToken must not appear in response body")
	}
	if !hasCookie(w, "access_token") {
		t.Error("expected access_token cookie to be set")
	}
}

func TestGoogleLogin_BadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := handler.NewAuthHandler(&fakeAuthService{}, &fakeGoogleClient{}, service.NewTokenDenylist(nil), 15*time.Minute, 7*24*time.Hour, defaultCookieCfg())
	router := testRouter()
	router.POST("/auth/google", h.GoogleLogin)

	req := httptest.NewRequest(http.MethodPost, "/auth/google", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestGoogleLogin_GoogleError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gc := &fakeGoogleClient{err: errors.New("bad code")}
	h := handler.NewAuthHandler(&fakeAuthService{}, gc, service.NewTokenDenylist(nil), 15*time.Minute, 7*24*time.Hour, defaultCookieCfg())
	router := testRouter()
	router.POST("/auth/google", h.GoogleLogin)

	body := strings.NewReader(`{"code":"abc","redirectUri":"http://localhost:3000/go/login"}`)
	req := httptest.NewRequest(http.MethodPost, "/auth/google", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestGoogleLogin_ServiceError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &fakeAuthService{
		googleFn: func(_ context.Context, _, _, _ string) (*model.AuthResponse, error) {
			return nil, errors.New("db down")
		},
	}
	gc := &fakeGoogleClient{info: &google.UserInfo{Email: "a@example.com"}}
	h := handler.NewAuthHandler(svc, gc, service.NewTokenDenylist(nil), 15*time.Minute, 7*24*time.Hour, defaultCookieCfg())
	router := testRouter()
	router.POST("/auth/google", h.GoogleLogin)

	body := strings.NewReader(`{"code":"abc","redirectUri":"http://localhost:3000/go/login"}`)
	req := httptest.NewRequest(http.MethodPost, "/auth/google", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

// --- Logout handler tests ---

func TestLogout_WithToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := handler.NewAuthHandler(&fakeAuthService{}, nil, service.NewTokenDenylist(nil), 15*time.Minute, 7*24*time.Hour, defaultCookieCfg())
	router := testRouter()
	router.POST("/auth/logout", h.Logout)

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer some.jwt.token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestLogout_NoToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := handler.NewAuthHandler(&fakeAuthService{}, nil, service.NewTokenDenylist(nil), 15*time.Minute, 7*24*time.Hour, defaultCookieCfg())
	router := testRouter()
	router.POST("/auth/logout", h.Logout)

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestRegister_SameSiteNone_SetsCookieCorrectly(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := newMockUserRepo()
	svc := service.NewAuthService(repo, "test-secret", 900000, 604800000)
	cfg := handler.CookieConfig{
		Secure:   true,
		Domain:   "example.com",
		SameSite: http.SameSiteNoneMode,
	}
	h := handler.NewAuthHandler(svc, nil, service.NewTokenDenylist(nil), 15*time.Minute, 7*24*time.Hour, cfg)

	router := testRouter()
	router.POST("/auth/register", h.Register)

	body, _ := json.Marshal(model.RegisterRequest{
		Email:    "samesite@example.com",
		Password: "password123456",
		Name:     "SameSite Test",
	})

	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	cookies := w.Result().Cookies()
	var accessCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "access_token" {
			accessCookie = c
			break
		}
	}
	if accessCookie == nil {
		t.Fatal("access_token cookie not found")
	}
	if accessCookie.Domain != "example.com" {
		t.Errorf("expected domain example.com, got %s", accessCookie.Domain)
	}
	if !accessCookie.Secure {
		t.Error("expected Secure flag to be true")
	}
	if accessCookie.SameSite != http.SameSiteNoneMode {
		t.Errorf("expected SameSite=None, got %v", accessCookie.SameSite)
	}

	var refreshCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "refresh_token" {
			refreshCookie = c
			break
		}
	}
	if refreshCookie == nil {
		t.Fatal("refresh_token cookie not found")
	}
	if refreshCookie.Domain != "example.com" {
		t.Errorf("expected refresh_token domain example.com, got %s", refreshCookie.Domain)
	}
	if !refreshCookie.Secure {
		t.Error("expected refresh_token Secure flag to be true")
	}
	if refreshCookie.SameSite != http.SameSiteNoneMode {
		t.Errorf("expected refresh_token SameSite=None, got %v", refreshCookie.SameSite)
	}
}

func TestRegister_SameSiteStrict_SetsCookieCorrectly(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := newMockUserRepo()
	svc := service.NewAuthService(repo, "test-secret", 900000, 604800000)
	cfg := handler.CookieConfig{
		Secure:   true,
		Domain:   "example.com",
		SameSite: http.SameSiteStrictMode,
	}
	h := handler.NewAuthHandler(svc, nil, service.NewTokenDenylist(nil), 15*time.Minute, 7*24*time.Hour, cfg)

	router := testRouter()
	router.POST("/auth/register", h.Register)

	body, _ := json.Marshal(model.RegisterRequest{
		Email:    "strict@example.com",
		Password: "password123456",
		Name:     "Strict Test",
	})

	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	cookies := w.Result().Cookies()
	var accessCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "access_token" {
			accessCookie = c
			break
		}
	}
	if accessCookie == nil {
		t.Fatal("access_token cookie not found")
	}
	if accessCookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("expected SameSite=Strict, got %v", accessCookie.SameSite)
	}
}
