package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLokiQueryLogsCapsResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/loki/api/v1/query_range" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		query := r.URL.Query().Get("query")
		if !strings.Contains(query, `{service="go-ai-service"}`) {
			t.Fatalf("query missing service selector: %s", query)
		}
		if !strings.Contains(query, `(?i)(error|warn|exception)`) {
			t.Fatalf("query missing default pattern: %s", query)
		}
		if r.URL.Query().Get("limit") != "2" {
			t.Fatalf("limit = %s", r.URL.Query().Get("limit"))
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"result":[{"stream":{"service":"go-ai-service"},"values":[["1710000000000000000","first error"],["1710000001000000000","second error"]]}]}}`))
	}))
	defer server.Close()

	client := NewLoki(server.URL, server.Client())
	lines, truncated, err := client.QueryLogs(context.Background(), LogQuery{
		Service: "go-ai-service",
		Start:   time.Unix(1710000000, 0),
		End:     time.Unix(1710000060, 0),
		Limit:   1,
	})
	if err != nil {
		t.Fatalf("QueryLogs() error = %v", err)
	}
	if len(lines) != 1 || !truncated {
		t.Fatalf("lines=%d truncated=%v", len(lines), truncated)
	}
}

func TestLokiErrors(t *testing.T) {
	tests := map[string]string{
		"loki status": `{"status":"error","error":"bad query"}`,
		"http status": ``,
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
			client := NewLoki(server.URL, server.Client())
			_, _, err := client.QueryLogs(context.Background(), LogQuery{Service: "go-ai-service", Start: time.Now().Add(-time.Minute), End: time.Now(), Limit: 10})
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
