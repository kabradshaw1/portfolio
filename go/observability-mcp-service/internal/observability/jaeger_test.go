package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJaegerRejectsEmptyTraceID(t *testing.T) {
	client := NewJaeger("http://example.test", nil, 100)
	if _, err := client.Trace(context.Background(), ""); err == nil {
		t.Fatal("expected empty trace_id error")
	}
}

func TestJaegerTraceParsesAndCapsSpans(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/traces/abc123" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[{"traceID":"abc123","processes":{"p1":{"serviceName":"go-ai-service"},"p2":{"serviceName":"chat"}},"spans":[{"operationName":"fast","processID":"p2","duration":1000,"tags":[]},{"operationName":"slow","processID":"p1","duration":250000,"tags":[{"key":"error","value":true}]}]}]}`))
	}))
	defer server.Close()

	client := NewJaeger(server.URL, server.Client(), 1)
	got, err := client.Trace(context.Background(), "abc123")
	if err != nil {
		t.Fatalf("Trace() error = %v", err)
	}
	if got.TraceID != "abc123" || got.SpanCount != 2 || !got.Truncated {
		t.Fatalf("summary = %+v", got)
	}
	if len(got.Spans) != 1 || got.Spans[0].Operation != "slow" || got.Spans[0].Service != "go-ai-service" || got.Spans[0].DurationMS != 250 || !got.Spans[0].Error {
		t.Fatalf("spans = %+v", got.Spans)
	}
}

func TestJaegerHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()
	client := NewJaeger(server.URL, server.Client(), 100)
	if _, err := client.Trace(context.Background(), "abc123"); err == nil {
		t.Fatal("expected error")
	}
}
