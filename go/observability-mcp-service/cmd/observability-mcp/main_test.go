package main

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"
)

func TestRunWiresConfigClientsAndRunner(t *testing.T) {
	t.Setenv("OBS_PROMETHEUS_URL", "")
	t.Setenv("OBS_LOKI_URL", "")
	t.Setenv("OBS_JAEGER_URL", "")
	t.Setenv("OBS_QUERY_TIMEOUT", "2s")
	t.Setenv("OBS_DEFAULT_WINDOW", "10m")
	t.Setenv("OBS_MAX_WINDOW", "30m")
	t.Setenv("OBS_MAX_LOG_LINES", "25")
	t.Setenv("OBS_MAX_TRACE_SPANS", "50")

	var logs bytes.Buffer
	logger := log.New(&logs, "", 0)
	called := false
	err := run(context.Background(), logger, func(_ context.Context, application *app) error {
		called = true
		if application.service == nil {
			t.Fatal("expected service")
		}
		if application.cfg.MaxLogLines != 25 || application.cfg.MaxTraceSpans != 50 {
			t.Fatalf("config not passed through: %+v", application.cfg)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if !called {
		t.Fatal("expected runner to be called")
	}
	if !strings.Contains(logs.String(), "observability MCP server running on stdio") {
		t.Fatalf("expected startup log, got %q", logs.String())
	}
}

func TestRunRejectsInvalidConfigBeforeServerStartup(t *testing.T) {
	t.Setenv("OBS_QUERY_TIMEOUT", "nope")
	called := false
	err := run(context.Background(), log.New(&bytes.Buffer{}, "", 0), func(context.Context, *app) error {
		called = true
		return nil
	})
	if err == nil {
		t.Fatal("expected config error")
	}
	if called {
		t.Fatal("runner should not be called")
	}
}
