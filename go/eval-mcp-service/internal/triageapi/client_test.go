package triageapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientTriageRAGRegressionSingleRun(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/triage/eval-run" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		var req TriageRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.EvalID != "eval-1" {
			t.Fatalf("eval_id = %q", req.EvalID)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "completed"})
	}))
	defer server.Close()

	client := New(server.URL, staticToken("token-1"), server.Client())
	result, err := client.TriageRAGRegression(context.Background(), TriageRequest{EvalID: "eval-1"})
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer token-1" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if result["status"].(string) != "completed" {
		t.Fatalf("result = %#v", result)
	}
}

func TestClientTriageRAGRegressionComparison(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/triage/comparison" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "completed"})
	}))
	defer server.Close()

	client := New(server.URL, nil, server.Client())
	_, err := client.TriageRAGRegression(context.Background(), TriageRequest{
		EvalID:         "candidate",
		BaselineEvalID: "baseline",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestClientTriageRAGRegressionReturnsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"detail":"missing"}`, http.StatusNotFound)
	}))
	defer server.Close()

	client := New(server.URL, nil, server.Client())
	_, err := client.TriageRAGRegression(context.Background(), TriageRequest{EvalID: "missing"})
	if err == nil {
		t.Fatal("expected error")
	}
}

type staticToken string

func (s staticToken) Token(context.Context) (string, error) { return string(s), nil }
func (s staticToken) Invalidate()                           {}
