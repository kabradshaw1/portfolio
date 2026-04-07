package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kabradshaw1/portfolio/go/auth-service/internal/handler"
	"github.com/kabradshaw1/portfolio/go/auth-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/auth-service/internal/service"
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

// --- tests ---

func TestRegisterHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := newMockUserRepo()
	svc := service.NewAuthService(repo, "test-secret", 900000, 604800000)
	h := handler.NewAuthHandler(svc)

	router := gin.New()
	router.POST("/auth/register", h.Register)

	body, _ := json.Marshal(model.RegisterRequest{
		Email:    "alice@example.com",
		Password: "password123",
		Name:     "Alice",
	})

	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp model.AuthResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Error("expected non-empty tokens")
	}
	if resp.Email != "alice@example.com" {
		t.Errorf("expected email alice@example.com, got %s", resp.Email)
	}
}

func TestRegisterHandler_InvalidEmail(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := newMockUserRepo()
	svc := service.NewAuthService(repo, "test-secret", 900000, 604800000)
	h := handler.NewAuthHandler(svc)

	router := gin.New()
	router.POST("/auth/register", h.Register)

	body, _ := json.Marshal(map[string]string{
		"email":    "not-an-email",
		"password": "password123",
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
