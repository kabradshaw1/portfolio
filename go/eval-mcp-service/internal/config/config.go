package config

import (
	"fmt"
	"os"
	"time"
)

const (
	defaultEvalAPIURL   = "http://localhost:8000/eval"
	defaultPollInterval = time.Second
	defaultWaitTimeout  = 5 * time.Minute
)

type Config struct {
	EvalAPIURL   string
	APIToken     string
	PollInterval time.Duration
	WaitTimeout  time.Duration
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
	if pollInterval <= 0 {
		return Config{}, fmt.Errorf("EVAL_MCP_POLL_INTERVAL must be positive")
	}
	if waitTimeout <= 0 {
		return Config{}, fmt.Errorf("EVAL_MCP_WAIT_TIMEOUT must be positive")
	}
	return Config{
		EvalAPIURL:   getenv("EVAL_API_URL", defaultEvalAPIURL),
		APIToken:     os.Getenv("EVAL_API_TOKEN"),
		PollInterval: pollInterval,
		WaitTimeout:  waitTimeout,
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
