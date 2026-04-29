package composite

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJaegerTraceSourceDecodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/api/traces/abc") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"data":[{"spans":[{"operationName":"checkout","duration":1234000},{"operationName":"reserve","duration":456000}]}]}`)
	}))
	defer srv.Close()
	src := JaegerTraceSource{BaseURL: srv.URL, HTTP: srv.Client()}
	got, err := src.FetchTrace(context.Background(), "abc")
	if err != nil {
		t.Fatalf("FetchTrace: %v", err)
	}
	if got.ID != "abc" || len(got.Spans) != 2 {
		t.Fatalf("unexpected: %+v", got)
	}
	if got.Spans[0].Name != "checkout" || got.Spans[0].DurationMs != 1234 {
		t.Fatalf("first span: %+v", got.Spans[0])
	}
}

func TestJaegerTraceSourceEmptyIDSkipsRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("server should not be hit on empty trace id")
	}))
	defer srv.Close()
	src := JaegerTraceSource{BaseURL: srv.URL, HTTP: srv.Client()}
	got, err := src.FetchTrace(context.Background(), "")
	if err != nil {
		t.Fatalf("FetchTrace: %v", err)
	}
	if got.ID != "" || len(got.Spans) != 0 {
		t.Fatalf("expected zero TraceSummary, got %+v", got)
	}
}

func TestJaegerTraceSourceErrorOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	src := JaegerTraceSource{BaseURL: srv.URL, HTTP: srv.Client()}
	_, err := src.FetchTrace(context.Background(), "abc")
	if err == nil {
		t.Fatalf("expected error on 500")
	}
}

func TestLokiLogSourceDecodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/loki/api/v1/query_range" {
			t.Errorf("path: %s", r.URL.Path)
		}
		q := r.URL.Query().Get("query")
		if !strings.Contains(q, `service=~"order-service|payment-service"`) {
			t.Errorf("query: %s", q)
		}
		fmt.Fprint(w, `{"data":{"result":[{"values":[["1","line one"],["2","line two"]]}]}}`)
	}))
	defer srv.Close()
	src := LokiLogSource{BaseURL: srv.URL, HTTP: srv.Client()}
	got, err := src.FetchLogs(context.Background(), []string{"order-service", "payment-service"}, 100, 200)
	if err != nil {
		t.Fatalf("FetchLogs: %v", err)
	}
	if len(got) != 2 || got[0] != "line one" || got[1] != "line two" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestLokiLogSourceEmptyServicesNoCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("server should not be hit when services empty")
	}))
	defer srv.Close()
	src := LokiLogSource{BaseURL: srv.URL, HTTP: srv.Client()}
	got, err := src.FetchLogs(context.Background(), nil, 100, 200)
	if err != nil {
		t.Fatalf("FetchLogs: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestNopRabbitSourceReturnsEmpty(t *testing.T) {
	got, err := NopRabbitSource{}.FetchEvents(context.Background(), "any")
	if err != nil {
		t.Fatalf("FetchEvents: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty, got %+v", got)
	}
}
