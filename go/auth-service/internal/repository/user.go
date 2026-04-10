package repository

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kabradshaw1/portfolio/go/auth-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
	gobreaker "github.com/sony/gobreaker/v2"
)

var ErrUserNotFound = apperror.NotFound("USER_NOT_FOUND", "user not found")
var ErrEmailExists = apperror.Conflict("EMAIL_EXISTS", "email already registered")

type UserRepository struct {
	pool    *pgxpool.Pool
	breaker *gobreaker.CircuitBreaker[any]
	retryCfg resilience.RetryConfig
}

func NewUserRepository(pool *pgxpool.Pool, breaker *gobreaker.CircuitBreaker[any]) *UserRepository {
	cfg := resilience.DefaultRetryConfig()
	cfg.IsRetryable = resilience.IsPgRetryable
	return &UserRepository{pool: pool, breaker: breaker, retryCfg: cfg}
}

func (r *UserRepository) Create(ctx context.Context, email, passwordHash, name string) (*model.User, error) {
	return resilience.Call(ctx, r.breaker, r.retryCfg, func(ctx context.Context) (*model.User, error) {
		user := &model.User{}
		err := r.pool.QueryRow(ctx,
			`INSERT INTO users (email, password_hash, name) VALUES ($1, $2, $3)
			 RETURNING id, email, password_hash, name, avatar_url, created_at`,
			email, passwordHash, name,
		).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Name, &user.AvatarURL, &user.CreatedAt)
		if err != nil {
			if strings.Contains(err.Error(), "duplicate key") {
				return nil, ErrEmailExists
			}
			return nil, err
		}
		return user, nil
	})
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*model.User, error) {
	return resilience.Call(ctx, r.breaker, r.retryCfg, func(ctx context.Context) (*model.User, error) {
		user := &model.User{}
		err := r.pool.QueryRow(ctx,
			`SELECT id, email, password_hash, name, avatar_url, created_at FROM users WHERE email = $1`,
			email,
		).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Name, &user.AvatarURL, &user.CreatedAt)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrUserNotFound
			}
			return nil, err
		}
		return user, nil
	})
}

func (r *UserRepository) FindByID(ctx context.Context, id string) (*model.User, error) {
	return resilience.Call(ctx, r.breaker, r.retryCfg, func(ctx context.Context) (*model.User, error) {
		user := &model.User{}
		err := r.pool.QueryRow(ctx,
			`SELECT id, email, password_hash, name, avatar_url, created_at FROM users WHERE id = $1`,
			id,
		).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Name, &user.AvatarURL, &user.CreatedAt)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrUserNotFound
			}
			return nil, err
		}
		return user, nil
	})
}

// UpsertGoogleUser creates a new Google-authenticated user, or updates an
// existing user's name and avatar. password_hash is never modified.
func (r *UserRepository) UpsertGoogleUser(ctx context.Context, email, name, avatarURL string) (*model.User, error) {
	return resilience.Call(ctx, r.breaker, r.retryCfg, func(ctx context.Context) (*model.User, error) {
		user := &model.User{}
		err := r.pool.QueryRow(ctx,
			`INSERT INTO users (email, name, avatar_url, password_hash)
			 VALUES ($1, $2, $3, NULL)
			 ON CONFLICT (email) DO UPDATE
			   SET name = EXCLUDED.name,
			       avatar_url = EXCLUDED.avatar_url
			 RETURNING id, email, password_hash, name, avatar_url, created_at`,
			email, name, avatarURL,
		).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Name, &user.AvatarURL, &user.CreatedAt)
		if err != nil {
			return nil, err
		}
		return user, nil
	})
}
