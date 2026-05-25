package evalworkflow

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/corpusfixture"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/evalapi"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/fixturecatalog"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/ingestionapi"
)

func TestStartExperimentDefaultsCollectionAndFocusMetric(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{}
	svc := newTestService(api)

	got, err := svc.StartExperiment(ctx, StartExperimentInput{
		Name:      "rerank trial",
		DatasetID: "dataset-1",
	})
	if err != nil {
		t.Fatalf("StartExperiment error: %v", err)
	}
	if got.Collection != DefaultCollection {
		t.Fatalf("Collection = %q, want %q", got.Collection, DefaultCollection)
	}
	if got.FocusMetric != DefaultFocusMetric {
		t.Fatalf("FocusMetric = %q, want %q", got.FocusMetric, DefaultFocusMetric)
	}
	if api.createExperimentRequests[0].Collection != DefaultCollection || api.createExperimentRequests[0].FocusMetric != DefaultFocusMetric {
		t.Fatalf("CreateExperiment input = %#v", api.createExperimentRequests[0])
	}
}

func TestStartRunSendsExperimentAttachmentToEvalAPI(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{startResponse: evalapi.StartEvaluationResponse{ID: "eval-123", Status: "queued"}}
	svc := newTestService(api)
	topK := 3

	got, err := svc.StartRun(ctx, StartRunInput{
		DatasetID:          "dataset-1",
		Collection:         "kb",
		Notes:              "candidate notes",
		ExperimentID:       "exp-7",
		Label:              "candidate",
		BaselineEvalID:     "eval-base",
		Rerank:             true,
		RetrievalConfig:    &evalapi.RetrievalConfig{TopK: &topK},
		AnswerTier:         "efficient",
		AnswerProvider:     "openai",
		AnswerBaseURL:      "https://api.openai.com/v1",
		AnswerModel:        "gpt-5.4-mini",
		AnswerAPIKeySecret: "OPENAI_API_KEY",
	})
	if err != nil {
		t.Fatalf("StartRun error: %v", err)
	}
	if got.EvalID != "eval-123" || got.Status != "queued" {
		t.Fatalf("StartRunResult = %#v", got)
	}
	if len(api.startRequests) != 1 {
		t.Fatalf("StartEvaluation calls = %d, want 1", len(api.startRequests))
	}
	req := api.startRequests[0]
	if req.ExperimentID != "exp-7" || req.ExperimentLabel != "candidate" {
		t.Fatalf("StartEvaluation request = %#v", req)
	}
	if req.AnswerTier != "efficient" || req.AnswerProvider != "openai" || req.AnswerBaseURL != "https://api.openai.com/v1" || req.AnswerModel != "gpt-5.4-mini" || req.AnswerAPIKeySecret != "OPENAI_API_KEY" {
		t.Fatalf("answer model override = %#v", req)
	}
	if req.RetrievalConfig == nil || req.RetrievalConfig.TopK == nil || *req.RetrievalConfig.TopK != 3 {
		t.Fatalf("retrieval config = %#v", req.RetrievalConfig)
	}
}

func TestCreateDatasetFromFixture(t *testing.T) {
	api := &fakeAPI{}
	fixture := fixturecatalog.Fixture{
		Name: "product-docs-rag-v1",
		Items: []fixturecatalog.GoldenItem{{
			Query: "q", ExpectedAnswer: "a", ExpectedSources: []string{"doc.pdf"},
		}},
		Valid: true,
	}
	service := New(api, fakeIngestion{}, fakeFixtures{fixture: fixture}, time.Second, time.Minute)

	got, err := service.CreateDatasetFromFixture(context.Background(), "rag-eval-dataset-product-docs.json")
	if err != nil {
		t.Fatalf("CreateDatasetFromFixture returned error: %v", err)
	}
	if got.ID != "ds-created" || got.Name != "product-docs-rag-v1" {
		t.Fatalf("unexpected result: %#v", got)
	}
	if api.createDatasetRequest.Name != "product-docs-rag-v1" || len(api.createDatasetRequest.Items) != 1 {
		t.Fatalf("unexpected request: %#v", api.createDatasetRequest)
	}
}

func TestCheckReadinessDelegatesToEvalAPI(t *testing.T) {
	api := &fakeAPI{readinessResponse: evalapi.RAGReadinessResponse{Status: "ready"}}
	svc := newTestService(api)

	got, err := svc.CheckReadiness(context.Background(), ReadinessInput{
		DatasetID:  "ds-1",
		Collection: "documents",
		Rerank:     true,
	})
	if err != nil {
		t.Fatalf("CheckReadiness error: %v", err)
	}
	if got.Status != "ready" {
		t.Fatalf("readiness = %#v", got)
	}
	if api.readinessRequest.DatasetID != "ds-1" || api.readinessRequest.Collection != "documents" || !api.readinessRequest.Rerank {
		t.Fatalf("request = %#v", api.readinessRequest)
	}
}

func TestStartRunBlocksWhenReadinessBlocked(t *testing.T) {
	api := &fakeAPI{readinessResponse: evalapi.RAGReadinessResponse{
		Status: "blocked",
		BlockingFailures: []evalapi.ReadinessFinding{{
			Code: "collection_empty",
		}},
	}}
	svc := newTestService(api)

	_, err := svc.StartRun(context.Background(), StartRunInput{
		DatasetID:  "ds-1",
		Collection: "documents",
	})
	if err == nil || !strings.Contains(err.Error(), "readiness blocked") || !strings.Contains(err.Error(), "collection_empty") {
		t.Fatalf("error = %v", err)
	}
	if len(api.startRequests) != 0 {
		t.Fatalf("StartEvaluation calls = %d, want 0", len(api.startRequests))
	}
}

func TestStartRunAllowsReadinessWarning(t *testing.T) {
	api := &fakeAPI{
		readinessResponse: evalapi.RAGReadinessResponse{
			Status:   "warning",
			Warnings: []evalapi.ReadinessFinding{{Code: "top_k_override"}},
		},
		startResponse: evalapi.StartEvaluationResponse{ID: "eval-1", Status: "queued"},
	}
	svc := newTestService(api)

	got, err := svc.StartRun(context.Background(), StartRunInput{
		DatasetID:  "ds-1",
		Collection: "documents",
	})
	if err != nil {
		t.Fatalf("StartRun error: %v", err)
	}
	if got.EvalID != "eval-1" || len(api.startRequests) != 1 {
		t.Fatalf("result=%#v startRequests=%d", got, len(api.startRequests))
	}
}

func TestStartExperimentBlocksWhenReadinessBlocked(t *testing.T) {
	api := &fakeAPI{readinessResponse: evalapi.RAGReadinessResponse{
		Status:           "blocked",
		BlockingFailures: []evalapi.ReadinessFinding{{Code: "collection_empty"}},
	}}
	svc := newTestService(api)

	_, err := svc.StartExperiment(context.Background(), StartExperimentInput{
		Name:       "experiment",
		DatasetID:  "ds-1",
		Collection: "documents",
	})
	if err == nil || !strings.Contains(err.Error(), "readiness blocked") {
		t.Fatalf("error = %v", err)
	}
	if len(api.createExperimentRequests) != 0 {
		t.Fatalf("CreateExperiment calls = %d, want 0", len(api.createExperimentRequests))
	}
}

func TestStartRunRejectsMissingCollection(t *testing.T) {
	api := &fakeAPI{}
	service := New(api, fakeIngestion{collections: []ingestionapi.Collection{{Name: "documents"}}}, fakeFixtures{}, time.Second, time.Minute)
	_, err := service.StartRun(context.Background(), StartRunInput{DatasetID: "ds-1", Collection: "missing"})
	if err == nil || !strings.Contains(err.Error(), `retrieval collection "missing" does not exist`) {
		t.Fatalf("error = %v", err)
	}
}

func TestProvisionRAGCorpusUploadsFixtureAndWritesManifest(t *testing.T) {
	root := t.TempDir()
	pdfPath := filepath.Join(root, "laptop.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.4\nlaptop"), 0o644); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	ingestion := &recordingCorpusIngestion{
		collections: []ingestionapi.Collection{},
		uploadResponse: ingestionapi.IngestResponse{
			Status:        "success",
			DocumentID:    "doc-1",
			ChunksCreated: 4,
			Filename:      "laptop.pdf",
		},
	}
	fixtures := fakeCorpusFixtures{fixture: corpusfixture.Fixture{
		ID:                "product_catalog_v1",
		Name:              "Product Catalog v1",
		SourceHash:        "a1b2c3d4e5f6",
		DefaultCollection: "eval_product_catalog_v1_a1b2c3d4",
		Documents: []corpusfixture.Document{{
			Path:   "laptop.pdf",
			Abs:    pdfPath,
			SHA256: "abc123",
		}},
		DocumentCount: 1,
	}}
	service := New(&fakeAPI{}, ingestion, fakeFixtures{}, time.Second, time.Minute).
		WithCorpusFixtures(fixtures)

	got, err := service.ProvisionRAGCorpus(context.Background(), ProvisionCorpusInput{
		Fixture: "product_catalog_v1",
	})
	if err != nil {
		t.Fatalf("ProvisionRAGCorpus returned error: %v", err)
	}
	if got.Collection != "eval_product_catalog_v1_a1b2c3d4" || got.ChunksCreated != 4 || got.FixtureHash != "a1b2c3d4e5f6" {
		t.Fatalf("result = %#v", got)
	}
	if ingestion.uploadCollection != "eval_product_catalog_v1_a1b2c3d4" || ingestion.uploadFilename != "laptop.pdf" {
		t.Fatalf("upload collection=%q filename=%q", ingestion.uploadCollection, ingestion.uploadFilename)
	}
	if ingestion.manifest["fixture_hash"] != "a1b2c3d4e5f6" {
		t.Fatalf("manifest = %#v", ingestion.manifest)
	}
}

func TestProvisionRAGCorpusRejectsUnsafeCollection(t *testing.T) {
	service := New(&fakeAPI{}, fakeIngestion{}, fakeFixtures{}, time.Second, time.Minute)
	_, err := service.ProvisionRAGCorpus(context.Background(), ProvisionCorpusInput{
		Fixture:    "product_catalog_v1",
		Collection: "documents",
	})
	if err == nil || !strings.Contains(err.Error(), "managed eval collection") {
		t.Fatalf("error = %v", err)
	}
}

func TestProvisionRAGCorpusIdempotentManifestReturnsTotals(t *testing.T) {
	ingestion := &recordingCorpusIngestion{
		manifestResponse: map[string]any{
			"fixture_hash": "a1b2c3d4e5f6",
			"documents": []any{
				map[string]any{"path": "laptop.pdf", "sha256": "abc123", "chunks_created": float64(4)},
			},
		},
	}
	fixtures := fakeCorpusFixtures{fixture: corpusfixture.Fixture{
		ID:                "product_catalog_v1",
		SourceHash:        "a1b2c3d4e5f6",
		DefaultCollection: "eval_product_catalog_v1_a1b2c3d4",
		DocumentCount:     1,
	}}
	service := New(&fakeAPI{}, ingestion, fakeFixtures{}, time.Second, time.Minute).
		WithCorpusFixtures(fixtures)

	got, err := service.ProvisionRAGCorpus(context.Background(), ProvisionCorpusInput{Fixture: "product_catalog_v1"})
	if err != nil {
		t.Fatalf("ProvisionRAGCorpus returned error: %v", err)
	}
	if !got.Idempotent || got.ChunksCreated != 4 || len(got.Documents) != 1 || got.Documents[0].Path != "laptop.pdf" {
		t.Fatalf("result = %#v", got)
	}
	if ingestion.uploadFilename != "" {
		t.Fatalf("unexpected upload: %q", ingestion.uploadFilename)
	}
}

func TestProvisionRAGCorpusForceRecreateIgnoresMissingCollection(t *testing.T) {
	root := t.TempDir()
	pdfPath := filepath.Join(root, "laptop.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.4\nlaptop"), 0o644); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	ingestion := &recordingCorpusIngestion{
		deleteErr: &ingestionapi.HTTPError{Method: http.MethodDelete, Path: "/collections/eval_product_catalog_v1_a1b2c3d4", StatusCode: http.StatusNotFound},
		uploadResponse: ingestionapi.IngestResponse{
			ChunksCreated: 4,
		},
	}
	fixtures := fakeCorpusFixtures{fixture: corpusfixture.Fixture{
		ID:                "product_catalog_v1",
		SourceHash:        "a1b2c3d4e5f6",
		DefaultCollection: "eval_product_catalog_v1_a1b2c3d4",
		Documents: []corpusfixture.Document{{
			Path:   "laptop.pdf",
			Abs:    pdfPath,
			SHA256: "abc123",
		}},
		DocumentCount: 1,
	}}
	service := New(&fakeAPI{}, ingestion, fakeFixtures{}, time.Second, time.Minute).
		WithCorpusFixtures(fixtures)

	got, err := service.ProvisionRAGCorpus(context.Background(), ProvisionCorpusInput{
		Fixture:       "product_catalog_v1",
		ForceRecreate: true,
	})
	if err != nil {
		t.Fatalf("ProvisionRAGCorpus returned error: %v", err)
	}
	if !ingestion.deleteCalled || got.ChunksCreated != 4 {
		t.Fatalf("deleteCalled=%v result=%#v", ingestion.deleteCalled, got)
	}
}

func TestCompareRejectsNonCompletedRuns(t *testing.T) {
	api := &fakeAPI{detailsByID: map[string][]evalapi.EvaluationDetail{
		"base": {{ID: "base", Status: "completed"}},
		"bad":  {{ID: "bad", Status: "failed"}},
	}}
	service := New(api, fakeIngestion{}, fakeFixtures{}, time.Second, time.Minute)
	_, err := service.Compare(context.Background(), CompareInput{EvalIDs: []string{"base", "bad"}})
	if err == nil || !strings.Contains(err.Error(), `bad=failed`) {
		t.Fatalf("error = %v", err)
	}
}

func TestRecordConclusionCompletesExperimentWithEvidence(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{}
	svc := newTestService(api)
	evidence := map[string]any{"baseline_eval_id": "eval-base"}

	if err := svc.RecordConclusion(ctx, RecordConclusionInput{
		ExperimentID: "exp-1",
		Decision:     "keep",
		Conclusion:   "Keep reranking.",
		Evidence:     evidence,
	}); err != nil {
		t.Fatalf("RecordConclusion error: %v", err)
	}
	if api.updateExperimentID != "exp-1" {
		t.Fatalf("updateExperimentID = %q", api.updateExperimentID)
	}
	if api.updateExperimentRequest.Status != "completed" || api.updateExperimentRequest.Decision != "keep" || api.updateExperimentRequest.Conclusion != "Keep reranking." {
		t.Fatalf("update request = %#v", api.updateExperimentRequest)
	}
}

func TestListEvalItemDLQDelegatesToAPI(t *testing.T) {
	api := &fakeAPI{dlqListResponse: evalapi.DLQListResponse{IndexesAreTransient: true}}
	svc := New(api, nil, nil, time.Second, time.Minute)

	got, err := svc.ListEvalItemDLQ(context.Background(), 7)
	if err != nil {
		t.Fatalf("ListEvalItemDLQ error: %v", err)
	}
	if api.dlqListRequestLimit != 7 || !got.IndexesAreTransient {
		t.Fatalf("limit=%d response=%#v", api.dlqListRequestLimit, got)
	}
}

func TestReplayEvalItemDLQDelegatesToAPI(t *testing.T) {
	api := &fakeAPI{dlqReplayResponse: evalapi.ReplayDLQItemResponse{ItemID: "item-1", MessagePublished: true}}
	svc := New(api, nil, nil, time.Second, time.Minute)

	got, err := svc.ReplayEvalItemDLQ(context.Background(), evalapi.ReplayDLQItemRequest{ItemID: "item-1"})
	if err != nil {
		t.Fatalf("ReplayEvalItemDLQ error: %v", err)
	}
	if api.dlqReplayRequest.ItemID != "item-1" || !got.MessagePublished {
		t.Fatalf("request=%#v response=%#v", api.dlqReplayRequest, got)
	}
}

func TestWaitForRunReturnsCompletedRun(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{detailsByID: map[string][]evalapi.EvaluationDetail{
		"eval-1": {
			{ID: "eval-1", Status: "running"},
			{ID: "eval-1", Status: "completed"},
		},
	}}
	svc := newTestService(api)

	got, err := svc.WaitForRun(ctx, "eval-1")
	if err != nil {
		t.Fatalf("WaitForRun error: %v", err)
	}
	if got.Run.Status != "completed" || got.TimedOut {
		t.Fatalf("WaitResult = %#v", got)
	}
}

func TestWaitForRunReturnsCompletedWithFailuresRun(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{detailsByID: map[string][]evalapi.EvaluationDetail{
		"eval-1": {
			{ID: "eval-1", Status: "running"},
			{ID: "eval-1", Status: "completed_with_failures"},
		},
	}}
	svc := newTestService(api)

	got, err := svc.WaitForRun(ctx, "eval-1")
	if err != nil {
		t.Fatalf("WaitForRun error: %v", err)
	}
	if got.Run.Status != "completed_with_failures" || got.TimedOut {
		t.Fatalf("WaitResult = %#v", got)
	}
}

func TestWaitForRunBacksOffAfterRateLimit(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{
		getEvaluationErrorsByID: map[string][]error{
			"eval-1": {
				&evalapi.HTTPError{
					Method:     "GET",
					Path:       "/evaluations/eval-1",
					StatusCode: 429,
					RetryAfter: 10 * time.Millisecond,
				},
			},
		},
		detailsByID: map[string][]evalapi.EvaluationDetail{
			"eval-1": {{ID: "eval-1", Status: "completed"}},
		},
	}
	svc := New(api, fakeIngestion{}, fakeFixtures{}, time.Hour, time.Second, 20*time.Millisecond)

	start := time.Now()
	got, err := svc.WaitForRun(ctx, "eval-1")
	if err != nil {
		t.Fatalf("WaitForRun error: %v", err)
	}
	if got.Run.Status != "completed" || got.TimedOut {
		t.Fatalf("WaitResult = %#v", got)
	}
	if elapsed := time.Since(start); elapsed < 10*time.Millisecond {
		t.Fatalf("WaitForRun returned before retry-after elapsed: %s", elapsed)
	}
}

func TestWaitForRunUsesCappedBackoffWhenRetryAfterMissing(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{
		getEvaluationErrorsByID: map[string][]error{
			"eval-1": {
				&evalapi.HTTPError{StatusCode: 429},
				&evalapi.HTTPError{StatusCode: 429},
			},
		},
		detailsByID: map[string][]evalapi.EvaluationDetail{
			"eval-1": {{ID: "eval-1", Status: "completed"}},
		},
	}
	svc := New(api, fakeIngestion{}, fakeFixtures{}, time.Hour, time.Second, time.Millisecond)

	got, err := svc.WaitForRun(ctx, "eval-1")
	if err != nil {
		t.Fatalf("WaitForRun error: %v", err)
	}
	if got.Run.Status != "completed" {
		t.Fatalf("WaitResult = %#v", got)
	}
}

func TestWaitForRunTimesOutWithLatestStatus(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{detailsByID: map[string][]evalapi.EvaluationDetail{
		"eval-1": {
			{ID: "eval-1", Status: "queued"},
			{ID: "eval-1", Status: "running"},
		},
	}}
	svc := newTestServiceWithTiming(api, time.Millisecond, 3*time.Millisecond)

	got, err := svc.WaitForRun(ctx, "eval-1")
	if err == nil {
		t.Fatal("WaitForRun error = nil, want timeout")
	}
	if !got.TimedOut || got.Run.Status != "running" {
		t.Fatalf("WaitResult = %#v", got)
	}
}

func TestWaitForRunTimeoutIncludesLatestRunMetadata(t *testing.T) {
	ctx := context.Background()
	collection := "documents"
	api := &fakeAPI{detailsByID: map[string][]evalapi.EvaluationDetail{
		"eval-1": {{
			ID:         "eval-1",
			Status:     "running",
			Collection: &collection,
			CreatedAt:  "2026-05-17T01:10:15Z",
		}},
	}}
	svc := newTestServiceWithTiming(api, time.Millisecond, 3*time.Millisecond)

	got, err := svc.WaitForRun(ctx, "eval-1")
	if err == nil {
		t.Fatal("WaitForRun error = nil, want timeout")
	}
	if !got.TimedOut || got.Run.Status != "running" {
		t.Fatalf("WaitResult = %#v", got)
	}
	for _, want := range []string{
		`evaluation "eval-1"`,
		`latest status "running"`,
		`created_at "2026-05-17T01:10:15Z"`,
		`collection "documents"`,
		"eval API run may still finish after the MCP wait timeout",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("WaitForRun error = %q, want substring %q", err.Error(), want)
		}
	}
}

func TestRunEvidenceSummarizesStaleRunningRun(t *testing.T) {
	ctx := context.Background()
	collection := "documents"
	api := &fakeAPI{detailsByID: map[string][]evalapi.EvaluationDetail{
		"eval-1": {{
			ID:         "eval-1",
			Status:     "running",
			Collection: &collection,
			CreatedAt:  time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339),
			Config: map[string]any{
				"chat":       map[string]any{"llm_model": "qwen"},
				"collection": map[string]any{"name": "documents"},
			},
		}},
	}}
	svc := New(api, fakeIngestion{}, fakeFixtures{}, time.Millisecond, time.Hour)

	got, err := svc.RunEvidence(ctx, "eval-1")
	if err != nil {
		t.Fatalf("RunEvidence error: %v", err)
	}
	if got.EvalID != "eval-1" || got.Status != "running" || !got.StaleRunning {
		t.Fatalf("RunEvidence = %#v", got)
	}
	if len(got.NextSteps) == 0 || !strings.Contains(got.NextSteps[0], "investigate_eval_run") {
		t.Fatalf("NextSteps = %#v", got.NextSteps)
	}
	if got.Config["chat"] == nil {
		t.Fatalf("Config = %#v", got.Config)
	}
}

func TestRunEvidenceNextStepsCancelled(t *testing.T) {
	evidence := RunEvidence{Status: "cancelled"}

	got := runEvidenceNextSteps(evidence)

	if len(got) != 1 || !strings.Contains(got[0], "cancelled") {
		t.Fatalf("cancelled next steps = %#v", got)
	}
}

func TestRunEvidenceNextStepsStaleRetryable(t *testing.T) {
	evidence := RunEvidence{
		Status:       "running",
		StaleRunning: true,
		ItemCounts: map[string]int{
			"stale":     2,
			"retryable": 2,
		},
	}

	got := runEvidenceNextSteps(evidence)

	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "republish") {
		t.Fatalf("retryable stale next steps should mention republish: %#v", got)
	}
}

func TestRunEvidenceNextStepsStaleExhausted(t *testing.T) {
	evidence := RunEvidence{
		Status:       "running",
		StaleRunning: true,
		ItemCounts: map[string]int{
			"stale":     2,
			"retryable": 0,
		},
	}

	got := runEvidenceNextSteps(evidence)

	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "exhausted") {
		t.Fatalf("exhausted stale next steps should mention exhausted work: %#v", got)
	}
}

func TestRunEvidenceIncludesPartialItemCounts(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{detailsByID: map[string][]evalapi.EvaluationDetail{
		"eval-1": {{
			ID:         "eval-1",
			Status:     "completed_with_failures",
			ItemCounts: map[string]int{"completed": 3, "failed": 1, "total": 4},
			Results:    []evalapi.QueryResult{queryResult("q", 0.5)},
		}},
	}}
	svc := newTestService(api)

	got, err := svc.RunEvidence(ctx, "eval-1")
	if err != nil {
		t.Fatalf("RunEvidence error: %v", err)
	}
	if got.ItemCounts["failed"] != 1 || got.ItemCounts["total"] != 4 {
		t.Fatalf("ItemCounts = %#v", got.ItemCounts)
	}
	if len(got.NextSteps) == 0 || !strings.Contains(got.NextSteps[0], "partial successful results") {
		t.Fatalf("NextSteps = %#v", got.NextSteps)
	}
}

func TestWaitForRunCancelsGetEvaluationWithWaitTimeout(t *testing.T) {
	parentCtx, cancelParent := context.WithCancel(context.Background())
	defer cancelParent()

	api := &blockingGetEvaluationAPI{entered: make(chan struct{})}
	svc := newTestServiceWithTiming(api, time.Hour, 5*time.Millisecond)

	type waitOutcome struct {
		result WaitResult
		err    error
	}
	outcomeCh := make(chan waitOutcome, 1)
	go func() {
		result, err := svc.WaitForRun(parentCtx, "eval-1")
		outcomeCh <- waitOutcome{result: result, err: err}
	}()

	select {
	case <-api.entered:
	case <-time.After(50 * time.Millisecond):
		t.Fatal("GetEvaluation was not called")
	}

	select {
	case outcome := <-outcomeCh:
		if outcome.err == nil {
			t.Fatal("WaitForRun error = nil, want timeout")
		}
		if !outcome.result.TimedOut {
			t.Fatalf("WaitResult = %#v, want TimedOut", outcome.result)
		}
		if !strings.Contains(outcome.err.Error(), `wait for evaluation "eval-1" timed out after 5ms with latest status ""`) {
			t.Fatalf("WaitForRun error = %v, want helpful timeout", outcome.err)
		}
	case <-time.After(100 * time.Millisecond):
		cancelParent()
		t.Fatal("WaitForRun did not cancel GetEvaluation using waitTimeout")
	}

	select {
	case <-parentCtx.Done():
		t.Fatal("parent context was canceled")
	default:
	}
}

func TestWaitForRunParentDeadlineIsNotServiceTimeout(t *testing.T) {
	parentCtx, cancelParent := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancelParent()

	api := &blockingGetEvaluationAPI{entered: make(chan struct{})}
	svc := newTestServiceWithTiming(api, time.Hour, time.Minute)

	type waitOutcome struct {
		result WaitResult
		err    error
	}
	outcomeCh := make(chan waitOutcome, 1)
	go func() {
		result, err := svc.WaitForRun(parentCtx, "eval-1")
		outcomeCh <- waitOutcome{result: result, err: err}
	}()

	select {
	case <-api.entered:
	case <-time.After(50 * time.Millisecond):
		t.Fatal("GetEvaluation was not called")
	}

	select {
	case outcome := <-outcomeCh:
		if !errors.Is(outcome.err, context.DeadlineExceeded) {
			t.Fatalf("WaitForRun error = %v, want parent deadline", outcome.err)
		}
		if outcome.result.TimedOut {
			t.Fatalf("WaitResult = %#v, want TimedOut=false", outcome.result)
		}
		if strings.Contains(outcome.err.Error(), "wait for evaluation") {
			t.Fatalf("WaitForRun error = %v, want parent context error", outcome.err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("WaitForRun did not return after parent deadline")
	}
}

func TestWorstCasesSortsAscendingByMetric(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{detailsByID: map[string][]evalapi.EvaluationDetail{
		"eval-1": {{
			ID:     "eval-1",
			Status: "completed",
			Results: []evalapi.QueryResult{
				queryResult("highest", 0.90),
				queryResult("lowest", 0.10),
				queryResult("middle", 0.40),
			},
		}},
	}}
	svc := newTestService(api)

	got, err := svc.WorstCases(ctx, WorstCasesInput{
		EvalID: "eval-1",
		Metric: "context_precision",
		Limit:  2,
	})
	if err != nil {
		t.Fatalf("WorstCases error: %v", err)
	}
	if got.Metric != "context_precision" {
		t.Fatalf("Metric = %q", got.Metric)
	}
	gotQueries := []string{got.Cases[0].Result.Query, got.Cases[1].Result.Query}
	if !reflect.DeepEqual(gotQueries, []string{"lowest", "middle"}) {
		t.Fatalf("queries = %#v", gotQueries)
	}
}

func TestTriageRAGRegressionCallsTriageAPI(t *testing.T) {
	ctx := context.Background()
	triage := &fakeTriageAPI{result: map[string]any{"status": "completed"}}
	svc := New(nil, nil, nil, time.Second, time.Minute)
	svc.triage = triage

	got, err := svc.TriageRAGRegression(ctx, TriageInput{
		EvalID: "eval-1",
		Metric: "context_precision",
		Limit:  5,
	})
	if err != nil {
		t.Fatalf("TriageRAGRegression error: %v", err)
	}
	if triage.input.EvalID != "eval-1" {
		t.Fatalf("triage eval id = %q, want eval-1", triage.input.EvalID)
	}
	if got["status"] != "completed" {
		t.Fatalf("triage result = %#v", got)
	}
}

func TestTriageRAGRegressionDefaultsMetricAndAllowsOmittedLimit(t *testing.T) {
	triage := &fakeTriageAPI{result: map[string]any{"status": "completed"}}
	svc := New(nil, nil, nil, time.Second, time.Minute).WithTriageAPI(triage)

	_, err := svc.TriageRAGRegression(context.Background(), TriageInput{
		EvalID: "eval-1",
	})
	if err != nil {
		t.Fatalf("TriageRAGRegression error: %v", err)
	}

	if triage.input.Metric != DefaultFocusMetric {
		t.Fatalf("metric = %q, want %q", triage.input.Metric, DefaultFocusMetric)
	}
	if triage.input.Limit != 0 {
		t.Fatalf("limit = %d, want omitted zero value", triage.input.Limit)
	}
}

func TestTriageRAGRegressionRejectsInvalidMetric(t *testing.T) {
	triage := &fakeTriageAPI{result: map[string]any{"status": "completed"}}
	svc := New(nil, nil, nil, time.Second, time.Minute).WithTriageAPI(triage)

	_, err := svc.TriageRAGRegression(context.Background(), TriageInput{
		EvalID: "eval-1",
		Metric: "latency",
	})

	if err == nil || !strings.Contains(err.Error(), `unsupported metric "latency"`) {
		t.Fatalf("error = %v", err)
	}
	if triage.input.EvalID != "" {
		t.Fatalf("triage API was called: %#v", triage.input)
	}
}

func TestTriageRAGRegressionRejectsInvalidLimit(t *testing.T) {
	triage := &fakeTriageAPI{result: map[string]any{"status": "completed"}}
	svc := New(nil, nil, nil, time.Second, time.Minute).WithTriageAPI(triage)

	_, err := svc.TriageRAGRegression(context.Background(), TriageInput{
		EvalID: "eval-1",
		Limit:  21,
	})

	if err == nil || !strings.Contains(err.Error(), "limit must be between 1 and 20 when provided") {
		t.Fatalf("error = %v", err)
	}
	if triage.input.EvalID != "" {
		t.Fatalf("triage API was called: %#v", triage.input)
	}
}

func TestCompareResolvesExperimentLabels(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{}
	api.experiments = map[string]evalapi.Experiment{
		"exp-9": {
			ID: "exp-9",
			Runs: []evalapi.ExperimentRun{
				{Label: "baseline", EvaluationID: "eval-base"},
				{Label: "candidate", EvaluationID: "eval-candidate"},
			},
		},
	}
	api.detailsByID = map[string][]evalapi.EvaluationDetail{
		"eval-base":      {{ID: "eval-base", Status: "completed"}},
		"eval-candidate": {{ID: "eval-candidate", Status: "completed"}},
	}
	svc := newTestService(api)

	if _, err := svc.Compare(ctx, CompareInput{ExperimentID: "exp-9", Labels: []string{"baseline", "missing"}}); err == nil || !strings.Contains(err.Error(), "known labels: baseline, candidate") {
		t.Fatalf("Compare missing label error = %v", err)
	}

	if _, err := svc.Compare(ctx, CompareInput{ExperimentID: "exp-9", Labels: []string{"baseline", "candidate"}}); err != nil {
		t.Fatalf("Compare error: %v", err)
	}
	if !reflect.DeepEqual(api.compareIDs, []string{"eval-base", "eval-candidate"}) {
		t.Fatalf("CompareEvaluations IDs = %#v", api.compareIDs)
	}
}

func TestCompareAllowsCompletedWithFailures(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{detailsByID: map[string][]evalapi.EvaluationDetail{
		"eval-base":      {{ID: "eval-base", Status: "completed"}},
		"eval-candidate": {{ID: "eval-candidate", Status: "completed_with_failures"}},
	}}
	svc := newTestService(api)

	if _, err := svc.Compare(ctx, CompareInput{EvalIDs: []string{"eval-base", "eval-candidate"}}); err != nil {
		t.Fatalf("Compare error: %v", err)
	}
	if !reflect.DeepEqual(api.compareIDs, []string{"eval-base", "eval-candidate"}) {
		t.Fatalf("CompareEvaluations IDs = %#v", api.compareIDs)
	}
}

func TestCompareRequiresTwoToFiveIDs(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{}
	svc := newTestService(api)

	if _, err := svc.Compare(ctx, CompareInput{EvalIDs: []string{"eval-1"}}); err == nil || !strings.Contains(err.Error(), "requires 2 to 5 eval IDs") {
		t.Fatalf("Compare one ID error = %v", err)
	}
	if _, err := svc.Compare(ctx, CompareInput{EvalIDs: []string{"eval-1", "eval-2", "eval-3", "eval-4", "eval-5", "eval-6"}}); err == nil || !strings.Contains(err.Error(), "requires 2 to 5 eval IDs") {
		t.Fatalf("Compare six IDs error = %v", err)
	}
	if len(api.compareIDs) != 0 {
		t.Fatalf("CompareEvaluations IDs = %#v, want no call", api.compareIDs)
	}
}

func TestSummarizeExperimentReturnsBaselineCandidateAndWorstCases(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{detailsByID: map[string][]evalapi.EvaluationDetail{
		"eval-base": {{
			ID:     "eval-base",
			Status: "completed",
			Results: []evalapi.QueryResult{
				queryResult("base low", 0.20),
				queryResult("base high", 0.80),
			},
		}},
		"eval-candidate": {{
			ID:     "eval-candidate",
			Status: "completed",
			Results: []evalapi.QueryResult{
				queryResult("candidate low", 0.10),
				queryResult("candidate high", 0.90),
			},
		}},
	}}
	api.experiments = map[string]evalapi.Experiment{
		"exp-3": {
			ID:          "exp-3",
			FocusMetric: "context_precision",
			Runs: []evalapi.ExperimentRun{
				{Label: "baseline", EvaluationID: "eval-base"},
				{Label: "candidate", EvaluationID: "eval-candidate"},
			},
		},
	}
	svc := newTestService(api)

	got, err := svc.SummarizeExperiment(ctx, "exp-3")
	if err != nil {
		t.Fatalf("SummarizeExperiment error: %v", err)
	}
	if got.Baseline == nil || got.Baseline.Label != "baseline" || got.Baseline.Run.ID != "eval-base" {
		t.Fatalf("Baseline = %#v", got.Baseline)
	}
	if len(got.Candidates) != 1 || got.Candidates[0].Label != "candidate" || got.Candidates[0].Run.ID != "eval-candidate" {
		t.Fatalf("Candidates = %#v", got.Candidates)
	}
	if !reflect.DeepEqual(api.compareIDs, []string{"eval-base", "eval-candidate"}) {
		t.Fatalf("CompareEvaluations IDs = %#v", api.compareIDs)
	}
	if len(got.WorstCases) != 2 {
		t.Fatalf("WorstCases length = %d, want 2", len(got.WorstCases))
	}
	if got.WorstCases[0].Label != "baseline" || got.WorstCases[0].Cases[0].Result.Query != "base low" {
		t.Fatalf("WorstCases[0] = %#v", got.WorstCases[0])
	}
}

func TestSummarizeExperimentComparesFirstFiveRuns(t *testing.T) {
	ctx := context.Background()
	api := &fakeAPI{detailsByID: make(map[string][]evalapi.EvaluationDetail)}
	exp := evalapi.Experiment{
		ID:          "exp-4",
		FocusMetric: "context_precision",
	}
	for i := 1; i <= 6; i++ {
		label := fmt.Sprintf("run-%d", i)
		evalID := fmt.Sprintf("eval-%d", i)
		exp.Runs = append(exp.Runs, evalapi.ExperimentRun{Label: label, EvaluationID: evalID})
		api.detailsByID[evalID] = []evalapi.EvaluationDetail{{
			ID:      evalID,
			Status:  "completed",
			Results: []evalapi.QueryResult{queryResult(label+" low", float64(i)/10)},
		}}
	}
	api.experiments = map[string]evalapi.Experiment{"exp-4": exp}
	svc := newTestService(api)

	got, err := svc.SummarizeExperiment(ctx, "exp-4")
	if err != nil {
		t.Fatalf("SummarizeExperiment error: %v", err)
	}
	if got.Baseline == nil || len(got.Candidates) != 5 || len(got.WorstCases) != 6 {
		t.Fatalf("summary = %#v", got)
	}
	if !reflect.DeepEqual(api.compareIDs, []string{"eval-1", "eval-2", "eval-3", "eval-4", "eval-5"}) {
		t.Fatalf("CompareEvaluations IDs = %#v", api.compareIDs)
	}
}

func queryResult(query string, contextPrecision float64) evalapi.QueryResult {
	return evalapi.QueryResult{
		Query: query,
		Scores: evalapi.Scores{
			ContextPrecision: &contextPrecision,
		},
	}
}

type fakeAPI struct {
	datasets                 []evalapi.Dataset
	createDatasetRequest     evalapi.CreateDatasetRequest
	createdDatasetID         string
	startResponse            evalapi.StartEvaluationResponse
	startRequests            []evalapi.StartEvaluationRequest
	readinessRequest         evalapi.RAGReadinessRequest
	readinessResponse        evalapi.RAGReadinessResponse
	detailsByID              map[string][]evalapi.EvaluationDetail
	getEvaluationErrorsByID  map[string][]error
	compareIDs               []string
	comparison               evalapi.Comparison
	experiments              map[string]evalapi.Experiment
	createExperimentRequests []evalapi.CreateExperimentRequest
	updateExperimentID       string
	updateExperimentRequest  evalapi.UpdateExperimentRequest
	dlqListRequestLimit      int
	dlqListResponse          evalapi.DLQListResponse
	dlqReplayRequest         evalapi.ReplayDLQItemRequest
	dlqReplayResponse        evalapi.ReplayDLQItemResponse
}

func (f *fakeAPI) ListDatasets(context.Context) ([]evalapi.Dataset, error) {
	return f.datasets, nil
}

func (f *fakeAPI) CreateDataset(_ context.Context, req evalapi.CreateDatasetRequest) (evalapi.CreateDatasetResponse, error) {
	f.createDatasetRequest = req
	id := f.createdDatasetID
	if id == "" {
		id = "ds-created"
	}
	return evalapi.CreateDatasetResponse{ID: id}, nil
}

func (f *fakeAPI) StartEvaluation(_ context.Context, in evalapi.StartEvaluationRequest) (evalapi.StartEvaluationResponse, error) {
	f.startRequests = append(f.startRequests, in)
	return f.startResponse, nil
}

func (f *fakeAPI) CheckRAGReadiness(_ context.Context, in evalapi.RAGReadinessRequest) (evalapi.RAGReadinessResponse, error) {
	f.readinessRequest = in
	return f.readinessResponse, nil
}

func (f *fakeAPI) GetEvaluation(_ context.Context, id string) (evalapi.EvaluationDetail, error) {
	errs := f.getEvaluationErrorsByID[id]
	if len(errs) > 0 {
		err := errs[0]
		f.getEvaluationErrorsByID[id] = errs[1:]
		return evalapi.EvaluationDetail{}, err
	}
	details := f.detailsByID[id]
	if len(details) == 0 {
		return evalapi.EvaluationDetail{}, errors.New("missing eval detail")
	}
	if len(details) == 1 {
		return details[0], nil
	}
	got := details[0]
	f.detailsByID[id] = details[1:]
	return got, nil
}

func (f *fakeAPI) CompareEvaluations(_ context.Context, ids []string) (evalapi.Comparison, error) {
	f.compareIDs = append([]string(nil), ids...)
	if f.comparison.Runs != nil || f.comparison.Deltas != nil {
		return f.comparison, nil
	}
	return evalapi.Comparison{Runs: nil, Deltas: map[string][]float64{}}, nil
}

func (f *fakeAPI) CreateExperiment(_ context.Context, in evalapi.CreateExperimentRequest) (evalapi.Experiment, error) {
	f.createExperimentRequests = append(f.createExperimentRequests, in)
	return evalapi.Experiment{
		ID:             "exp-1",
		Name:           in.Name,
		DatasetID:      in.DatasetID,
		Collection:     in.Collection,
		BaselineEvalID: nil,
		FocusMetric:    in.FocusMetric,
		Status:         in.Status,
	}, nil
}

func (f *fakeAPI) ListExperiments(context.Context) ([]evalapi.Experiment, error) {
	experiments := make([]evalapi.Experiment, 0, len(f.experiments))
	for _, exp := range f.experiments {
		experiments = append(experiments, exp)
	}
	return experiments, nil
}

func (f *fakeAPI) GetExperiment(_ context.Context, id string) (evalapi.Experiment, error) {
	exp, ok := f.experiments[id]
	if !ok {
		return evalapi.Experiment{}, errors.New("missing experiment")
	}
	return exp, nil
}

func (f *fakeAPI) AttachExperimentRun(_ context.Context, id string, in evalapi.AttachExperimentRunRequest) (evalapi.Experiment, error) {
	exp := f.experiments[id]
	exp.Runs = append(exp.Runs, evalapi.ExperimentRun{
		EvaluationID: in.EvaluationID,
		Label:        in.Label,
	})
	f.experiments[id] = exp
	return exp, nil
}

func (f *fakeAPI) UpdateExperiment(_ context.Context, id string, in evalapi.UpdateExperimentRequest) (evalapi.Experiment, error) {
	f.updateExperimentID = id
	f.updateExperimentRequest = in
	if f.experiments == nil {
		return evalapi.Experiment{ID: id, Status: in.Status, Decision: in.Decision, Conclusion: in.Conclusion, Evidence: in.Evidence}, nil
	}
	exp := f.experiments[id]
	exp.Status = in.Status
	exp.Decision = in.Decision
	exp.Conclusion = in.Conclusion
	exp.Evidence = in.Evidence
	f.experiments[id] = exp
	return exp, nil
}

func (f *fakeAPI) ListEvalItemDLQ(_ context.Context, limit int) (evalapi.DLQListResponse, error) {
	f.dlqListRequestLimit = limit
	return f.dlqListResponse, nil
}

func (f *fakeAPI) ReplayEvalItemDLQ(_ context.Context, in evalapi.ReplayDLQItemRequest) (evalapi.ReplayDLQItemResponse, error) {
	f.dlqReplayRequest = in
	return f.dlqReplayResponse, nil
}

type fakeIngestion struct {
	collections []ingestionapi.Collection
	configs     map[string]map[string]any
}

func (f fakeIngestion) ListCollections(context.Context) ([]ingestionapi.Collection, error) {
	if f.collections != nil {
		return f.collections, nil
	}
	return []ingestionapi.Collection{{Name: "documents"}, {Name: "kb"}}, nil
}

func (f fakeIngestion) GetCollectionConfig(_ context.Context, name string) (map[string]any, error) {
	return f.configs[name], nil
}

func (f fakeIngestion) UploadPDF(context.Context, string, string, []byte) (ingestionapi.IngestResponse, error) {
	return ingestionapi.IngestResponse{}, nil
}

func (f fakeIngestion) PutCollectionManifest(context.Context, string, map[string]any) error {
	return nil
}

func (f fakeIngestion) GetCollectionManifest(context.Context, string) (map[string]any, error) {
	return nil, &ingestionapi.HTTPError{Method: http.MethodGet, Path: "/manifest", StatusCode: http.StatusNotFound}
}

func (f fakeIngestion) DeleteCollection(context.Context, string) error {
	return nil
}

type fakeFixtures struct {
	fixtures []fixturecatalog.Fixture
	fixture  fixturecatalog.Fixture
}

func (f fakeFixtures) List() ([]fixturecatalog.Fixture, error)     { return f.fixtures, nil }
func (f fakeFixtures) Load(string) (fixturecatalog.Fixture, error) { return f.fixture, nil }

type fakeCorpusFixtures struct {
	fixtures []corpusfixture.Fixture
	fixture  corpusfixture.Fixture
}

func (f fakeCorpusFixtures) List() ([]corpusfixture.Fixture, error) {
	return f.fixtures, nil
}

func (f fakeCorpusFixtures) Load(string) (corpusfixture.Fixture, error) {
	return f.fixture, nil
}

type recordingCorpusIngestion struct {
	collections      []ingestionapi.Collection
	manifest         map[string]any
	manifestResponse map[string]any
	uploadResponse   ingestionapi.IngestResponse
	deleteErr        error
	deleteCalled     bool
	uploadCollection string
	uploadFilename   string
}

func (r *recordingCorpusIngestion) ListCollections(context.Context) ([]ingestionapi.Collection, error) {
	return r.collections, nil
}

func (r *recordingCorpusIngestion) GetCollectionConfig(context.Context, string) (map[string]any, error) {
	return map[string]any{}, nil
}

func (r *recordingCorpusIngestion) UploadPDF(_ context.Context, collection, filename string, _ []byte) (ingestionapi.IngestResponse, error) {
	r.uploadCollection = collection
	r.uploadFilename = filename
	return r.uploadResponse, nil
}

func (r *recordingCorpusIngestion) PutCollectionManifest(_ context.Context, _ string, manifest map[string]any) error {
	r.manifest = manifest
	return nil
}

func (r *recordingCorpusIngestion) GetCollectionManifest(context.Context, string) (map[string]any, error) {
	if r.manifestResponse != nil {
		return r.manifestResponse, nil
	}
	return nil, &ingestionapi.HTTPError{Method: http.MethodGet, Path: "/manifest", StatusCode: http.StatusNotFound}
}

func (r *recordingCorpusIngestion) DeleteCollection(context.Context, string) error {
	r.deleteCalled = true
	return r.deleteErr
}

type fakeTriageAPI struct {
	input  TriageInput
	result map[string]any
}

func (f *fakeTriageAPI) TriageRAGRegression(_ context.Context, in TriageInput) (map[string]any, error) {
	f.input = in
	return f.result, nil
}

func newTestService(api API) *Service {
	return newTestServiceWithTiming(api, time.Millisecond, time.Second)
}

func newTestServiceWithTiming(api API, pollInterval, waitTimeout time.Duration) *Service {
	return New(api, fakeIngestion{}, fakeFixtures{}, pollInterval, waitTimeout)
}

type blockingGetEvaluationAPI struct {
	entered chan struct{}
}

func (b *blockingGetEvaluationAPI) ListDatasets(context.Context) ([]evalapi.Dataset, error) {
	return nil, nil
}

func (b *blockingGetEvaluationAPI) CreateDataset(context.Context, evalapi.CreateDatasetRequest) (evalapi.CreateDatasetResponse, error) {
	return evalapi.CreateDatasetResponse{}, nil
}

func (b *blockingGetEvaluationAPI) CheckRAGReadiness(context.Context, evalapi.RAGReadinessRequest) (evalapi.RAGReadinessResponse, error) {
	return evalapi.RAGReadinessResponse{Status: "ready"}, nil
}

func (b *blockingGetEvaluationAPI) StartEvaluation(context.Context, evalapi.StartEvaluationRequest) (evalapi.StartEvaluationResponse, error) {
	return evalapi.StartEvaluationResponse{}, nil
}

func (b *blockingGetEvaluationAPI) GetEvaluation(ctx context.Context, _ string) (evalapi.EvaluationDetail, error) {
	close(b.entered)
	<-ctx.Done()
	return evalapi.EvaluationDetail{}, ctx.Err()
}

func (b *blockingGetEvaluationAPI) CompareEvaluations(context.Context, []string) (evalapi.Comparison, error) {
	return evalapi.Comparison{}, nil
}

func (b *blockingGetEvaluationAPI) ListEvalItemDLQ(context.Context, int) (evalapi.DLQListResponse, error) {
	return evalapi.DLQListResponse{}, nil
}

func (b *blockingGetEvaluationAPI) ReplayEvalItemDLQ(context.Context, evalapi.ReplayDLQItemRequest) (evalapi.ReplayDLQItemResponse, error) {
	return evalapi.ReplayDLQItemResponse{}, nil
}

func (b *blockingGetEvaluationAPI) CreateExperiment(context.Context, evalapi.CreateExperimentRequest) (evalapi.Experiment, error) {
	return evalapi.Experiment{}, nil
}

func (b *blockingGetEvaluationAPI) ListExperiments(context.Context) ([]evalapi.Experiment, error) {
	return nil, nil
}

func (b *blockingGetEvaluationAPI) GetExperiment(context.Context, string) (evalapi.Experiment, error) {
	return evalapi.Experiment{}, nil
}

func (b *blockingGetEvaluationAPI) AttachExperimentRun(context.Context, string, evalapi.AttachExperimentRunRequest) (evalapi.Experiment, error) {
	return evalapi.Experiment{}, nil
}

func (b *blockingGetEvaluationAPI) UpdateExperiment(context.Context, string, evalapi.UpdateExperimentRequest) (evalapi.Experiment, error) {
	return evalapi.Experiment{}, nil
}
