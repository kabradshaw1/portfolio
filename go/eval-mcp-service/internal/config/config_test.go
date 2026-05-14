package config

import (
	"testing"
	"time"
)

func TestFromEnvDefaults(t *testing.T) {
	t.Setenv("EVAL_MCP_DB_PATH", "")
	t.Setenv("EVAL_API_URL", "")
	t.Setenv("EVAL_API_TOKEN", "")
	t.Setenv("EVAL_MCP_POLL_INTERVAL", "")
	t.Setenv("EVAL_MCP_WAIT_TIMEOUT", "")

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
	if cfg.PollInterval != time.Second {
		t.Fatalf("PollInterval = %s", cfg.PollInterval)
	}
	if cfg.WaitTimeout != 5*time.Minute {
		t.Fatalf("WaitTimeout = %s", cfg.WaitTimeout)
	}
}

func TestFromEnvOverrides(t *testing.T) {
	t.Setenv("EVAL_MCP_DB_PATH", "/tmp/eval.db")
	t.Setenv("EVAL_API_URL", "http://127.0.0.1:9000/eval")
	t.Setenv("EVAL_API_TOKEN", "token-123")
	t.Setenv("EVAL_MCP_POLL_INTERVAL", "250ms")
	t.Setenv("EVAL_MCP_WAIT_TIMEOUT", "30s")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv returned error: %v", err)
	}
	if cfg.DBPath != "/tmp/eval.db" || cfg.EvalAPIURL != "http://127.0.0.1:9000/eval" || cfg.APIToken != "token-123" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
	if cfg.PollInterval != 250*time.Millisecond || cfg.WaitTimeout != 30*time.Second {
		t.Fatalf("unexpected durations: %#v", cfg)
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("EVAL_MCP_POLL_INTERVAL", "")
			t.Setenv("EVAL_MCP_WAIT_TIMEOUT", "")
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("EVAL_MCP_POLL_INTERVAL", "")
			t.Setenv("EVAL_MCP_WAIT_TIMEOUT", "")
			t.Setenv(tt.key, "0s")

			_, err := FromEnv()
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
