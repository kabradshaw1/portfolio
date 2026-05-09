package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPrometheusInstantQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("query") != "up" {
			t.Fatalf("query = %s", r.URL.Query().Get("query"))
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{"job":"prometheus"},"value":[1710000000,"1"]}]}}`))
	}))
	defer server.Close()

	client := NewPrometheus(server.URL, server.Client())
	got, err := client.Query(context.Background(), "up")
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(got) != 1 || got[0].Value != 1 {
		t.Fatalf("samples = %+v", got)
	}
}

func TestPrometheusRangeQueryParsesMatrix(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query_range" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("query") != "rate(http_requests_total[5m])" {
			t.Fatalf("query = %s", r.URL.Query().Get("query"))
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"service":"go-ai-service"},"values":[[1710000000,"1.5"],[1710000060,"2.5"]]}]}}`))
	}))
	defer server.Close()

	client := NewPrometheus(server.URL+"/", server.Client())
	got, err := client.QueryRange(context.Background(), "rate(http_requests_total[5m])", time.Unix(1710000000, 0), time.Unix(1710000060, 0), time.Minute)
	if err != nil {
		t.Fatalf("QueryRange() error = %v", err)
	}
	if len(got) != 2 || got[1].Value != 2.5 {
		t.Fatalf("samples = %+v", got)
	}
}

func TestPrometheusErrors(t *testing.T) {
	tests := map[string]string{
		"prometheus status": `{"status":"error","error":"bad query"}`,
		"http status":       ``,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if body == "" {
					http.Error(w, "boom", http.StatusInternalServerError)
					return
				}
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			client := NewPrometheus(server.URL, server.Client())
			if _, err := client.Query(context.Background(), "up"); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
