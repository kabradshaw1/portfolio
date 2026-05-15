package evalapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestListDatasetsAddsBearerToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/datasets" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token-123" {
			t.Fatalf("Authorization = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"datasets": []Dataset{{ID: "ds-1", Name: "rag", ItemCount: 2}}})
	}))
	defer server.Close()

	client := New(server.URL, "token-123", server.Client())
	got, err := client.ListDatasets(context.Background())
	if err != nil {
		t.Fatalf("ListDatasets error: %v", err)
	}
	if len(got) != 1 || got[0].ID != "ds-1" {
		t.Fatalf("datasets = %#v", got)
	}
}

func TestStartEvaluationSendsOptionalFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body StartEvaluationRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body.DatasetID != "ds-1" || body.Collection != "documents" || body.Notes != "candidate" || body.BaselineEvalID != "eval-base" || !body.Rerank {
			t.Fatalf("body = %#v", body)
		}
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(StartEvaluationResponse{ID: "eval-2", Status: "running"})
	}))
	defer server.Close()

	client := New(server.URL, "", server.Client())
	got, err := client.StartEvaluation(context.Background(), StartEvaluationRequest{
		DatasetID:      "ds-1",
		Collection:     "documents",
		Notes:          "candidate",
		BaselineEvalID: "eval-base",
		Rerank:         true,
	})
	if err != nil {
		t.Fatalf("StartEvaluation error: %v", err)
	}
	if got.ID != "eval-2" || got.Status != "running" {
		t.Fatalf("response = %#v", got)
	}
}

func TestGetEvaluationAndCompare(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/evaluations/eval-1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":     "eval-1",
				"status": "completed",
				"aggregate_scores": map[string]float64{
					"context_precision": 0.42,
				},
				"results": []map[string]any{{
					"query":    "what is rag?",
					"answer":   "retrieval augmented generation",
					"contexts": []string{"context"},
					"scores": map[string]float64{
						"faithfulness": 0.9,
					},
					"score_reasons": map[string]string{
						"faithfulness": "answer is grounded in the retrieved context",
					},
				}},
			})
		case "/evaluations/compare":
			if got := r.URL.Query().Get("ids"); got != "eval-1,eval-2" {
				t.Fatalf("ids = %q", got)
			}
			_ = json.NewEncoder(w).Encode(Comparison{Runs: []EvaluationDetail{{ID: "eval-1"}, {ID: "eval-2"}}, Deltas: map[string][]float64{"context_precision": {0, 0.1}}})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := New(server.URL, "", server.Client())
	run, err := client.GetEvaluation(context.Background(), "eval-1")
	if err != nil {
		t.Fatalf("GetEvaluation error: %v", err)
	}
	if run.AggregateScores == nil || run.AggregateScores.ContextPrecision == nil || *run.AggregateScores.ContextPrecision != 0.42 {
		t.Fatalf("run = %#v", run)
	}
	if len(run.Results) != 1 || run.Results[0].ScoreReasons["faithfulness"] != "answer is grounded in the retrieved context" {
		t.Fatalf("score reasons = %#v", run.Results)
	}
	comp, err := client.CompareEvaluations(context.Background(), []string{"eval-1", "eval-2"})
	if err != nil {
		t.Fatalf("CompareEvaluations error: %v", err)
	}
	if len(comp.Runs) != 2 {
		t.Fatalf("comparison = %#v", comp)
	}
}

func TestHTTPErrorIncludesStatusAndExcerpt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, strings.Repeat("x", 300), http.StatusUnauthorized)
	}))
	defer server.Close()

	client := New(server.URL, "", server.Client())
	_, err := client.ListDatasets(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "status 401") {
		t.Fatalf("error = %v", err)
	}
}

func TestListDatasetsRetriesOnceAfterUnauthorized(t *testing.T) {
	provider := &sequenceTokenProvider{tokens: []string{"expired-token", "fresh-token"}}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch requests {
		case 1:
			if got := r.Header.Get("Authorization"); got != "Bearer expired-token" {
				t.Fatalf("first Authorization = %q", got)
			}
			http.Error(w, "expired", http.StatusUnauthorized)
		case 2:
			if got := r.Header.Get("Authorization"); got != "Bearer fresh-token" {
				t.Fatalf("retry Authorization = %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"datasets": []Dataset{{ID: "ds-retry", Name: "rag"}}})
		default:
			t.Fatalf("unexpected request %d", requests)
		}
	}))
	defer server.Close()

	client := NewWithTokenProvider(server.URL, provider, server.Client())
	got, err := client.ListDatasets(context.Background())
	if err != nil {
		t.Fatalf("ListDatasets error: %v", err)
	}
	if provider.invalidations != 1 {
		t.Fatalf("invalidations = %d", provider.invalidations)
	}
	if provider.calls != 2 {
		t.Fatalf("token calls = %d", provider.calls)
	}
	if requests != 2 {
		t.Fatalf("requests = %d", requests)
	}
	if len(got) != 1 || got[0].ID != "ds-retry" {
		t.Fatalf("datasets = %#v", got)
	}
}

func TestListDatasetsDoesNotRetryNonUnauthorizedError(t *testing.T) {
	provider := &sequenceTokenProvider{tokens: []string{"token-1"}}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Error(w, "broken", http.StatusBadGateway)
	}))
	defer server.Close()

	client := NewWithTokenProvider(server.URL, provider, server.Client())
	_, err := client.ListDatasets(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if requests != 1 {
		t.Fatalf("requests = %d", requests)
	}
	if provider.invalidations != 0 {
		t.Fatalf("invalidations = %d", provider.invalidations)
	}
	if !strings.Contains(err.Error(), "status 502") {
		t.Fatalf("error = %v", err)
	}
}

func TestNewUsesDefaultHTTPClient(t *testing.T) {
	client := New("http://example.test/eval", "", nil)
	if client.httpClient == nil {
		t.Fatal("httpClient is nil")
	}
	if client.httpClient.Timeout != 30*time.Second {
		t.Fatalf("timeout = %s", client.httpClient.Timeout)
	}
}

type sequenceTokenProvider struct {
	tokens        []string
	calls         int
	invalidations int
}

func (p *sequenceTokenProvider) Token(context.Context) (string, error) {
	if p.calls >= len(p.tokens) {
		return "", nil
	}
	token := p.tokens[p.calls]
	p.calls++
	return token, nil
}

func (p *sequenceTokenProvider) Invalidate() {
	p.invalidations++
}
