package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kabradshaw1/portfolio/go/study-service/internal/store"
	"github.com/kabradshaw1/portfolio/go/study-service/internal/study"
)

type fakeStudy struct {
	submittedQuestionID int64
	submittedAnswer     string
	feedback            study.FeedbackInput
	nextFilter          store.QuestionFilter
	nextQuestionCalls   int
}

func (f *fakeStudy) ImportMaterial(context.Context) (study.ImportResult, error) {
	return study.ImportResult{ImportedQuestions: 12}, nil
}

func (f *fakeStudy) ListTopics(context.Context) ([]store.Topic, error) {
	return []store.Topic{{Name: "Go", QuestionCount: 2, AnsweredCount: 1}}, nil
}

func (f *fakeStudy) GetNextQuestion(_ context.Context, filter store.QuestionFilter) (store.Question, error) {
	f.nextFilter = filter
	f.nextQuestionCalls++
	return store.Question{ID: 3, Topic: "Go", Prompt: "How do maps work?", ExpectedAnswer: "Use synchronization.", Priority: 10, Tier: filter.Tier}, nil
}

func (f *fakeStudy) SubmitAnswer(_ context.Context, questionID int64, answer string) (study.AnswerReview, error) {
	f.submittedQuestionID = questionID
	f.submittedAnswer = answer
	return study.AnswerReview{AttemptID: 9, QuestionID: questionID, Answer: answer, ExpectedAnswer: "Use synchronization."}, nil
}

func (f *fakeStudy) RecordFeedback(_ context.Context, input study.FeedbackInput) error {
	f.feedback = input
	return nil
}

func (f *fakeStudy) ProgressSummary(context.Context) (store.ProgressSummary, error) {
	return store.ProgressSummary{TotalAttempts: 1, AverageScore: 2}, nil
}

func (f *fakeStudy) UpdateExpectedAnswer(context.Context, int64, string) error {
	return nil
}

func TestSubmitAnswerHandlerDecodesArguments(t *testing.T) {
	fake := &fakeStudy{}
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

	var payload study.AnswerReview
	unmarshalTextResult(t, result, &payload)
	if payload.AttemptID != 9 || payload.ExpectedAnswer == "" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestRecordFeedbackHandlerValidatesScore(t *testing.T) {
	fake := &fakeStudy{}
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
	fake := &fakeStudy{}
	handler := submitAnswerAndPrepareNextHandler(fake)
	result, err := handler(context.Background(), callReq(map[string]any{
		"question_id": float64(2),
		"answer":      "Use channels.",
		"tier":        float64(1),
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

	var payload studyTurn
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

func TestStartStudySessionReturnsWorkflowAndCurrentState(t *testing.T) {
	fake := &fakeStudy{}
	handler := startStudySessionHandler(fake)
	result, err := handler(context.Background(), callReq(map[string]any{
		"study_set": "micro1",
		"tier":      float64(1),
	}))
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handler returned MCP error: %#v", result)
	}

	var payload studySession
	unmarshalTextResult(t, result, &payload)
	if payload.StudySet != "micro1" {
		t.Fatalf("unexpected study set: %q", payload.StudySet)
	}
	if payload.NextQuestion.ID != 3 {
		t.Fatalf("expected next question in session payload, got %#v", payload.NextQuestion)
	}
	if payload.Tier != 1 || fake.nextFilter.Tier != 1 {
		t.Fatalf("expected tier 1 session and filter, payload=%#v filter=%#v", payload, fake.nextFilter)
	}
	if len(payload.Topics) == 0 {
		t.Fatalf("expected topics in session payload")
	}
	if payload.Progress.TotalAttempts != 1 {
		t.Fatalf("expected progress summary in session payload, got %#v", payload.Progress)
	}
	if !strings.Contains(payload.Instructions, "submit_answer_and_prepare_next") {
		t.Fatalf("expected workflow instructions to mention submit_answer_and_prepare_next, got %q", payload.Instructions)
	}
	if !strings.Contains(payload.Instructions, "requested tier") {
		t.Fatalf("workflow should require staying within the requested tier, got %q", payload.Instructions)
	}
	if strings.Contains(strings.ToLower(payload.Instructions), "clarifying questions") {
		t.Fatalf("workflow should not pause for clarifying questions: %q", payload.Instructions)
	}
	if strings.Contains(payload.Instructions, "immediately call get_next_question") {
		t.Fatalf("workflow should not require a separate get_next_question call after feedback, got %q", payload.Instructions)
	}
	if !strings.Contains(payload.Instructions, "exactly one MCP tool") {
		t.Fatalf("workflow should require exactly one MCP tool after an answer, got %q", payload.Instructions)
	}
	if !strings.Contains(payload.Instructions, "Explanation:") {
		t.Fatalf("workflow should require an explanation section, got %q", payload.Instructions)
	}
	if !strings.Contains(payload.Instructions, "Interview answer:") {
		t.Fatalf("workflow should require an interview answer section, got %q", payload.Instructions)
	}
	if !strings.Contains(payload.Instructions, "Minimum answer") {
		t.Fatalf("workflow should mention an optional minimum answer, got %q", payload.Instructions)
	}
	if !strings.Contains(payload.Instructions, "Go code snippet") {
		t.Fatalf("workflow should mention useful Go code snippets, got %q", payload.Instructions)
	}
}

func TestStudyMicro1PromptInstructsAgentToStartSession(t *testing.T) {
	result, err := studyMicro1PromptHandler()(context.Background(), &sdkmcp.GetPromptRequest{})
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
	if !strings.Contains(text.Text, "start_study_session") {
		t.Fatalf("expected prompt to mention start_study_session, got %q", text.Text)
	}
}

func TestWorkflowResourceDescribesMicro1StudyFlow(t *testing.T) {
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
	if !strings.Contains(content.Text, "submit_answer_and_prepare_next") {
		t.Fatalf("expected workflow resource to mention submit_answer_and_prepare_next, got %q", content.Text)
	}
	if !strings.Contains(content.Text, "requested tier") {
		t.Fatalf("workflow resource should require staying within the requested tier, got %q", content.Text)
	}
	if strings.Contains(strings.ToLower(content.Text), "clarifying questions") {
		t.Fatalf("workflow resource should not pause for clarifying questions: %q", content.Text)
	}
	if strings.Contains(content.Text, "immediately call get_next_question") {
		t.Fatalf("workflow resource should not require a separate get_next_question call after feedback, got %q", content.Text)
	}
	if !strings.Contains(content.Text, "exactly one MCP tool") {
		t.Fatalf("workflow resource should require exactly one MCP tool after an answer, got %q", content.Text)
	}
	if !strings.Contains(content.Text, "Explanation:") {
		t.Fatalf("workflow resource should require an explanation section, got %q", content.Text)
	}
	if !strings.Contains(content.Text, "Interview answer:") {
		t.Fatalf("workflow resource should require an interview answer section, got %q", content.Text)
	}
	if !strings.Contains(content.Text, "Minimum answer") {
		t.Fatalf("workflow resource should mention an optional minimum answer, got %q", content.Text)
	}
	if !strings.Contains(content.Text, "Go code snippet") {
		t.Fatalf("workflow resource should mention useful Go code snippets, got %q", content.Text)
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
