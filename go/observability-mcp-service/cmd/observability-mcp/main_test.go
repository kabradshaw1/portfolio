package main

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRunWiresConfigClientsAndRunner(t *testing.T) {
	clearEnv(t)
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
	clearEnv(t)
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

func TestRunUsesGrafanaGatewayMode(t *testing.T) {
	clearEnv(t)

	var mu sync.Mutex
	seen := map[string]bool{}
	var handlerErrors []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			mu.Lock()
			handlerErrors = append(handlerErrors, "Authorization = "+got)
			mu.Unlock()
			http.Error(w, "bad authorization", http.StatusUnauthorized)
			return
		}
		if got := r.Header.Get("CF-Access-Client-Id"); got != "cf-id" {
			mu.Lock()
			handlerErrors = append(handlerErrors, "CF-Access-Client-Id = "+got)
			mu.Unlock()
			http.Error(w, "bad access client id", http.StatusUnauthorized)
			return
		}
		if got := r.Header.Get("CF-Access-Client-Secret"); got != "cf-secret" {
			mu.Lock()
			handlerErrors = append(handlerErrors, "CF-Access-Client-Secret = "+got)
			mu.Unlock()
			http.Error(w, "bad access client secret", http.StatusUnauthorized)
			return
		}

		mu.Lock()
		seen[r.URL.Path] = true
		mu.Unlock()

		switch {
		case strings.HasSuffix(r.URL.Path, "/api/v1/query"):
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{"job":"prometheus"},"value":[1710000000,"0"]}]}}`))
		case strings.HasSuffix(r.URL.Path, "/loki/api/v1/query_range"):
			_, _ = w.Write([]byte(`{"status":"success","data":{"result":[{"stream":{"service":"go-order-service"},"values":[["1710000000000000000","order error"]]}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	t.Setenv("OBS_GRAFANA_URL", server.URL)
	t.Setenv("OBS_GRAFANA_TOKEN", "token")
	t.Setenv("OBS_GRAFANA_ACCESS_CLIENT_ID", "cf-id")
	t.Setenv("OBS_GRAFANA_ACCESS_CLIENT_SECRET", "cf-secret")
	var got *app
	err := run(context.Background(), log.New(io.Discard, "", 0), func(_ context.Context, application *app) error {
		got = application
		bundle := application.service.GetServiceEvidence(context.Background(), "go-order-service", time.Minute, "")
		if len(bundle.Errors) > 0 {
			t.Fatalf("service evidence errors = %+v", bundle.Errors)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if got == nil || got.service == nil {
		t.Fatal("expected app service")
	}
	if !got.cfg.UseGrafanaGateway() {
		t.Fatal("expected Grafana gateway mode")
	}

	prometheusPath := "/api/datasources/proxy/uid/PBFA97CFB590B2093/api/v1/query"
	lokiPath := "/api/datasources/proxy/uid/loki/loki/api/v1/query_range"
	mu.Lock()
	defer mu.Unlock()
	if len(handlerErrors) > 0 {
		t.Fatalf("handler errors = %+v", handlerErrors)
	}
	if !seen[prometheusPath] {
		t.Fatalf("expected Prometheus datasource proxy request; saw paths %+v", seen)
	}
	if !seen[lokiPath] {
		t.Fatalf("expected Loki datasource proxy request; saw paths %+v", seen)
	}
}

func clearEnv(t *testing.T) {
	t.Helper()
	t.Setenv("OBS_PROMETHEUS_URL", "")
	t.Setenv("OBS_LOKI_URL", "")
	t.Setenv("OBS_JAEGER_URL", "")
	t.Setenv("OBS_QUERY_TIMEOUT", "")
	t.Setenv("OBS_DEFAULT_WINDOW", "")
	t.Setenv("OBS_MAX_WINDOW", "")
	t.Setenv("OBS_MAX_LOG_LINES", "")
	t.Setenv("OBS_MAX_TRACE_SPANS", "")
	t.Setenv("OBS_GRAFANA_URL", "")
	t.Setenv("OBS_GRAFANA_TOKEN", "")
	t.Setenv("OBS_GRAFANA_ACCESS_CLIENT_ID", "")
	t.Setenv("OBS_GRAFANA_ACCESS_CLIENT_SECRET", "")
}
