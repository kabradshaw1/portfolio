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
		if body.DatasetID != "ds-1" || body.Collection != "documents" || body.Notes != "candidate" || body.BaselineEvalID != "eval-base" || body.ExperimentID != "exp-1" || body.ExperimentLabel != "candidate" || !body.Rerank {
			t.Fatalf("body = %#v", body)
		}
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(StartEvaluationResponse{ID: "eval-2", Status: "running"})
	}))
	defer server.Close()

	client := New(server.URL, "", server.Client())
	got, err := client.StartEvaluation(context.Background(), StartEvaluationRequest{
		DatasetID:       "ds-1",
		Collection:      "documents",
		Notes:           "candidate",
		BaselineEvalID:  "eval-base",
		ExperimentID:    "exp-1",
		ExperimentLabel: "candidate",
		Rerank:          true,
	})
	if err != nil {
		t.Fatalf("StartEvaluation error: %v", err)
	}
	if got.ID != "eval-2" || got.Status != "running" {
		t.Fatalf("response = %#v", got)
	}
}

func TestExperimentAPIMethods(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/experiments":
			switch r.Method {
			case http.MethodPost:
				var body CreateExperimentRequest
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatalf("decode create experiment: %v", err)
				}
				if body.Name != "precision tuning" || body.FocusMetric != "context_precision" {
					t.Fatalf("create body = %#v", body)
				}
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(Experiment{ID: "exp-1", Name: body.Name, DatasetID: body.DatasetID, Collection: body.Collection, FocusMetric: body.FocusMetric, Status: "running"})
			case http.MethodGet:
				_ = json.NewEncoder(w).Encode(map[string]any{"experiments": []Experiment{{ID: "exp-1", Name: "precision tuning", DatasetID: "ds-1", Collection: "documents", FocusMetric: "context_precision", Status: "running"}}})
			default:
				t.Fatalf("unexpected method %s", r.Method)
			}
		case "/experiments/exp-1":
			switch r.Method {
			case http.MethodGet:
				_ = json.NewEncoder(w).Encode(Experiment{ID: "exp-1", Name: "precision tuning", DatasetID: "ds-1", Collection: "documents", FocusMetric: "context_precision", Status: "running"})
			case http.MethodPatch:
				var body UpdateExperimentRequest
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatalf("decode update experiment: %v", err)
				}
				if body.Status != "completed" || body.Decision != "keep" || body.Conclusion != "Keep reranking." {
					t.Fatalf("update body = %#v", body)
				}
				_ = json.NewEncoder(w).Encode(Experiment{ID: "exp-1", Name: "precision tuning", DatasetID: "ds-1", Collection: "documents", FocusMetric: "context_precision", Status: "completed", Decision: "keep", Conclusion: "Keep reranking.", Evidence: body.Evidence})
			default:
				t.Fatalf("unexpected method %s", r.Method)
			}
		case "/experiments/exp-1/runs":
			var body AttachExperimentRunRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode attach run: %v", err)
			}
			if body.EvaluationID != "eval-candidate" || body.Label != "candidate" {
				t.Fatalf("attach body = %#v", body)
			}
			_ = json.NewEncoder(w).Encode(Experiment{ID: "exp-1", Name: "precision tuning", DatasetID: "ds-1", Collection: "documents", FocusMetric: "context_precision", Status: "running"})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := New(server.URL, "", server.Client())
	ctx := context.Background()
	exp, err := client.CreateExperiment(ctx, CreateExperimentRequest{Name: "precision tuning", Hypothesis: "rerank improves precision", DatasetID: "ds-1", Collection: "documents", FocusMetric: "context_precision", Status: "running"})
	if err != nil {
		t.Fatalf("CreateExperiment error: %v", err)
	}
	if exp.ID != "exp-1" {
		t.Fatalf("created experiment = %#v", exp)
	}
	experiments, err := client.ListExperiments(ctx)
	if err != nil || len(experiments) != 1 {
		t.Fatalf("ListExperiments = %#v, %v", experiments, err)
	}
	got, err := client.GetExperiment(ctx, "exp-1")
	if err != nil || got.FocusMetric != "context_precision" {
		t.Fatalf("GetExperiment = %#v, %v", got, err)
	}
	if _, err := client.AttachExperimentRun(ctx, "exp-1", AttachExperimentRunRequest{EvaluationID: "eval-candidate", Label: "candidate"}); err != nil {
		t.Fatalf("AttachExperimentRun error: %v", err)
	}
	updated, err := client.UpdateExperiment(ctx, "exp-1", UpdateExperimentRequest{Status: "completed", Decision: "keep", Conclusion: "Keep reranking.", Evidence: map[string]any{"baseline_eval_id": "eval-base"}})
	if err != nil {
		t.Fatalf("UpdateExperiment error: %v", err)
	}
	if updated.Status != "completed" || updated.Evidence["baseline_eval_id"] != "eval-base" {
		t.Fatalf("updated experiment = %#v", updated)
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

func TestNewUsesDefaultHTTPClient(t *testing.T) {
	client := New("http://example.test/eval", "", nil)
	if client.httpClient == nil {
		t.Fatal("httpClient is nil")
	}
	if client.httpClient.Timeout != 30*time.Second {
		t.Fatalf("timeout = %s", client.httpClient.Timeout)
	}
}
