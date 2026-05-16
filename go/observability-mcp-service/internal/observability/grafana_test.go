package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGrafanaPrometheusQueryUsesDatasourceProxy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer grafana-token" {
			t.Fatalf("Authorization = %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("CF-Access-Client-Id") != "cf-id" {
			t.Fatalf("CF-Access-Client-Id = %s", r.Header.Get("CF-Access-Client-Id"))
		}
		if r.Header.Get("CF-Access-Client-Secret") != "cf-secret" {
			t.Fatalf("CF-Access-Client-Secret = %s", r.Header.Get("CF-Access-Client-Secret"))
		}
		if r.URL.Path != "/api/datasources/proxy/uid/PBFA97CFB590B2093/api/v1/query" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("query") != "up" {
			t.Fatalf("query = %s", r.URL.Query().Get("query"))
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{"job":"prometheus"},"value":[1710000000,"1"]}]}}`))
	}))
	defer server.Close()

	client := NewGrafana(GrafanaConfig{
		BaseURL:            server.URL,
		Token:              "grafana-token",
		AccessClientID:     "cf-id",
		AccessClientSecret: "cf-secret",
	}, server.Client())
	got, err := client.Query(context.Background(), "up")
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(got) != 1 || got[0].Value != 1 {
		t.Fatalf("samples = %+v", got)
	}
}

func TestGrafanaLokiQueryUsesDatasourceProxy(t *testing.T) {
	start := time.Unix(1710000000, 0).UTC()
	end := time.Unix(1710000060, 0).UTC()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/datasources/proxy/uid/loki/loki/api/v1/query_range" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("start") != "1710000000000000000" {
			t.Fatalf("start = %s", r.URL.Query().Get("start"))
		}
		if r.URL.Query().Get("end") != "1710000060000000000" {
			t.Fatalf("end = %s", r.URL.Query().Get("end"))
		}
		if r.URL.Query().Get("limit") != "3" {
			t.Fatalf("limit = %s", r.URL.Query().Get("limit"))
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"result":[{"stream":{"service":"go-ai-service"},"values":[["1710000000000000000","first error"]]}]}}`))
	}))
	defer server.Close()

	client := NewGrafana(GrafanaConfig{BaseURL: server.URL}, server.Client())
	lines, truncated, err := client.QueryLogs(context.Background(), LogQuery{
		Service: "go-ai-service",
		Start:   start,
		End:     end,
		Limit:   2,
	})
	if err != nil {
		t.Fatalf("QueryLogs() error = %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("lines = %+v", lines)
	}
	if lines[0].Labels["service"] != "go-ai-service" {
		t.Fatalf("service label = %s", lines[0].Labels["service"])
	}
	if truncated {
		t.Fatal("truncated = true")
	}
}
