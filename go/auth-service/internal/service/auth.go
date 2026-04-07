package service

import (
	"context"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kabradshaw1/portfolio/go/auth-service/internal/model"
	"golang.org/x/crypto/bcrypt"
)

// UserRepo abstracts user persistence so the service is testable.
type UserRepo interface {
	Create(ctx context.Context, email, passwordHash, name string) (*model.User, error)
	FindByEmail(ctx context.Context, email string) (*model.User, error)
	FindByID(ctx context.Context, id string) (*model.User, error)
	UpsertGoogleUser(ctx context.Context, email, name, avatarURL string) (*model.User, error)
}

// AuthService handles registration, login, and token refresh.
type AuthService struct {
	repo            UserRepo
	jwtSecret       []byte
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
}

// NewAuthService creates an AuthService. TTL values are in milliseconds.
func NewAuthService(repo UserRepo, jwtSecret string, accessTTLMs, refreshTTLMs int64) *AuthService {
	return &AuthService{
		repo:            repo,
		jwtSecret:       []byte(jwtSecret),
		accessTokenTTL:  time.Duration(accessTTLMs) * time.Millisecond,
		refreshTokenTTL: time.Duration(refreshTTLMs) * time.Millisecond,
	}
}

// Register hashes the password, creates the user, and returns JWT tokens.
func (s *AuthService) Register(ctx context.Context, email, password, name string) (*model.AuthResponse, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user, err := s.repo.Create(ctx, email, string(hash), name)
	if err != nil {
		return nil, err
	}

	return s.generateTokens(user)
}

// Login verifies credentials and returns JWT tokens.
func (s *AuthService) Login(ctx context.Context, email, password string) (*model.AuthResponse, error) {
	user, err := s.repo.FindByEmail(ctx, email)
	if err != nil {
		return nil, err
	}

	if user.PasswordHash == nil {
		return nil, errors.New("invalid credentials")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(password)); err != nil {
		return nil, errors.New("invalid credentials")
	}

	return s.generateTokens(user)
}

// Refresh parses the refresh token, looks up the user, and issues new tokens.
func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (*model.AuthResponse, error) {
	token, err := jwt.Parse(refreshToken, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}

	sub, err := claims.GetSubject()
	if err != nil {
		return nil, err
	}

	user, err := s.repo.FindByID(ctx, sub)
	if err != nil {
		return nil, err
	}

	return s.generateTokens(user)
}

// AuthenticateGoogleUser upserts a Google-authenticated user and issues tokens.
func (s *AuthService) AuthenticateGoogleUser(ctx context.Context, email, name, avatarURL string) (*model.AuthResponse, error) {
	user, err := s.repo.UpsertGoogleUser(ctx, email, name, avatarURL)
	if err != nil {
		return nil, err
	}
	return s.generateTokens(user)
}

// generateTokens creates an access token and a refresh token for the user.
func (s *AuthService) generateTokens(user *model.User) (*model.AuthResponse, error) {
	now := time.Now()

	accessClaims := jwt.MapClaims{
		"sub":   user.ID.String(),
		"email": user.Email,
		"iat":   now.Unix(),
		"exp":   now.Add(s.accessTokenTTL).Unix(),
	}
	accessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims).SignedString(s.jwtSecret)
	if err != nil {
		return nil, err
	}

	refreshClaims := jwt.MapClaims{
		"sub":   user.ID.String(),
		"email": user.Email,
		"iat":   now.Unix(),
		"exp":   now.Add(s.refreshTokenTTL).Unix(),
	}
	refreshToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims).SignedString(s.jwtSecret)
	if err != nil {
		return nil, err
	}

	avatar := ""
	if user.AvatarURL != nil {
		avatar = *user.AvatarURL
	}

	return &model.AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		UserID:       user.ID.String(),
		Email:        user.Email,
		Name:         user.Name,
		AvatarURL:    avatar,
	}, nil
}
