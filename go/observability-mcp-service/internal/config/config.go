package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const (
	defaultPrometheusURL                  = "http://localhost:9090"
	defaultLokiURL                        = "http://localhost:3100"
	defaultJaegerURL                      = "http://localhost:16686"
	defaultGrafanaPrometheusDatasourceUID = "PBFA97CFB590B2093"
	defaultGrafanaLokiDatasourceUID       = "loki"
)

type Config struct {
	PrometheusURL                  string
	LokiURL                        string
	JaegerURL                      string
	GrafanaURL                     string
	GrafanaToken                   string
	GrafanaAccessClientID          string
	GrafanaAccessClientSecret      string
	GrafanaPrometheusDatasourceUID string
	GrafanaLokiDatasourceUID       string
	QueryTimeout                   time.Duration
	DefaultWindow                  time.Duration
	MaxWindow                      time.Duration
	MaxLogLines                    int
	MaxTraceSpans                  int
	HistoryEnabled                 bool
	HistoryDBPath                  string
	HistoryAutoCapture             bool
	ManagementActionsEnabled       bool
	ManagementAllowHighRisk        bool
	ManagementActionTimeout        time.Duration
	ManagementMaxOutputBytes       int
}

func FromEnv() (Config, error) {
	queryTimeout, err := durationEnv("OBS_QUERY_TIMEOUT", 5*time.Second)
	if err != nil {
		return Config{}, err
	}
	defaultWindow, err := durationEnv("OBS_DEFAULT_WINDOW", 15*time.Minute)
	if err != nil {
		return Config{}, err
	}
	maxWindow, err := durationEnv("OBS_MAX_WINDOW", time.Hour)
	if err != nil {
		return Config{}, err
	}
	maxLogLines, err := intEnv("OBS_MAX_LOG_LINES", 100)
	if err != nil {
		return Config{}, err
	}
	maxTraceSpans, err := intEnv("OBS_MAX_TRACE_SPANS", 100)
	if err != nil {
		return Config{}, err
	}
	historyEnabled, err := boolEnv("OBS_HISTORY_ENABLED", true)
	if err != nil {
		return Config{}, err
	}
	historyAutoCapture, err := boolEnv("OBS_HISTORY_AUTO_CAPTURE", false)
	if err != nil {
		return Config{}, err
	}
	managementActionsEnabled, err := boolEnv("OBS_MANAGEMENT_ACTIONS_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	managementAllowHighRisk, err := boolEnv("OBS_MANAGEMENT_ALLOW_HIGH_RISK", false)
	if err != nil {
		return Config{}, err
	}
	managementActionTimeout, err := durationEnv("OBS_MANAGEMENT_ACTION_TIMEOUT", 45*time.Minute)
	if err != nil {
		return Config{}, err
	}
	managementMaxOutputBytes, err := intEnv("OBS_MANAGEMENT_MAX_OUTPUT_BYTES", 32768)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		PrometheusURL:                  getenv("OBS_PROMETHEUS_URL", defaultPrometheusURL),
		LokiURL:                        getenv("OBS_LOKI_URL", defaultLokiURL),
		JaegerURL:                      getenv("OBS_JAEGER_URL", defaultJaegerURL),
		GrafanaURL:                     getenv("OBS_GRAFANA_URL", ""),
		GrafanaToken:                   os.Getenv("OBS_GRAFANA_TOKEN"),
		GrafanaAccessClientID:          os.Getenv("OBS_GRAFANA_ACCESS_CLIENT_ID"),
		GrafanaAccessClientSecret:      os.Getenv("OBS_GRAFANA_ACCESS_CLIENT_SECRET"),
		GrafanaPrometheusDatasourceUID: getenv("OBS_GRAFANA_PROMETHEUS_DS_UID", defaultGrafanaPrometheusDatasourceUID),
		GrafanaLokiDatasourceUID:       getenv("OBS_GRAFANA_LOKI_DS_UID", defaultGrafanaLokiDatasourceUID),
		QueryTimeout:                   queryTimeout,
		DefaultWindow:                  defaultWindow,
		MaxWindow:                      maxWindow,
		MaxLogLines:                    maxLogLines,
		MaxTraceSpans:                  maxTraceSpans,
		HistoryEnabled:                 historyEnabled,
		HistoryDBPath:                  getenv("OBS_HISTORY_DB_PATH", defaultHistoryDBPath()),
		HistoryAutoCapture:             historyAutoCapture,
		ManagementActionsEnabled:       managementActionsEnabled,
		ManagementAllowHighRisk:        managementAllowHighRisk,
		ManagementActionTimeout:        managementActionTimeout,
		ManagementMaxOutputBytes:       managementMaxOutputBytes,
	}
	if cfg.QueryTimeout <= 0 {
		return Config{}, fmt.Errorf("OBS_QUERY_TIMEOUT must be positive")
	}
	if cfg.DefaultWindow <= 0 {
		return Config{}, fmt.Errorf("OBS_DEFAULT_WINDOW must be positive")
	}
	if cfg.MaxWindow <= 0 {
		return Config{}, fmt.Errorf("OBS_MAX_WINDOW must be positive")
	}
	if cfg.DefaultWindow > cfg.MaxWindow {
		return Config{}, fmt.Errorf("OBS_DEFAULT_WINDOW must be less than or equal to OBS_MAX_WINDOW")
	}
	if cfg.MaxLogLines <= 0 {
		return Config{}, fmt.Errorf("OBS_MAX_LOG_LINES must be positive")
	}
	if cfg.MaxTraceSpans <= 0 {
		return Config{}, fmt.Errorf("OBS_MAX_TRACE_SPANS must be positive")
	}
	if cfg.ManagementActionTimeout <= 0 {
		return Config{}, fmt.Errorf("OBS_MANAGEMENT_ACTION_TIMEOUT must be positive")
	}
	if cfg.ManagementMaxOutputBytes <= 0 {
		return Config{}, fmt.Errorf("OBS_MANAGEMENT_MAX_OUTPUT_BYTES must be positive")
	}
	if (cfg.GrafanaAccessClientID == "") != (cfg.GrafanaAccessClientSecret == "") {
		return Config{}, fmt.Errorf("OBS_GRAFANA_ACCESS_CLIENT_ID and OBS_GRAFANA_ACCESS_CLIENT_SECRET must be set together")
	}
	return cfg, nil
}

func (c Config) UseGrafanaGateway() bool {
	return c.GrafanaURL != ""
}

func (c Config) UsesDefaultJaegerURL() bool {
	return c.JaegerURL == defaultJaegerURL
}

func (c Config) WindowOrDefault(raw string) (time.Duration, error) {
	if raw == "" {
		return c.DefaultWindow, nil
	}
	window, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("parse window: %w", err)
	}
	if window <= 0 {
		return 0, fmt.Errorf("window must be positive")
	}
	if window > c.MaxWindow {
		return 0, fmt.Errorf("window must be <= %s", c.MaxWindow)
	}
	return window, nil
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func defaultHistoryDBPath() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".codex", "data", "observability-mcp", "history.db")
	}
	return filepath.Join(".codex", "data", "observability-mcp", "history.db")
}

func boolEnv(key string, fallback bool) (bool, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s: %w", key, err)
	}
	return parsed, nil
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

func intEnv(key string, fallback int) (int, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return parsed, nil
}
