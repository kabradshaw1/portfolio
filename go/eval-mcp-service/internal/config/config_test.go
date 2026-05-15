package config

import (
	"testing"
	"time"
)

func TestFromEnvDefaults(t *testing.T) {
	t.Setenv("EVAL_MCP_DB_PATH", "")
	t.Setenv("EVAL_API_URL", "")
	t.Setenv("EVAL_API_TOKEN", "")
	t.Setenv("AUTH_SERVICE_URL", "")
	t.Setenv("EVAL_MCP_AUTH_EMAIL", "user@example.test")
	t.Setenv("EVAL_MCP_AUTH_PASSWORD", "secret")
	t.Setenv("EVAL_MCP_TOKEN_CACHE_PATH", "")
	t.Setenv("EVAL_MCP_POLL_INTERVAL", "")
	t.Setenv("EVAL_MCP_WAIT_TIMEOUT", "")
	t.Setenv("EVAL_MCP_TOKEN_REFRESH_SKEW", "")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv returned error: %v", err)
	}
	if cfg.DBPath != "data/eval-mcp.db" {
		t.Fatalf("DBPath = %q", cfg.DBPath)
	}
	if cfg.EvalAPIURL != "http://localhost:8000/eval" {
		t.Fatalf("EvalAPIURL = %q", cfg.EvalAPIURL)
	}
	if cfg.APIToken != "" {
		t.Fatalf("APIToken = %q", cfg.APIToken)
	}
	if cfg.AuthServiceURL != "http://localhost:8091/auth" {
		t.Fatalf("AuthServiceURL = %q", cfg.AuthServiceURL)
	}
	if cfg.AuthEmail != "user@example.test" {
		t.Fatalf("AuthEmail = %q", cfg.AuthEmail)
	}
	if cfg.AuthPassword != "secret" {
		t.Fatalf("AuthPassword = %q", cfg.AuthPassword)
	}
	if cfg.TokenCachePath != "data/eval-mcp-auth.json" {
		t.Fatalf("TokenCachePath = %q", cfg.TokenCachePath)
	}
	if cfg.PollInterval != time.Second {
		t.Fatalf("PollInterval = %s", cfg.PollInterval)
	}
	if cfg.WaitTimeout != 5*time.Minute {
		t.Fatalf("WaitTimeout = %s", cfg.WaitTimeout)
	}
	if cfg.TokenRefreshSkew != time.Minute {
		t.Fatalf("TokenRefreshSkew = %s", cfg.TokenRefreshSkew)
	}
}

func TestFromEnvOverrides(t *testing.T) {
	t.Setenv("EVAL_MCP_DB_PATH", "/tmp/eval.db")
	t.Setenv("EVAL_API_URL", "http://127.0.0.1:9000/eval")
	t.Setenv("EVAL_API_TOKEN", "token-123")
	t.Setenv("AUTH_SERVICE_URL", "http://127.0.0.1:8091/auth")
	t.Setenv("EVAL_MCP_AUTH_EMAIL", "user@example.test")
	t.Setenv("EVAL_MCP_AUTH_PASSWORD", "secret")
	t.Setenv("EVAL_MCP_TOKEN_CACHE_PATH", "/tmp/tokens.json")
	t.Setenv("EVAL_MCP_POLL_INTERVAL", "250ms")
	t.Setenv("EVAL_MCP_WAIT_TIMEOUT", "30s")
	t.Setenv("EVAL_MCP_TOKEN_REFRESH_SKEW", "2m")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv returned error: %v", err)
	}
	if cfg.DBPath != "/tmp/eval.db" || cfg.EvalAPIURL != "http://127.0.0.1:9000/eval" || cfg.APIToken != "token-123" || cfg.AuthServiceURL != "http://127.0.0.1:8091/auth" || cfg.AuthEmail != "user@example.test" || cfg.AuthPassword != "secret" || cfg.TokenCachePath != "/tmp/tokens.json" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
	if cfg.PollInterval != 250*time.Millisecond || cfg.WaitTimeout != 30*time.Second || cfg.TokenRefreshSkew != 2*time.Minute {
		t.Fatalf("unexpected durations: %#v", cfg)
	}
}

func TestFromEnvStaticTokenBypassesAuthCredentials(t *testing.T) {
	t.Setenv("EVAL_API_TOKEN", "token-123")
	t.Setenv("EVAL_MCP_AUTH_EMAIL", "")
	t.Setenv("EVAL_MCP_AUTH_PASSWORD", "")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv returned error: %v", err)
	}
	if cfg.APIToken != "token-123" {
		t.Fatalf("APIToken = %q", cfg.APIToken)
	}
	if cfg.AuthEmail != "" || cfg.AuthPassword != "" {
		t.Fatalf("unexpected auth credentials: %#v", cfg)
	}
}

func TestFromEnvRequiresAuthConfigWithoutStaticToken(t *testing.T) {
	tests := []struct {
		name     string
		email    string
		password string
	}{
		{
			name:     "missing email",
			password: "secret",
		},
		{
			name:  "missing password",
			email: "user@example.test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("EVAL_API_TOKEN", "")
			t.Setenv("EVAL_MCP_AUTH_EMAIL", tt.email)
			t.Setenv("EVAL_MCP_AUTH_PASSWORD", tt.password)

			_, err := FromEnv()
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestFromEnvRejectsBadDuration(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{
			name: "poll interval",
			key:  "EVAL_MCP_POLL_INTERVAL",
		},
		{
			name: "wait timeout",
			key:  "EVAL_MCP_WAIT_TIMEOUT",
		},
		{
			name: "token refresh skew",
			key:  "EVAL_MCP_TOKEN_REFRESH_SKEW",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("EVAL_API_TOKEN", "token-123")
			t.Setenv("EVAL_MCP_POLL_INTERVAL", "")
			t.Setenv("EVAL_MCP_WAIT_TIMEOUT", "")
			t.Setenv("EVAL_MCP_TOKEN_REFRESH_SKEW", "")
			t.Setenv(tt.key, "not-a-duration")

			_, err := FromEnv()
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestFromEnvRejectsNonPositiveDurations(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{
			name: "poll interval",
			key:  "EVAL_MCP_POLL_INTERVAL",
		},
		{
			name: "wait timeout",
			key:  "EVAL_MCP_WAIT_TIMEOUT",
		},
		{
			name: "token refresh skew",
			key:  "EVAL_MCP_TOKEN_REFRESH_SKEW",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("EVAL_API_TOKEN", "token-123")
			t.Setenv("EVAL_MCP_POLL_INTERVAL", "")
			t.Setenv("EVAL_MCP_WAIT_TIMEOUT", "")
			t.Setenv("EVAL_MCP_TOKEN_REFRESH_SKEW", "")
			t.Setenv(tt.key, "0s")

			_, err := FromEnv()
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
