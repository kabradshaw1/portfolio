package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kabradshaw1/portfolio/go/qa-mcp-service/internal/qa"
	"github.com/kabradshaw1/portfolio/go/qa-mcp-service/internal/store"
)

type fakeQA struct {
	submittedQuestionID int64
	submittedAnswer     string
	feedback            qa.FeedbackInput
	nextFilter          store.QuestionFilter
	nextQuestionCalls   int
}

func (f *fakeQA) ImportMaterial(context.Context) (qa.ImportResult, error) {
	return qa.ImportResult{ImportedQuestions: 12}, nil
}

func (f *fakeQA) ListTopics(context.Context) ([]store.Topic, error) {
	return []store.Topic{{Name: "Go", QuestionCount: 2, AnsweredCount: 1}}, nil
}

func (f *fakeQA) GetNextQuestion(_ context.Context, filter store.QuestionFilter) (store.Question, error) {
	f.nextFilter = filter
	f.nextQuestionCalls++
	return store.Question{ID: 3, Topic: "Go", Prompt: "How do maps work?", ExpectedAnswer: "Use synchronization.", Priority: 10, Tier: filter.Tier}, nil
}

func (f *fakeQA) SubmitAnswer(_ context.Context, questionID int64, answer string) (qa.AnswerReview, error) {
	f.submittedQuestionID = questionID
	f.submittedAnswer = answer
	return qa.AnswerReview{AttemptID: 9, QuestionID: questionID, Answer: answer, ExpectedAnswer: "Use synchronization."}, nil
}

func (f *fakeQA) RecordFeedback(_ context.Context, input qa.FeedbackInput) error {
	f.feedback = input
	return nil
}

func (f *fakeQA) ProgressSummary(context.Context) (store.ProgressSummary, error) {
	return store.ProgressSummary{TotalAttempts: 1, AverageScore: 2}, nil
}

func (f *fakeQA) UpdateExpectedAnswer(context.Context, int64, string) error {
	return nil
}

func TestSubmitAnswerHandlerDecodesArguments(t *testing.T) {
	fake := &fakeQA{}
	handler := submitAnswerHandler(fake)
	result, err := handler(context.Background(), callReq(map[string]any{
		"question_id": float64(3),
		"answer":      "Use a mutex.",
	}))
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handler returned MCP error: %#v", result)
	}
	if fake.submittedQuestionID != 3 || fake.submittedAnswer != "Use a mutex." {
		t.Fatalf("unexpected submit args: id=%d answer=%q", fake.submittedQuestionID, fake.submittedAnswer)
	}

	var payload qa.AnswerReview
	unmarshalTextResult(t, result, &payload)
	if payload.AttemptID != 9 || payload.ExpectedAnswer == "" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestRecordFeedbackHandlerValidatesScore(t *testing.T) {
	fake := &fakeQA{}
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

func TestSubmitAnswerAndPrepareNextRecordsPriorFeedbackAndReturnsReviewWithNextQuestion(t *testing.T) {
	fake := &fakeQA{}
	handler := submitAnswerAndPrepareNextHandler(fake)
	result, err := handler(context.Background(), callReq(map[string]any{
		"question_id": float64(2),
		"answer":      "Use channels.",
		"tier":        float64(1),
		"category":    "golang",
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
	if fake.submittedQuestionID != 2 || fake.submittedAnswer != "Use channels." {
		t.Fatalf("unexpected submit args: id=%d answer=%q", fake.submittedQuestionID, fake.submittedAnswer)
	}
	if fake.nextQuestionCalls != 1 {
		t.Fatalf("expected next question to be loaded once, got %d", fake.nextQuestionCalls)
	}
	if fake.nextFilter.Tier != 1 {
		t.Fatalf("expected tier filter to be passed, got %#v", fake.nextFilter)
	}
	if fake.nextFilter.Category != "golang" {
		t.Fatalf("expected category filter to be passed, got %#v", fake.nextFilter)
	}

	var payload qaTurn
	unmarshalTextResult(t, result, &payload)
	if payload.Review.AttemptID != 9 || payload.Review.ExpectedAnswer == "" {
		t.Fatalf("unexpected review payload: %#v", payload.Review)
	}
	if payload.NextQuestion.ID != 3 {
		t.Fatalf("unexpected next question: %#v", payload.NextQuestion)
	}
	if payload.Tier != 1 {
		t.Fatalf("expected turn payload tier, got %#v", payload)
	}
	if !payload.PreviousFeedbackRecorded {
		t.Fatalf("expected previous feedback recorded flag")
	}
}

func TestStartQASessionReturnsWorkflowAndCurrentState(t *testing.T) {
	fake := &fakeQA{}
	handler := startQASessionHandler(fake)
	result, err := handler(context.Background(), callReq(map[string]any{
		"tier":     float64(1),
		"category": "golang",
	}))
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handler returned MCP error: %#v", result)
	}

	var payload qaSession
	unmarshalTextResult(t, result, &payload)
	if payload.NextQuestion.ID != 3 {
		t.Fatalf("expected next question in session payload, got %#v", payload.NextQuestion)
	}
	if payload.Tier != 1 || fake.nextFilter.Tier != 1 {
		t.Fatalf("expected tier 1 session and filter, payload=%#v filter=%#v", payload, fake.nextFilter)
	}
	if payload.Category != "golang" || fake.nextFilter.Category != "golang" {
		t.Fatalf("expected golang category session and filter, payload=%#v filter=%#v", payload, fake.nextFilter)
	}
	if len(payload.Topics) == 0 {
		t.Fatalf("expected topics in session payload")
	}
	if payload.Progress.TotalAttempts != 1 {
		t.Fatalf("expected progress summary in session payload, got %#v", payload.Progress)
	}
	assertQAWorkflowInstructions(t, payload.Instructions)
}

func assertQAWorkflowInstructions(t *testing.T, instructions string) {
	t.Helper()
	if !strings.Contains(instructions, "submit_qa_answer_and_prepare_next") {
		t.Fatalf("expected workflow instructions to mention submit_qa_answer_and_prepare_next, got %q", instructions)
	}
	if !strings.Contains(instructions, "requested tier") {
		t.Fatalf("workflow should require staying within the requested tier, got %q", instructions)
	}
	if !strings.Contains(instructions, "requested category") {
		t.Fatalf("workflow should require staying within the requested category, got %q", instructions)
	}
	if strings.Contains(instructions, "coding_exercise") {
		t.Fatalf("QA workflow should not mention coding exercises, got %q", instructions)
	}
	lower := strings.ToLower(instructions)
	if strings.Contains(lower, "implementation review") {
		t.Fatalf("QA workflow should not mention implementation review, got %q", instructions)
	}
	if strings.Contains(lower, "inspect files") || strings.Contains(lower, "inspect the relevant files") {
		t.Fatalf("QA workflow should not mention workspace file inspection, got %q", instructions)
	}
	if strings.Contains(lower, "coding exercise submission") {
		t.Fatalf("QA workflow should not mention coding exercise submission, got %q", instructions)
	}
	if strings.Contains(lower, "clarifying questions") {
		t.Fatalf("workflow should not pause for clarifying questions: %q", instructions)
	}
	if strings.Contains(instructions, "immediately call get_next_question") {
		t.Fatalf("workflow should not require a separate get_next_question call after feedback, got %q", instructions)
	}
	if !strings.Contains(instructions, "After the user answers a QA question, call exactly one MCP tool: submit_qa_answer_and_prepare_next with question_id, answer, tier, category, and any previous_feedback payload you prepared for the prior answer.") {
		t.Fatalf("workflow should require exactly one QA submit-and-next tool after an answer, got %q", instructions)
	}
	if !strings.Contains(instructions, "Score: X/3") {
		t.Fatalf("workflow should require a score section, got %q", instructions)
	}
	if !strings.Contains(instructions, "Explanation:") {
		t.Fatalf("workflow should require an explanation section, got %q", instructions)
	}
	if !strings.Contains(instructions, "Interview answer:") {
		t.Fatalf("workflow should require an interview answer section, got %q", instructions)
	}
	if !strings.Contains(instructions, "Minimum answer, only when useful:") {
		t.Fatalf("workflow should mention an optional minimum answer, got %q", instructions)
	}
	if !strings.Contains(instructions, "Memory hook, only when useful:") {
		t.Fatalf("workflow should mention an optional memory hook, got %q", instructions)
	}
}

func TestQAPromptInstructsAgentToStartSession(t *testing.T) {
	result, err := qaPromptHandler()(context.Background(), &sdkmcp.GetPromptRequest{})
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
	if !strings.Contains(text.Text, "start_qa_session") {
		t.Fatalf("expected prompt to mention start_qa_session, got %q", text.Text)
	}
}

func TestWorkflowResourceDescribesQAFlow(t *testing.T) {
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
	assertQAWorkflowInstructions(t, content.Text)
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
