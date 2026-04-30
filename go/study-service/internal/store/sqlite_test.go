package store

import (
	"context"
	"database/sql"
	"testing"

	"github.com/kabradshaw1/portfolio/go/study-service/internal/content"
	_ "modernc.org/sqlite"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db := OpenSQL(sqlDB)
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestUpsertQuestionsIsIdempotent(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	questions := []content.Question{
		{SourcePath: "02-go.md", Topic: "Go", Prompt: "How do maps work?", ExpectedAnswer: "Use synchronization.", Priority: 10},
		{SourcePath: "02-go.md", Topic: "Go", Prompt: "When is sync.Map appropriate?", IsFollowUp: true, Priority: 10},
	}

	if err := db.UpsertQuestions(ctx, questions); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if err := db.UpsertQuestions(ctx, questions); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	topics, err := db.ListTopics(ctx)
	if err != nil {
		t.Fatalf("list topics: %v", err)
	}
	if len(topics) != 1 {
		t.Fatalf("expected 1 topic, got %#v", topics)
	}
	if topics[0].Name != "Go" || topics[0].QuestionCount != 2 || topics[0].AnsweredCount != 0 {
		t.Fatalf("unexpected topic summary: %#v", topics[0])
	}
}

func TestSubmitAnswerAndFeedbackArePersisted(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	if err := db.UpsertQuestions(ctx, []content.Question{{
		SourcePath:     "02-go.md",
		Topic:          "Go",
		Prompt:         "How do maps work?",
		ExpectedAnswer: "Use a mutex or sharding.",
		Priority:       10,
	}}); err != nil {
		t.Fatalf("upsert question: %v", err)
	}

	q, err := db.NextQuestion(ctx)
	if err != nil {
		t.Fatalf("next question: %v", err)
	}
	sessionID, err := db.CreateSession(ctx)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	attempt, err := db.SubmitAnswer(ctx, SubmitAnswerInput{
		SessionID:  sessionID,
		QuestionID: q.ID,
		Answer:     "Maps need a lock for concurrent writes.",
	})
	if err != nil {
		t.Fatalf("submit answer: %v", err)
	}
	if attempt.ExpectedAnswerSnapshot != "Use a mutex or sharding." {
		t.Fatalf("unexpected expected answer snapshot: %#v", attempt)
	}

	err = db.RecordFeedback(ctx, FeedbackInput{
		AttemptID:        attempt.ID,
		Score:            2,
		MissingPoints:    "Mention race detector.",
		InaccuratePoints: "",
		SuggestedAnswer:  "Use sync.RWMutex, sharding, or sync.Map for specific cases.",
	})
	if err != nil {
		t.Fatalf("record feedback: %v", err)
	}

	summary, err := db.ProgressSummary(ctx)
	if err != nil {
		t.Fatalf("progress summary: %v", err)
	}
	if summary.TotalAttempts != 1 || summary.AverageScore != 2 {
		t.Fatalf("unexpected progress summary: %#v", summary)
	}
	if len(summary.RecentAttempts) != 1 || summary.RecentAttempts[0].Score == nil || *summary.RecentAttempts[0].Score != 2 {
		t.Fatalf("recent attempt missing score: %#v", summary.RecentAttempts)
	}
}

func TestNextQuestionPrefersUnansweredThenWeakest(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	if err := db.UpsertQuestions(ctx, []content.Question{
		{SourcePath: "02-go.md", Topic: "Go", Prompt: "Weak?", ExpectedAnswer: "Weak answer.", Priority: 10},
		{SourcePath: "03-api.md", Topic: "API", Prompt: "Unseen?", ExpectedAnswer: "Unseen answer.", Priority: 10},
	}); err != nil {
		t.Fatalf("upsert questions: %v", err)
	}

	first, err := db.NextQuestion(ctx)
	if err != nil {
		t.Fatalf("first next question: %v", err)
	}
	sessionID, err := db.CreateSession(ctx)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	attempt, err := db.SubmitAnswer(ctx, SubmitAnswerInput{SessionID: sessionID, QuestionID: first.ID, Answer: "bad"})
	if err != nil {
		t.Fatalf("submit answer: %v", err)
	}
	if err := db.RecordFeedback(ctx, FeedbackInput{AttemptID: attempt.ID, Score: 0}); err != nil {
		t.Fatalf("record feedback: %v", err)
	}

	second, err := db.NextQuestion(ctx)
	if err != nil {
		t.Fatalf("second next question: %v", err)
	}
	if second.ID == first.ID {
		t.Fatalf("expected unanswered question before weak repeat, got same id %d", second.ID)
	}
}
