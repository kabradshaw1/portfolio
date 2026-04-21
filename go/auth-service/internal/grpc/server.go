package grpc

import (
	"context"
	"errors"

	jwtlib "github.com/golang-jwt/jwt/v5"
	pb "github.com/kabradshaw1/portfolio/go/auth-service/pb/auth/v1"
)

// Denylist checks whether a raw token string has been revoked.
type Denylist interface {
	IsRevoked(ctx context.Context, token string) bool
}

// AuthGRPCServer implements auth.v1.AuthService.
type AuthGRPCServer struct {
	pb.UnimplementedAuthServiceServer
	jwtSecret []byte
	denylist  Denylist
}

// NewAuthGRPCServer creates an AuthGRPCServer.
func NewAuthGRPCServer(jwtSecret string, denylist Denylist) *AuthGRPCServer {
	return &AuthGRPCServer{
		jwtSecret: []byte(jwtSecret),
		denylist:  denylist,
	}
}

// CheckToken validates a JWT and checks the Redis denylist.
func (s *AuthGRPCServer) CheckToken(ctx context.Context, req *pb.CheckTokenRequest) (*pb.CheckTokenResponse, error) {
	claims := jwtlib.MapClaims{}
	_, err := jwtlib.ParseWithClaims(req.Token, claims, func(t *jwtlib.Token) (any, error) {
		if _, ok := t.Method.(*jwtlib.SigningMethodHMAC); !ok {
			return nil, jwtlib.ErrSignatureInvalid
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		reason := "malformed"
		if errors.Is(err, jwtlib.ErrTokenExpired) {
			reason = "expired"
		}
		return &pb.CheckTokenResponse{Valid: false, Reason: reason}, nil
	}

	sub, ok := claims["sub"].(string)
	if !ok || sub == "" {
		return &pb.CheckTokenResponse{Valid: false, Reason: "malformed"}, nil
	}

	if s.denylist.IsRevoked(ctx, req.Token) {
		return &pb.CheckTokenResponse{Valid: false, UserId: sub, Reason: "revoked"}, nil
	}

	return &pb.CheckTokenResponse{Valid: true, UserId: sub}, nil
}
