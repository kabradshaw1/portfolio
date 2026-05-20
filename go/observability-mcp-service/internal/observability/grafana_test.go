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

func TestGrafanaDatasourceUIDOverrides(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/datasources/proxy/uid/custom-prometheus/api/v1/query":
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
		case "/api/datasources/proxy/uid/custom-loki/loki/api/v1/query_range":
			_, _ = w.Write([]byte(`{"status":"success","data":{"result":[]}}`))
		default:
			t.Fatalf("path = %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewGrafana(GrafanaConfig{
		BaseURL:                 server.URL,
		PrometheusDatasourceUID: "custom-prometheus",
		LokiDatasourceUID:       "custom-loki",
	}, server.Client())
	if _, err := client.Query(context.Background(), "up"); err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	_, _, err := client.QueryLogs(context.Background(), LogQuery{
		Service: "go-order-service",
		Start:   time.Unix(1710000000, 0).UTC(),
		End:     time.Unix(1710000060, 0).UTC(),
		Limit:   2,
	})
	if err != nil {
		t.Fatalf("QueryLogs() error = %v", err)
	}
}

func TestGrafanaCloudflareHeadersRequirePair(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("CF-Access-Client-Id"); got != "" {
			t.Fatalf("CF-Access-Client-Id = %q", got)
		}
		if got := r.Header.Get("CF-Access-Client-Secret"); got != "" {
			t.Fatalf("CF-Access-Client-Secret = %q", got)
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	}))
	defer server.Close()

	client := NewGrafana(GrafanaConfig{
		BaseURL:            server.URL,
		AccessClientSecret: "cf-secret",
	}, server.Client())
	if _, err := client.Query(context.Background(), "up"); err != nil {
		t.Fatalf("Query() error = %v", err)
	}
}

func TestGrafanaActiveAlertsUsesAlertmanagerAPI(t *testing.T) {
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
		if r.URL.Path != "/api/alertmanager/grafana/api/v2/alerts" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[
			{
				"labels":{"alertname":"HighErrorRate","grafana_rule_uid":"rule-123","service":"go-order-service"},
				"annotations":{"summary":"Order service errors","__dashboardUid__":"orders"},
				"startsAt":"2026-05-20T12:00:00Z",
				"endsAt":"0001-01-01T00:00:00Z",
				"generatorURL":"https://grafana.example/alerting/grafana/rule-123/view",
				"status":{"state":"active"}
			}
		]`))
	}))
	defer server.Close()

	client := NewGrafana(GrafanaConfig{
		BaseURL:            server.URL,
		Token:              "grafana-token",
		AccessClientID:     "cf-id",
		AccessClientSecret: "cf-secret",
	}, server.Client())

	got, err := client.ActiveAlerts(context.Background())
	if err != nil {
		t.Fatalf("ActiveAlerts() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("alerts = %+v", got)
	}
	if got[0].Name != "HighErrorRate" || got[0].State != "active" || got[0].RuleUID != "rule-123" {
		t.Fatalf("alert = %+v", got[0])
	}
	if got[0].Annotations["summary"] != "Order service errors" {
		t.Fatalf("annotations = %+v", got[0].Annotations)
	}
}

func TestGrafanaAlertRulesUsesProvisioningAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/provisioning/alert-rules" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[
			{
				"uid":"rule-123",
				"title":"HighErrorRate",
				"folderUID":"go-services",
				"ruleGroup":"slo",
				"condition":"C",
				"labels":{"service":"go-order-service"},
				"provenance":"file"
			}
		]`))
	}))
	defer server.Close()

	client := NewGrafana(GrafanaConfig{BaseURL: server.URL}, server.Client())
	got, err := client.AlertRules(context.Background())
	if err != nil {
		t.Fatalf("AlertRules() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("rules = %+v", got)
	}
	if got[0].UID != "rule-123" || got[0].Title != "HighErrorRate" || got[0].FolderUID != "go-services" {
		t.Fatalf("rule = %+v", got[0])
	}
}

func TestGrafanaAlertingBoundsLabelsAndAnnotations(t *testing.T) {
	labels := map[string]string{
		"alertname": "Noisy",
		"keep1":     "1",
		"keep2":     "2",
		"keep3":     "3",
		"keep4":     "4",
		"keep5":     "5",
		"drop":      "6",
	}
	got := boundedMap(labels, 5)
	if len(got) != 5 {
		t.Fatalf("bounded labels length = %d", len(got))
	}
	if got["alertname"] != "Noisy" {
		t.Fatalf("bounded labels = %+v", got)
	}
}
