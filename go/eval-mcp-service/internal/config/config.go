package config

import (
	"fmt"
	"os"
	"time"
)

const (
	defaultDBPath           = "data/eval-mcp.db"
	defaultEvalAPIURL       = "http://localhost:8000/eval"
	defaultAuthServiceURL   = "http://localhost:8091/auth"
	defaultTokenCachePath   = "data/eval-mcp-auth.json"
	defaultPollInterval     = time.Second
	defaultWaitTimeout      = 5 * time.Minute
	defaultTokenRefreshSkew = time.Minute
)

type Config struct {
	DBPath           string
	EvalAPIURL       string
	APIToken         string
	AuthServiceURL   string
	AuthEmail        string
	AuthPassword     string
	TokenCachePath   string
	PollInterval     time.Duration
	WaitTimeout      time.Duration
	TokenRefreshSkew time.Duration
}

func FromEnv() (Config, error) {
	pollInterval, err := durationEnv("EVAL_MCP_POLL_INTERVAL", defaultPollInterval)
	if err != nil {
		return Config{}, err
	}
	waitTimeout, err := durationEnv("EVAL_MCP_WAIT_TIMEOUT", defaultWaitTimeout)
	if err != nil {
		return Config{}, err
	}
	tokenRefreshSkew, err := durationEnv("EVAL_MCP_TOKEN_REFRESH_SKEW", defaultTokenRefreshSkew)
	if err != nil {
		return Config{}, err
	}
	if pollInterval <= 0 {
		return Config{}, fmt.Errorf("EVAL_MCP_POLL_INTERVAL must be positive")
	}
	if waitTimeout <= 0 {
		return Config{}, fmt.Errorf("EVAL_MCP_WAIT_TIMEOUT must be positive")
	}
	if tokenRefreshSkew <= 0 {
		return Config{}, fmt.Errorf("EVAL_MCP_TOKEN_REFRESH_SKEW must be positive")
	}

	apiToken := os.Getenv("EVAL_API_TOKEN")
	authEmail := os.Getenv("EVAL_MCP_AUTH_EMAIL")
	authPassword := os.Getenv("EVAL_MCP_AUTH_PASSWORD")
	if apiToken == "" {
		if authEmail == "" {
			return Config{}, fmt.Errorf("EVAL_MCP_AUTH_EMAIL is required when EVAL_API_TOKEN is empty")
		}
		if authPassword == "" {
			return Config{}, fmt.Errorf("EVAL_MCP_AUTH_PASSWORD is required when EVAL_API_TOKEN is empty")
		}
	}

	return Config{
		DBPath:           getenv("EVAL_MCP_DB_PATH", defaultDBPath),
		EvalAPIURL:       getenv("EVAL_API_URL", defaultEvalAPIURL),
		APIToken:         apiToken,
		AuthServiceURL:   getenv("AUTH_SERVICE_URL", defaultAuthServiceURL),
		AuthEmail:        authEmail,
		AuthPassword:     authPassword,
		TokenCachePath:   getenv("EVAL_MCP_TOKEN_CACHE_PATH", defaultTokenCachePath),
		PollInterval:     pollInterval,
		WaitTimeout:      waitTimeout,
		TokenRefreshSkew: tokenRefreshSkew,
	}, nil
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func durationEnv(key string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return parsed, nil
}
