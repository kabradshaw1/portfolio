package config

import (
	"testing"
	"time"
)

func TestFromEnvDefaults(t *testing.T) {
	t.Setenv("OBS_PROMETHEUS_URL", "")
	t.Setenv("OBS_LOKI_URL", "")
	t.Setenv("OBS_JAEGER_URL", "")
	t.Setenv("OBS_QUERY_TIMEOUT", "")
	t.Setenv("OBS_DEFAULT_WINDOW", "")
	t.Setenv("OBS_MAX_WINDOW", "")
	t.Setenv("OBS_MAX_LOG_LINES", "")
	t.Setenv("OBS_MAX_TRACE_SPANS", "")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error = %v", err)
	}
	if cfg.PrometheusURL != "http://localhost:9090" {
		t.Fatalf("PrometheusURL = %q", cfg.PrometheusURL)
	}
	if cfg.LokiURL != "http://localhost:3100" {
		t.Fatalf("LokiURL = %q", cfg.LokiURL)
	}
	if cfg.JaegerURL != "http://localhost:16686" {
		t.Fatalf("JaegerURL = %q", cfg.JaegerURL)
	}
	if cfg.QueryTimeout != 5*time.Second {
		t.Fatalf("QueryTimeout = %s", cfg.QueryTimeout)
	}
	if cfg.DefaultWindow != 15*time.Minute {
		t.Fatalf("DefaultWindow = %s", cfg.DefaultWindow)
	}
	if cfg.MaxWindow != time.Hour {
		t.Fatalf("MaxWindow = %s", cfg.MaxWindow)
	}
	if cfg.MaxLogLines != 100 {
		t.Fatalf("MaxLogLines = %d", cfg.MaxLogLines)
	}
	if cfg.MaxTraceSpans != 100 {
		t.Fatalf("MaxTraceSpans = %d", cfg.MaxTraceSpans)
	}
}

func TestFromEnvOverrides(t *testing.T) {
	t.Setenv("OBS_PROMETHEUS_URL", "http://prometheus.monitoring.svc:9090")
	t.Setenv("OBS_LOKI_URL", "http://loki.monitoring.svc:3100")
	t.Setenv("OBS_JAEGER_URL", "http://jaeger.monitoring.svc:16686")
	t.Setenv("OBS_QUERY_TIMEOUT", "2s")
	t.Setenv("OBS_DEFAULT_WINDOW", "10m")
	t.Setenv("OBS_MAX_WINDOW", "45m")
	t.Setenv("OBS_MAX_LOG_LINES", "25")
	t.Setenv("OBS_MAX_TRACE_SPANS", "50")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error = %v", err)
	}
	if cfg.QueryTimeout != 2*time.Second || cfg.DefaultWindow != 10*time.Minute || cfg.MaxWindow != 45*time.Minute {
		t.Fatalf("duration overrides not applied: %+v", cfg)
	}
	if cfg.MaxLogLines != 25 || cfg.MaxTraceSpans != 50 {
		t.Fatalf("limit overrides not applied: %+v", cfg)
	}
}

func TestFromEnvRejectsInvalidValues(t *testing.T) {
	t.Setenv("OBS_QUERY_TIMEOUT", "nope")
	if _, err := FromEnv(); err == nil {
		t.Fatal("expected invalid duration error")
	}
}

func TestValidateWindow(t *testing.T) {
	cfg := Config{DefaultWindow: 15 * time.Minute, MaxWindow: time.Hour}
	got, err := cfg.WindowOrDefault("")
	if err != nil {
		t.Fatalf("WindowOrDefault(empty) error = %v", err)
	}
	if got != 15*time.Minute {
		t.Fatalf("default window = %s", got)
	}
	if _, err := cfg.WindowOrDefault("2h"); err == nil {
		t.Fatal("expected max window error")
	}
	if _, err := cfg.WindowOrDefault("0s"); err == nil {
		t.Fatal("expected positive window error")
	}
}
