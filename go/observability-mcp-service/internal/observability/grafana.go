package observability

import (
	"context"
	"net/http"
	"strings"
	"time"
)

const (
	grafanaPrometheusUID = "PBFA97CFB590B2093"
	grafanaLokiUID       = "loki"
)

type GrafanaConfig struct {
	BaseURL            string
	Token              string
	AccessClientID     string
	AccessClientSecret string
}

type GrafanaClient struct {
	prometheus *PrometheusClient
	loki       *LokiClient
	headers    map[string]string
}

func NewGrafana(cfg GrafanaConfig, httpClient *http.Client) *GrafanaClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	headers := make(map[string]string)
	if cfg.Token != "" {
		headers["Authorization"] = "Bearer " + cfg.Token
	}
	if cfg.AccessClientID != "" {
		headers["CF-Access-Client-Id"] = cfg.AccessClientID
		headers["CF-Access-Client-Secret"] = cfg.AccessClientSecret
	}

	client := *httpClient
	client.Transport = headerRoundTripper{
		base:    httpClient.Transport,
		headers: headers,
	}

	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	return &GrafanaClient{
		prometheus: NewPrometheus(baseURL+"/api/datasources/proxy/uid/"+grafanaPrometheusUID, &client),
		loki:       NewLoki(baseURL+"/api/datasources/proxy/uid/"+grafanaLokiUID, &client),
		headers:    headers,
	}
}

func (c *GrafanaClient) Query(ctx context.Context, query string) ([]MetricSample, error) {
	return c.prometheus.Query(ctx, query)
}

func (c *GrafanaClient) QueryRange(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]MetricSample, error) {
	return c.prometheus.QueryRange(ctx, query, start, end, step)
}

func (c *GrafanaClient) QueryLogs(ctx context.Context, q LogQuery) ([]LogLine, bool, error) {
	return c.loki.QueryLogs(ctx, q)
}

type headerRoundTripper struct {
	base    http.RoundTripper
	headers map[string]string
}

func (t headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	for name, value := range t.headers {
		cloned.Header.Set(name, value)
	}

	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(cloned)
}
