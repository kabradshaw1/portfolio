package service

import (
	"context"
	"testing"
)

func BenchmarkRegister(b *testing.B) {
	svc := NewAuthService(newMockRepo(), "test-secret-at-least-32-characters-long!!", 900000, 604800000)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = svc.Register(ctx, "bench@test.com", "password123", "Bench User")
	}
}

func BenchmarkLogin(b *testing.B) {
	repo := newMockRepo()
	svc := NewAuthService(repo, "test-secret-at-least-32-characters-long!!", 900000, 604800000)
	ctx := context.Background()
	_, _ = svc.Register(ctx, "bench@test.com", "password123", "Bench User")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = svc.Login(ctx, "bench@test.com", "password123")
	}
}
