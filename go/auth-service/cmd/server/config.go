package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const (
	accessTokenTTLMs  = 900_000     // 15 minutes (900 seconds)
	refreshTokenTTLMs = 604_800_000 // 7 days (604800 seconds)
)

// Config holds all configuration values for the auth service.
type Config struct {
	DatabaseURL       string
	JWTSecret         string
	GoogleClientID    string
	GoogleClientSecret string
	GoogleTokenURL    string
	GoogleUserinfoURL string
	AllowedOrigins    string
	Port              string
	RedisURL          string
	CookieSecure      bool
	CookieDomain      string
	CookieSameSite    http.SameSite
	OTELEndpoint      string
}

// loadConfig reads environment variables and returns a Config.
// It fatals on missing required values.
func loadConfig() Config {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET is required")
	}
	googleClientID := os.Getenv("GOOGLE_CLIENT_ID")
	if googleClientID == "" {
		log.Fatal("GOOGLE_CLIENT_ID is required")
	}
	googleClientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
	if googleClientSecret == "" {
		log.Fatal("GOOGLE_CLIENT_SECRET is required")
	}

	googleTokenURL := os.Getenv("GOOGLE_TOKEN_URL")
	if googleTokenURL == "" {
		googleTokenURL = "https://oauth2.googleapis.com/token"
	}
	googleUserinfoURL := os.Getenv("GOOGLE_USERINFO_URL")
	if googleUserinfoURL == "" {
		googleUserinfoURL = "https://www.googleapis.com/oauth2/v3/userinfo"
	}
	allowedOrigins := os.Getenv("ALLOWED_ORIGINS")
	if allowedOrigins == "" {
		allowedOrigins = "http://localhost:3000"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8091"
	}

	cookieSameSite := http.SameSiteLaxMode
	switch strings.ToLower(os.Getenv("COOKIE_SAMESITE")) {
	case "none":
		cookieSameSite = http.SameSiteNoneMode
	case "strict":
		cookieSameSite = http.SameSiteStrictMode
	case "lax", "":
		// default already set
	default:
		slog.Warn("unrecognised COOKIE_SAMESITE value, defaulting to Lax",
			"value", os.Getenv("COOKIE_SAMESITE"))
	}

	return Config{
		DatabaseURL:        databaseURL,
		JWTSecret:          jwtSecret,
		GoogleClientID:     googleClientID,
		GoogleClientSecret: googleClientSecret,
		GoogleTokenURL:     googleTokenURL,
		GoogleUserinfoURL:  googleUserinfoURL,
		AllowedOrigins:     allowedOrigins,
		Port:               port,
		RedisURL:           os.Getenv("REDIS_URL"),
		CookieSecure:       os.Getenv("COOKIE_SECURE") == "true",
		CookieDomain:       os.Getenv("COOKIE_DOMAIN"),
		CookieSameSite:     cookieSameSite,
		OTELEndpoint:       os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
	}
}

// connectPostgres creates a tuned pgxpool connection pool.
func connectPostgres(ctx context.Context, databaseURL string) *pgxpool.Pool {
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		log.Fatalf("failed to parse database config: %v", err)
	}
	poolConfig.MaxConns = 10
	poolConfig.MinConns = 2
	poolConfig.MaxConnIdleTime = 5 * time.Minute
	poolConfig.MaxConnLifetime = 30 * time.Minute
	poolConfig.HealthCheckPeriod = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("failed to ping database: %v", err)
	}
	slog.Info("connected to database")
	return pool
}

// connectRedis optionally connects to Redis. Returns nil if URL is empty or connection fails.
func connectRedis(ctx context.Context, redisURL string) *redis.Client {
	if redisURL == "" {
		return nil
	}
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		slog.Warn("failed to parse REDIS_URL, token revocation disabled", "error", err)
		return nil
	}
	client := redis.NewClient(opts)
	if err := client.Ping(ctx).Err(); err != nil {
		slog.Warn("redis not available, token revocation disabled", "error", err)
		return nil
	}
	slog.Info("connected to redis")
	return client
}
