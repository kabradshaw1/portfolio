package study

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kabradshaw1/portfolio/go/study-service/internal/content"
	"github.com/kabradshaw1/portfolio/go/study-service/internal/store"
)

type fakeStore struct {
	questions       []content.Question
	sessionID       int64
	createdSessions int
	submitted       store.SubmitAnswerInput
	feedback        store.FeedbackInput
	next            store.Question
	attempt         store.AnswerAttempt
}

func (f *fakeStore) UpsertQuestions(_ context.Context, questions []content.Question) error {
	f.questions = questions
	return nil
}

func (f *fakeStore) CreateSession(context.Context) (int64, error) {
	f.createdSessions++
	return f.sessionID, nil
}

func (f *fakeStore) ListTopics(context.Context) ([]store.Topic, error) {
	return []store.Topic{{Name: "Go", QuestionCount: 1}}, nil
}

func (f *fakeStore) NextQuestion(context.Context) (store.Question, error) {
	return f.next, nil
}

func (f *fakeStore) SubmitAnswer(_ context.Context, in store.SubmitAnswerInput) (store.AnswerAttempt, error) {
	f.submitted = in
	return f.attempt, nil
}

func (f *fakeStore) RecordFeedback(_ context.Context, in store.FeedbackInput) error {
	f.feedback = in
	return nil
}

func (f *fakeStore) UpdateExpectedAnswer(context.Context, int64, string) error {
	return nil
}

func (f *fakeStore) ProgressSummary(context.Context) (store.ProgressSummary, error) {
	return store.ProgressSummary{TotalAttempts: 1, AverageScore: 2}, nil
}

func TestImportMaterialParsesAndStoresQuestions(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "02-go.md", "# Go\n\n### 1. Maps?\n\nFast answer:\n\n> Use locks.\n")
	fake := &fakeStore{}
	svc := New(fake, dir)

	result, err := svc.ImportMaterial(context.Background())
	if err != nil {
		t.Fatalf("ImportMaterial returned error: %v", err)
	}

	if result.ImportedQuestions != 1 {
		t.Fatalf("expected 1 imported question, got %#v", result)
	}
	if len(fake.questions) != 1 || fake.questions[0].Prompt != "Maps?" {
		t.Fatalf("questions not stored: %#v", fake.questions)
	}
}

func TestSubmitAnswerCreatesSessionLazily(t *testing.T) {
	fake := &fakeStore{
		sessionID: 42,
		attempt: store.AnswerAttempt{
			ID:                     7,
			QuestionID:             3,
			Answer:                 "Use a mutex.",
			ExpectedAnswerSnapshot: "Use synchronization.",
		},
	}
	svc := New(fake, "/unused")

	review, err := svc.SubmitAnswer(context.Background(), 3, "Use a mutex.")
	if err != nil {
		t.Fatalf("SubmitAnswer returned error: %v", err)
	}

	if fake.createdSessions != 1 {
		t.Fatalf("expected one lazy session, got %d", fake.createdSessions)
	}
	if fake.submitted.SessionID != 42 || fake.submitted.QuestionID != 3 {
		t.Fatalf("unexpected submit input: %#v", fake.submitted)
	}
	if review.AttemptID != 7 || review.ExpectedAnswer != "Use synchronization." {
		t.Fatalf("unexpected review: %#v", review)
	}
}

func TestRecordFeedbackRejectsInvalidScores(t *testing.T) {
	fake := &fakeStore{}
	svc := New(fake, "/unused")

	err := svc.RecordFeedback(context.Background(), FeedbackInput{AttemptID: 1, Score: 4})
	if err == nil {
		t.Fatal("expected invalid score error")
	}
}

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}
