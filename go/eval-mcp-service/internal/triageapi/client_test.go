package triageapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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
		var req struct {
			BaselineEvalID       string `json:"baseline_eval_id"`
			CandidateEvalID      string `json:"candidate_eval_id"`
			Metric               string `json:"metric"`
			Limit                int    `json:"limit"`
			IncludeObservability bool   `json:"include_observability"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.BaselineEvalID != "baseline" || req.CandidateEvalID != "candidate" {
			t.Fatalf("comparison request = %#v", req)
		}
		if req.Metric != "context_precision" || req.Limit != 5 || !req.IncludeObservability {
			t.Fatalf("optional request fields = %#v", req)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "completed"})
	}))
	defer server.Close()

	client := New(server.URL, nil, server.Client())
	_, err := client.TriageRAGRegression(context.Background(), TriageRequest{
		EvalID:               "candidate",
		BaselineEvalID:       "baseline",
		Metric:               "context_precision",
		Limit:                5,
		IncludeObservability: true,
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
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("error type = %T", err)
	}
	if httpErr.StatusCode != http.StatusNotFound {
		t.Fatalf("status code = %d", httpErr.StatusCode)
	}
	if !strings.Contains(err.Error(), "POST /triage/eval-run: status 404") || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("error = %v", err)
	}
}

func TestClientTriageRAGRegressionRetriesUnauthorizedWithFreshToken(t *testing.T) {
	var calls int
	var authHeaders []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		authHeaders = append(authHeaders, r.Header.Get("Authorization"))
		if calls == 1 {
			http.Error(w, "expired", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "completed"})
	}))
	defer server.Close()

	tokenProvider := &rotatingToken{tokens: []string{"old-token", "new-token"}}
	client := New(server.URL, tokenProvider, server.Client())
	result, err := client.TriageRAGRegression(context.Background(), TriageRequest{EvalID: "eval-1"})
	if err != nil {
		t.Fatal(err)
	}
	if result["status"].(string) != "completed" {
		t.Fatalf("result = %#v", result)
	}
	if calls != 2 {
		t.Fatalf("calls = %d", calls)
	}
	if !tokenProvider.invalidated {
		t.Fatal("expected token provider to be invalidated")
	}
	if authHeaders[0] != "Bearer old-token" || authHeaders[1] != "Bearer new-token" {
		t.Fatalf("auth headers = %#v", authHeaders)
	}
}

type staticToken string

func (s staticToken) Token(context.Context) (string, error) { return string(s), nil }
func (s staticToken) Invalidate()                           {}

type rotatingToken struct {
	tokens      []string
	calls       int
	invalidated bool
}

func (r *rotatingToken) Token(context.Context) (string, error) {
	token := r.tokens[r.calls]
	r.calls++
	return token, nil
}

func (r *rotatingToken) Invalidate() {
	r.invalidated = true
}
