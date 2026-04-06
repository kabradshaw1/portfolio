package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kabradshaw1/portfolio/go/auth-service/internal/model"
	"golang.org/x/crypto/bcrypt"
)

// --- mock repo ---

type mockUserRepo struct {
	users map[string]*model.User // keyed by email
}

func newMockRepo() *mockUserRepo {
	return &mockUserRepo{users: make(map[string]*model.User)}
}

func (m *mockUserRepo) Create(_ context.Context, email, passwordHash, name string) (*model.User, error) {
	if _, exists := m.users[email]; exists {
		return nil, fmt.Errorf("email already registered")
	}
	u := &model.User{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: passwordHash,
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

// --- tests ---

func TestRegister(t *testing.T) {
	repo := newMockRepo()
	svc := NewAuthService(repo, "test-secret", 900000, 604800000)

	resp, err := svc.Register(context.Background(), "alice@example.com", "password123", "Alice")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if resp.Email != "alice@example.com" {
		t.Errorf("expected email alice@example.com, got %s", resp.Email)
	}
	if resp.Name != "Alice" {
		t.Errorf("expected name Alice, got %s", resp.Name)
	}
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Error("expected non-empty tokens")
	}
}

func TestLogin(t *testing.T) {
	repo := newMockRepo()
	svc := NewAuthService(repo, "test-secret", 900000, 604800000)

	_, err := svc.Register(context.Background(), "bob@example.com", "secret99", "Bob")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	resp, err := svc.Login(context.Background(), "bob@example.com", "secret99")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if resp.Email != "bob@example.com" {
		t.Errorf("expected email bob@example.com, got %s", resp.Email)
	}
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Error("expected non-empty tokens")
	}
}

func TestLoginWrongPassword(t *testing.T) {
	repo := newMockRepo()
	svc := NewAuthService(repo, "test-secret", 900000, 604800000)

	_, err := svc.Register(context.Background(), "carol@example.com", "correct-pw", "Carol")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	_, err = svc.Login(context.Background(), "carol@example.com", "wrong-pw")
	if err == nil {
		t.Fatal("expected login to fail with wrong password")
	}
}

func TestRefresh(t *testing.T) {
	repo := newMockRepo()
	svc := NewAuthService(repo, "test-secret", 900000, 604800000)

	reg, err := svc.Register(context.Background(), "dave@example.com", "mypass", "Dave")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	resp, err := svc.Refresh(context.Background(), reg.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}
	if resp.Email != "dave@example.com" {
		t.Errorf("expected email dave@example.com, got %s", resp.Email)
	}
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Error("expected non-empty tokens")
	}
}

// ensure bcrypt import is used (compile guard)
var _ = bcrypt.MinCost
