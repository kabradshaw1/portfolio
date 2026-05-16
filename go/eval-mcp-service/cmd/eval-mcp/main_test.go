package main

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRunWiresDependenciesAndCallsServer(t *testing.T) {
	apiServer := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(apiServer.Close)

	t.Setenv("EVAL_API_URL", apiServer.URL)
	t.Setenv("EVAL_API_TOKEN", "test-token")
	t.Setenv("EVAL_MCP_POLL_INTERVAL", "10ms")
	t.Setenv("EVAL_MCP_WAIT_TIMEOUT", "2s")

	var logs bytes.Buffer
	logger := log.New(&logs, "", 0)
	called := false
	err := run(context.Background(), logger, func(_ context.Context, application *app) error {
		called = true
		if application.service == nil {
			t.Fatal("expected service")
		}
		if application.cfg.EvalAPIURL != apiServer.URL {
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
	if !strings.Contains(logs.String(), "eval MCP server running on stdio") {
		t.Fatalf("expected startup log, got %q", logs.String())
	}
}

func TestRunAcceptsAuthServiceConfigWithoutStaticToken(t *testing.T) {
	apiServer := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(apiServer.Close)
	authServer := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(authServer.Close)

	t.Setenv("EVAL_API_URL", apiServer.URL)
	t.Setenv("EVAL_API_TOKEN", "")
	t.Setenv("AUTH_SERVICE_URL", authServer.URL)
	t.Setenv("EVAL_MCP_AUTH_EMAIL", "eval@example.test")
	t.Setenv("EVAL_MCP_AUTH_PASSWORD", "secret")
	t.Setenv("EVAL_MCP_TOKEN_CACHE_PATH", t.TempDir()+"/tokens.json")
	t.Setenv("EVAL_MCP_POLL_INTERVAL", "10ms")
	t.Setenv("EVAL_MCP_WAIT_TIMEOUT", "2s")

	called := false
	err := run(context.Background(), log.New(&bytes.Buffer{}, "", 0), func(_ context.Context, application *app) error {
		called = true
		if application.service == nil {
			t.Fatal("expected service")
		}
		if application.cfg.APIToken != "" {
			t.Fatalf("APIToken = %q", application.cfg.APIToken)
		}
		if application.cfg.AuthServiceURL != authServer.URL {
			t.Fatalf("AuthServiceURL = %q", application.cfg.AuthServiceURL)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if !called {
		t.Fatal("expected runner to be called")
	}
}

func TestRunReturnsConfigError(t *testing.T) {
	t.Setenv("EVAL_MCP_POLL_INTERVAL", "nope")

	called := false
	err := run(context.Background(), log.New(&bytes.Buffer{}, "", 0), func(context.Context, *app) error {
		called = true
		return nil
	})
	if err == nil {
		t.Fatal("expected config error")
	}
	if !strings.Contains(err.Error(), "config:") {
		t.Fatalf("expected config-wrapped error, got %v", err)
	}
	if called {
		t.Fatal("runner should not be called")
	}
}
