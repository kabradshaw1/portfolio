package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kabradshaw1/portfolio/go/coding-exercises-mcp-service/internal/coding"
	"github.com/kabradshaw1/portfolio/go/coding-exercises-mcp-service/internal/store"
)

type fakeCoding struct {
	submittedQuestionID int64
	submittedReview     string
	feedback            coding.FeedbackInput
	nextFilter          store.QuestionFilter
	nextQuestionCalls   int
}

func (f *fakeCoding) ImportMaterial(context.Context) (coding.ImportResult, error) {
	return coding.ImportResult{ImportedQuestions: 12}, nil
}

func (f *fakeCoding) ListTopics(context.Context) ([]store.Topic, error) {
	return []store.Topic{{Name: "Go", QuestionCount: 2, AnsweredCount: 1}}, nil
}

func (f *fakeCoding) GetNextQuestion(_ context.Context, filter store.QuestionFilter) (store.Question, error) {
	f.nextFilter = filter
	f.nextQuestionCalls++
	return store.Question{ID: 3, Topic: "Go", Prompt: "How do maps work?", ExpectedAnswer: "Use synchronization.", Priority: 10, Tier: filter.Tier}, nil
}

func (f *fakeCoding) SubmitAnswer(_ context.Context, questionID int64, reviewSummary string) (coding.AnswerReview, error) {
	f.submittedQuestionID = questionID
	f.submittedReview = reviewSummary
	return coding.AnswerReview{AttemptID: 9, QuestionID: questionID, ReviewSummary: reviewSummary, ExpectedAnswer: "Use synchronization."}, nil
}

func (f *fakeCoding) RecordFeedback(_ context.Context, input coding.FeedbackInput) error {
	f.feedback = input
	return nil
}

func (f *fakeCoding) ProgressSummary(context.Context) (store.ProgressSummary, error) {
	return store.ProgressSummary{TotalAttempts: 1, AverageScore: 2}, nil
}

func (f *fakeCoding) UpdateExpectedAnswer(context.Context, int64, string) error {
	return nil
}

func TestSubmitCodingReviewHandlerDecodesReviewSummaryArguments(t *testing.T) {
	fake := &fakeCoding{}
	handler := submitCodingReviewHandler(fake)
	result, err := handler(context.Background(), callReq(map[string]any{
		"question_id":    float64(3),
		"review_summary": "Reviewed worker_pool.go and worker_pool_test.go.",
	}))
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handler returned MCP error: %#v", result)
	}
	if fake.submittedQuestionID != 3 || fake.submittedReview != "Reviewed worker_pool.go and worker_pool_test.go." {
		t.Fatalf("unexpected submit args: id=%d review=%q", fake.submittedQuestionID, fake.submittedReview)
	}

	var payload coding.AnswerReview
	unmarshalTextResult(t, result, &payload)
	if payload.AttemptID != 9 || payload.ExpectedAnswer == "" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestRecordFeedbackHandlerValidatesScore(t *testing.T) {
	fake := &fakeCoding{}
	handler := recordFeedbackHandler(fake)
	result, err := handler(context.Background(), callReq(map[string]any{
		"attempt_id": float64(9),
		"score":      float64(4),
	}))
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected MCP tool error for invalid score")
	}
}

func TestSubmitCodingReviewAndPrepareNextRecordsPriorFeedbackAndReturnsReviewWithNextQuestion(t *testing.T) {
	fake := &fakeCoding{}
	handler := submitCodingReviewAndPrepareNextHandler(fake)
	result, err := handler(context.Background(), callReq(map[string]any{
		"question_id":    float64(2),
		"review_summary": "Reviewed worker_pool.go; go test ./... passed.",
		"tier":           float64(1),
		"category":       "coding",
		"previous_feedback": map[string]any{
			"attempt_id":        float64(8),
			"score":             float64(1),
			"missing_points":    "mutex tradeoffs",
			"inaccurate_points": "",
			"suggested_answer":  "Use a mutex or sharding.",
		},
	}))
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handler returned MCP error: %#v", result)
	}
	if fake.feedback.AttemptID != 8 || fake.feedback.Score != 1 {
		t.Fatalf("expected previous feedback to be recorded, got %#v", fake.feedback)
	}
	if fake.submittedQuestionID != 2 || fake.submittedReview != "Reviewed worker_pool.go; go test ./... passed." {
		t.Fatalf("unexpected submit args: id=%d review=%q", fake.submittedQuestionID, fake.submittedReview)
	}
	if fake.nextQuestionCalls != 1 {
		t.Fatalf("expected next question to be loaded once, got %d", fake.nextQuestionCalls)
	}
	if fake.nextFilter.Tier != 1 {
		t.Fatalf("expected tier filter to be passed, got %#v", fake.nextFilter)
	}
	if fake.nextFilter.Category != "coding" {
		t.Fatalf("expected category filter to be passed, got %#v", fake.nextFilter)
	}

	var payload codingTurn
	unmarshalTextResult(t, result, &payload)
	if payload.Review.AttemptID != 9 || payload.Review.ExpectedAnswer == "" {
		t.Fatalf("unexpected review payload: %#v", payload.Review)
	}
	if payload.NextExercise.ID != 3 {
		t.Fatalf("unexpected next exercise: %#v", payload.NextExercise)
	}
	if payload.Tier != 1 {
		t.Fatalf("expected turn payload tier, got %#v", payload)
	}
	if !payload.PreviousFeedbackRecorded {
		t.Fatalf("expected previous feedback recorded flag")
	}
}

func TestStartCodingExerciseSessionReturnsWorkflowAndCurrentState(t *testing.T) {
	fake := &fakeCoding{}
	handler := startCodingExerciseSessionHandler(fake)
	result, err := handler(context.Background(), callReq(map[string]any{
		"tier":     float64(1),
		"category": "coding",
	}))
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handler returned MCP error: %#v", result)
	}

	var payload codingSession
	unmarshalTextResult(t, result, &payload)
	if payload.NextExercise.ID != 3 {
		t.Fatalf("expected next exercise in session payload, got %#v", payload.NextExercise)
	}
	if payload.Tier != 1 || fake.nextFilter.Tier != 1 {
		t.Fatalf("expected tier 1 session and filter, payload=%#v filter=%#v", payload, fake.nextFilter)
	}
	if payload.Category != "coding" || fake.nextFilter.Category != "coding" {
		t.Fatalf("expected coding category session and filter, payload=%#v filter=%#v", payload, fake.nextFilter)
	}
	if len(payload.Topics) == 0 {
		t.Fatalf("expected topics in session payload")
	}
	if payload.Progress.TotalAttempts != 1 {
		t.Fatalf("expected progress summary in session payload, got %#v", payload.Progress)
	}
	if !strings.Contains(payload.Instructions, "submit_coding_review_and_prepare_next") {
		t.Fatalf("expected workflow instructions to mention submit_coding_review_and_prepare_next, got %q", payload.Instructions)
	}
	lowerInstructions := strings.ToLower(payload.Instructions)
	for _, want := range []string{"inspect", "files", "tests"} {
		if !strings.Contains(lowerInstructions, want) {
			t.Fatalf("workflow should require %q in review instructions, got %q", want, payload.Instructions)
		}
	}
	if !strings.Contains(lowerInstructions, "do not ask for a prose answer in chat") {
		t.Fatalf("workflow should explicitly forbid prose answers in chat, got %q", payload.Instructions)
	}
	if strings.Contains(lowerInstructions, "ask the user for a prose answer in chat") {
		t.Fatalf("workflow should not ask for prose answers in chat, got %q", payload.Instructions)
	}
	if !strings.Contains(payload.Instructions, "requested tier") {
		t.Fatalf("workflow should require staying within the requested tier, got %q", payload.Instructions)
	}
	if !strings.Contains(payload.Instructions, "requested category") {
		t.Fatalf("workflow should require staying within the requested category, got %q", payload.Instructions)
	}
	if !strings.Contains(payload.Instructions, "coding_exercise") {
		t.Fatalf("workflow should describe coding exercise handling, got %q", payload.Instructions)
	}
	if strings.Contains(strings.ToLower(payload.Instructions), "clarifying questions") {
		t.Fatalf("workflow should not pause for clarifying questions: %q", payload.Instructions)
	}
	if strings.Contains(payload.Instructions, "immediately call get_next_question") {
		t.Fatalf("workflow should not require a separate get_next_question call after feedback, got %q", payload.Instructions)
	}
}

func TestCodingExercisesPromptInstructsAgentToStartSession(t *testing.T) {
	result, err := codingExercisesPromptHandler()(context.Background(), &sdkmcp.GetPromptRequest{})
	if err != nil {
		t.Fatalf("prompt handler returned error: %v", err)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("expected one prompt message, got %d", len(result.Messages))
	}
	text, ok := result.Messages[0].Content.(*sdkmcp.TextContent)
	if !ok {
		t.Fatalf("expected text prompt content, got %T", result.Messages[0].Content)
	}
	if !strings.Contains(text.Text, "start_coding_exercise_session") {
		t.Fatalf("expected prompt to mention start_coding_exercise_session, got %q", text.Text)
	}
}

func TestWorkflowResourceDescribesCodingExerciseFlow(t *testing.T) {
	result, err := workflowResourceHandler()(context.Background(), &sdkmcp.ReadResourceRequest{})
	if err != nil {
		t.Fatalf("resource handler returned error: %v", err)
	}
	if len(result.Contents) != 1 {
		t.Fatalf("expected one resource content item, got %d", len(result.Contents))
	}
	content := result.Contents[0]
	if content.URI != workflowResourceURI {
		t.Fatalf("unexpected resource URI: %q", content.URI)
	}
	if !strings.Contains(content.Text, "submit_coding_review_and_prepare_next") {
		t.Fatalf("expected workflow resource to mention submit_coding_review_and_prepare_next, got %q", content.Text)
	}
	lowerText := strings.ToLower(content.Text)
	for _, want := range []string{"inspect", "files", "tests"} {
		if !strings.Contains(lowerText, want) {
			t.Fatalf("workflow resource should require %q in review instructions, got %q", want, content.Text)
		}
	}
	if !strings.Contains(lowerText, "do not ask for a prose answer in chat") {
		t.Fatalf("workflow resource should explicitly forbid prose answers in chat, got %q", content.Text)
	}
	if strings.Contains(lowerText, "ask the user for a prose answer in chat") {
		t.Fatalf("workflow resource should not ask for prose answers in chat, got %q", content.Text)
	}
	if !strings.Contains(content.Text, "requested tier") {
		t.Fatalf("workflow resource should require staying within the requested tier, got %q", content.Text)
	}
	if !strings.Contains(content.Text, "requested category") {
		t.Fatalf("workflow resource should require staying within the requested category, got %q", content.Text)
	}
	if !strings.Contains(content.Text, "coding_exercise") {
		t.Fatalf("workflow resource should describe coding exercise handling, got %q", content.Text)
	}
	if strings.Contains(strings.ToLower(content.Text), "clarifying questions") {
		t.Fatalf("workflow resource should not pause for clarifying questions: %q", content.Text)
	}
	if strings.Contains(content.Text, "immediately call get_next_question") {
		t.Fatalf("workflow resource should not require a separate get_next_question call after feedback, got %q", content.Text)
	}
}

func callReq(args map[string]any) *sdkmcp.CallToolRequest {
	raw, err := json.Marshal(args)
	if err != nil {
		panic(err)
	}
	return &sdkmcp.CallToolRequest{Params: &sdkmcp.CallToolParamsRaw{Arguments: raw}}
}

func unmarshalTextResult(t *testing.T, result *sdkmcp.CallToolResult, out any) {
	t.Helper()
	if len(result.Content) != 1 {
		t.Fatalf("expected one content item, got %d", len(result.Content))
	}
	text, ok := result.Content[0].(*sdkmcp.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", result.Content[0])
	}
	if err := json.Unmarshal([]byte(text.Text), out); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
}
