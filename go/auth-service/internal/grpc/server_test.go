package grpc

import (
	"context"
	"testing"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
	pb "github.com/kabradshaw1/portfolio/go/auth-service/pb/auth/v1"
)

type fakeDenylist struct {
	revoked map[string]bool
}

func (f *fakeDenylist) IsRevoked(_ context.Context, token string) bool {
	return f.revoked[token]
}

func TestCheckToken_Valid(t *testing.T) {
	secret := "test-secret"
	token := createTestToken(t, secret, "user-123", false)
	srv := NewAuthGRPCServer(secret, &fakeDenylist{revoked: map[string]bool{}})

	resp, err := srv.CheckToken(context.Background(), &pb.CheckTokenRequest{Token: token})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Valid {
		t.Fatalf("expected valid=true, got false, reason=%s", resp.Reason)
	}
	if resp.UserId != "user-123" {
		t.Fatalf("expected user-123, got %s", resp.UserId)
	}
}

func TestCheckToken_Expired(t *testing.T) {
	secret := "test-secret"
	token := createTestToken(t, secret, "user-123", true)
	srv := NewAuthGRPCServer(secret, &fakeDenylist{revoked: map[string]bool{}})

	resp, err := srv.CheckToken(context.Background(), &pb.CheckTokenRequest{Token: token})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Valid {
		t.Fatal("expected valid=false for expired token")
	}
	if resp.Reason != "expired" {
		t.Fatalf("expected reason=expired, got %s", resp.Reason)
	}
}

func TestCheckToken_Revoked(t *testing.T) {
	secret := "test-secret"
	token := createTestToken(t, secret, "user-456", false)
	srv := NewAuthGRPCServer(secret, &fakeDenylist{revoked: map[string]bool{token: true}})

	resp, err := srv.CheckToken(context.Background(), &pb.CheckTokenRequest{Token: token})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Valid {
		t.Fatal("expected valid=false for revoked token")
	}
	if resp.Reason != "revoked" {
		t.Fatalf("expected reason=revoked, got %s", resp.Reason)
	}
}

func TestCheckToken_Malformed(t *testing.T) {
	srv := NewAuthGRPCServer("test-secret", &fakeDenylist{revoked: map[string]bool{}})

	resp, err := srv.CheckToken(context.Background(), &pb.CheckTokenRequest{Token: "not-a-jwt"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Valid {
		t.Fatal("expected valid=false for malformed token")
	}
	if resp.Reason != "malformed" {
		t.Fatalf("expected reason=malformed, got %s", resp.Reason)
	}
}

func TestCheckToken_WrongSecret(t *testing.T) {
	token := createTestToken(t, "secret-A", "user-789", false)
	srv := NewAuthGRPCServer("secret-B", &fakeDenylist{revoked: map[string]bool{}})

	resp, err := srv.CheckToken(context.Background(), &pb.CheckTokenRequest{Token: token})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Valid {
		t.Fatal("expected valid=false for wrong-secret token")
	}
	if resp.Reason != "malformed" {
		t.Fatalf("expected reason=malformed, got %s", resp.Reason)
	}
}

func createTestToken(t *testing.T, secret, userID string, expired bool) string {
	t.Helper()
	claims := jwtlib.MapClaims{
		"sub": userID,
		"iat": time.Now().Unix(),
	}
	if expired {
		claims["exp"] = time.Now().Add(-1 * time.Hour).Unix()
	} else {
		claims["exp"] = time.Now().Add(1 * time.Hour).Unix()
	}
	token := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}
