package config

import (
	"strings"
	"testing"
	"time"
)

func TestFromEnvDefaults(t *testing.T) {
	clearEnv(t)

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
	if cfg.UseGrafanaGateway() {
		t.Fatal("expected direct backend mode by default")
	}
	if cfg.GrafanaURL != "" || cfg.GrafanaToken != "" || cfg.GrafanaAccessClientID != "" || cfg.GrafanaAccessClientSecret != "" {
		t.Fatalf("expected empty Grafana config by default: %+v", cfg)
	}
	if cfg.GrafanaPrometheusDatasourceUID != "PBFA97CFB590B2093" {
		t.Fatalf("GrafanaPrometheusDatasourceUID = %q", cfg.GrafanaPrometheusDatasourceUID)
	}
	if cfg.GrafanaLokiDatasourceUID != "loki" {
		t.Fatalf("GrafanaLokiDatasourceUID = %q", cfg.GrafanaLokiDatasourceUID)
	}
	if !cfg.UsesDefaultJaegerURL() {
		t.Fatal("expected default Jaeger URL")
	}
}

func TestFromEnvOverrides(t *testing.T) {
	clearEnv(t)
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
	if cfg.UsesDefaultJaegerURL() {
		t.Fatal("expected Jaeger override to be detected")
	}
}

func TestFromEnvGrafanaMode(t *testing.T) {
	clearEnv(t)
	t.Setenv("OBS_GRAFANA_URL", "https://observability-api.kylebradshaw.dev")
	t.Setenv("OBS_GRAFANA_TOKEN", "grafana-token")
	t.Setenv("OBS_GRAFANA_ACCESS_CLIENT_ID", "cf-id")
	t.Setenv("OBS_GRAFANA_ACCESS_CLIENT_SECRET", "cf-secret")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error = %v", err)
	}
	if !cfg.UseGrafanaGateway() {
		t.Fatal("expected Grafana gateway mode")
	}
	if cfg.GrafanaURL != "https://observability-api.kylebradshaw.dev" {
		t.Fatalf("GrafanaURL = %q", cfg.GrafanaURL)
	}
	if cfg.GrafanaToken != "grafana-token" {
		t.Fatal("GrafanaToken not loaded")
	}
	if cfg.GrafanaAccessClientID != "cf-id" || cfg.GrafanaAccessClientSecret != "cf-secret" {
		t.Fatalf("Cloudflare token config not loaded: %+v", cfg)
	}
}

func TestFromEnvGrafanaDatasourceUIDOverrides(t *testing.T) {
	clearEnv(t)
	t.Setenv("OBS_GRAFANA_PROMETHEUS_DS_UID", "custom-prometheus")
	t.Setenv("OBS_GRAFANA_LOKI_DS_UID", "custom-loki")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error = %v", err)
	}
	if cfg.GrafanaPrometheusDatasourceUID != "custom-prometheus" {
		t.Fatalf("GrafanaPrometheusDatasourceUID = %q", cfg.GrafanaPrometheusDatasourceUID)
	}
	if cfg.GrafanaLokiDatasourceUID != "custom-loki" {
		t.Fatalf("GrafanaLokiDatasourceUID = %q", cfg.GrafanaLokiDatasourceUID)
	}
}

func TestFromEnvHistoryDefaults(t *testing.T) {
	clearEnv(t)
	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error = %v", err)
	}
	if !cfg.HistoryEnabled {
		t.Fatal("expected history enabled by default")
	}
	if cfg.HistoryAutoCapture {
		t.Fatal("expected auto capture disabled by default")
	}
	if !strings.HasSuffix(cfg.HistoryDBPath, "observability-mcp/history.db") {
		t.Fatalf("HistoryDBPath = %q", cfg.HistoryDBPath)
	}
}

func TestFromEnvHistoryOverrides(t *testing.T) {
	clearEnv(t)
	t.Setenv("OBS_HISTORY_ENABLED", "false")
	t.Setenv("OBS_HISTORY_AUTO_CAPTURE", "true")
	t.Setenv("OBS_HISTORY_DB_PATH", "/tmp/obs-history.db")
	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error = %v", err)
	}
	if cfg.HistoryEnabled {
		t.Fatal("expected history disabled")
	}
	if !cfg.HistoryAutoCapture {
		t.Fatal("expected auto capture enabled")
	}
	if cfg.HistoryDBPath != "/tmp/obs-history.db" {
		t.Fatalf("HistoryDBPath = %q", cfg.HistoryDBPath)
	}
}

func TestFromEnvRejectsInvalidHistoryBool(t *testing.T) {
	clearEnv(t)
	t.Setenv("OBS_HISTORY_ENABLED", "maybe")
	if _, err := FromEnv(); err == nil {
		t.Fatal("expected invalid bool error")
	}
}

func TestFromEnvRejectsPartialGrafanaAccessToken(t *testing.T) {
	clearEnv(t)
	t.Setenv("OBS_GRAFANA_URL", "https://observability-api.kylebradshaw.dev")
	t.Setenv("OBS_GRAFANA_ACCESS_CLIENT_ID", "cf-id")
	t.Setenv("OBS_GRAFANA_ACCESS_CLIENT_SECRET", "")

	if _, err := FromEnv(); err == nil {
		t.Fatal("expected partial Cloudflare access token error")
	}
}

func TestFromEnvRejectsInvalidValues(t *testing.T) {
	clearEnv(t)
	t.Setenv("OBS_QUERY_TIMEOUT", "nope")
	if _, err := FromEnv(); err == nil {
		t.Fatal("expected invalid duration error")
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
	t.Setenv("OBS_GRAFANA_PROMETHEUS_DS_UID", "")
	t.Setenv("OBS_GRAFANA_LOKI_DS_UID", "")
	t.Setenv("OBS_HISTORY_ENABLED", "")
	t.Setenv("OBS_HISTORY_AUTO_CAPTURE", "")
	t.Setenv("OBS_HISTORY_DB_PATH", "")
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
