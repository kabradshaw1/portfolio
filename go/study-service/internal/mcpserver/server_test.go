package mcpserver

import (
	"context"
	"encoding/json"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kabradshaw1/portfolio/go/study-service/internal/store"
	"github.com/kabradshaw1/portfolio/go/study-service/internal/study"
)

type fakeStudy struct {
	submittedQuestionID int64
	submittedAnswer     string
	feedback            study.FeedbackInput
}

func (f *fakeStudy) ImportMaterial(context.Context) (study.ImportResult, error) {
	return study.ImportResult{ImportedQuestions: 12}, nil
}

func (f *fakeStudy) ListTopics(context.Context) ([]store.Topic, error) {
	return []store.Topic{{Name: "Go", QuestionCount: 2, AnsweredCount: 1}}, nil
}

func (f *fakeStudy) GetNextQuestion(context.Context) (store.Question, error) {
	return store.Question{ID: 3, Topic: "Go", Prompt: "How do maps work?", ExpectedAnswer: "Use synchronization.", Priority: 10}, nil
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
