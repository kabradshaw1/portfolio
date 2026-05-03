package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/kabradshaw1/portfolio/go/coding-exercises-mcp-service/internal/content"
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

func TestUpsertQuestionsDeactivatesStaleQuestionsFromSameSource(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	if err := db.UpsertQuestions(ctx, []content.Question{
		{SourcePath: "08-coding-exercises.md", Topic: "Timed Coding Exercises", Prompt: "Sharded cache", Priority: 10, Tier: 1},
	}); err != nil {
		t.Fatalf("initial upsert: %v", err)
	}

	if err := db.UpsertQuestions(ctx, []content.Question{
		{SourcePath: "08-coding-exercises.md", Topic: "Timed Coding Exercises", Prompt: "Implement a cache with 32 shards, `Get`, `Set`, `Delete`, and TTL expiration.", Priority: 10, Tier: 1},
	}); err != nil {
		t.Fatalf("refresh upsert: %v", err)
	}

	topics, err := db.ListTopics(ctx)
	if err != nil {
		t.Fatalf("list topics: %v", err)
	}
	if len(topics) != 1 || topics[0].QuestionCount != 1 {
		t.Fatalf("expected one active refreshed question, got %#v", topics)
	}

	next, err := db.NextQuestion(ctx, QuestionFilter{Tier: 1})
	if err != nil {
		t.Fatalf("next question: %v", err)
	}
	if next.Prompt != "Implement a cache with 32 shards, `Get`, `Set`, `Delete`, and TTL expiration." {
		t.Fatalf("expected refreshed prompt, got %q", next.Prompt)
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

	q, err := db.NextQuestion(ctx, QuestionFilter{})
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

func TestRecordFeedbackIsIdempotentPerAttempt(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	if err := db.UpsertQuestions(ctx, []content.Question{{
		SourcePath:     "08-coding-exercises.md",
		Topic:          "Timed Coding Exercises",
		Category:       "coding",
		Kind:           "coding_exercise",
		Prompt:         "Build a worker pool.",
		ExpectedAnswer: "Use context cancellation and wait groups.",
		Priority:       10,
		Tier:           1,
	}}); err != nil {
		t.Fatalf("upsert question: %v", err)
	}

	q, err := db.NextQuestion(ctx, QuestionFilter{Tier: 1, Category: "coding"})
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
		Answer:     "Reviewed worker_pool.go and worker_pool_test.go.",
	})
	if err != nil {
		t.Fatalf("submit answer: %v", err)
	}

	if err := db.RecordFeedback(ctx, FeedbackInput{AttemptID: attempt.ID, Score: 1, MissingPoints: "cancellation"}); err != nil {
		t.Fatalf("first feedback: %v", err)
	}
	if err := db.RecordFeedback(ctx, FeedbackInput{AttemptID: attempt.ID, Score: 3, MissingPoints: ""}); err != nil {
		t.Fatalf("second feedback: %v", err)
	}

	summary, err := db.ProgressSummary(ctx)
	if err != nil {
		t.Fatalf("progress summary: %v", err)
	}
	if summary.TotalAttempts != 1 || summary.AverageScore != 3 {
		t.Fatalf("expected one scored attempt with updated score, got %#v", summary)
	}
	if len(summary.RecentAttempts) != 1 || summary.RecentAttempts[0].Score == nil || *summary.RecentAttempts[0].Score != 3 {
		t.Fatalf("expected one recent attempt with updated score, got %#v", summary.RecentAttempts)
	}
}

func TestRecordFeedbackRejectsUnknownAttempt(t *testing.T) {
	db := newTestDB(t)
	err := db.RecordFeedback(context.Background(), FeedbackInput{
		AttemptID: 999,
		Score:     2,
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for unknown attempt, got %v", err)
	}
}

func TestMigrateDeduplicatesLegacyFeedbackBeforeUniqueIndex(t *testing.T) {
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	ctx := context.Background()

	if _, err := sqlDB.ExecContext(ctx, `
CREATE TABLE questions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	source_path TEXT NOT NULL,
	topic TEXT NOT NULL,
	category TEXT NOT NULL DEFAULT '',
	kind TEXT NOT NULL DEFAULT 'qa',
	prompt TEXT NOT NULL,
	expected_answer TEXT NOT NULL DEFAULT '',
	is_follow_up INTEGER NOT NULL DEFAULT 0,
	priority INTEGER NOT NULL DEFAULT 0,
	tier INTEGER NOT NULL DEFAULT 3,
	active INTEGER NOT NULL DEFAULT 1,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(source_path, prompt, is_follow_up)
);
CREATE TABLE sessions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	started_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	ended_at TEXT
);
CREATE TABLE answer_attempts (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	session_id INTEGER NOT NULL REFERENCES sessions(id),
	question_id INTEGER NOT NULL REFERENCES questions(id),
	answer TEXT NOT NULL,
	expected_answer_snapshot TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE feedback (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	attempt_id INTEGER NOT NULL REFERENCES answer_attempts(id),
	score INTEGER NOT NULL CHECK(score >= 0 AND score <= 3),
	missing_points TEXT NOT NULL DEFAULT '',
	inaccurate_points TEXT NOT NULL DEFAULT '',
	suggested_answer TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO questions (id, source_path, topic, category, kind, prompt, expected_answer)
VALUES (1, '08-coding-exercises.md', 'Timed Coding Exercises', 'coding', 'coding_exercise', 'Build a worker pool.', 'Use context cancellation.');
INSERT INTO sessions (id) VALUES (1);
INSERT INTO answer_attempts (id, session_id, question_id, answer, expected_answer_snapshot)
VALUES (1, 1, 1, 'Reviewed worker_pool.go.', 'Use context cancellation.');
INSERT INTO feedback (attempt_id, score, missing_points) VALUES (1, 1, 'first');
INSERT INTO feedback (attempt_id, score, missing_points) VALUES (1, 3, 'latest');
`); err != nil {
		t.Fatalf("seed legacy schema: %v", err)
	}

	db := OpenSQL(sqlDB)
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate legacy duplicate feedback db: %v", err)
	}

	summary, err := db.ProgressSummary(ctx)
	if err != nil {
		t.Fatalf("progress summary: %v", err)
	}
	if summary.TotalAttempts != 1 || summary.AverageScore != 3 {
		t.Fatalf("expected deduped latest score, got %#v", summary)
	}
	if len(summary.RecentAttempts) != 1 || summary.RecentAttempts[0].Score == nil || *summary.RecentAttempts[0].Score != 3 {
		t.Fatalf("expected one recent attempt with latest score, got %#v", summary.RecentAttempts)
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

	first, err := db.NextQuestion(ctx, QuestionFilter{})
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

	second, err := db.NextQuestion(ctx, QuestionFilter{})
	if err != nil {
		t.Fatalf("second next question: %v", err)
	}
	if second.ID == first.ID {
		t.Fatalf("expected unanswered question before weak repeat, got same id %d", second.ID)
	}
}

func TestNextQuestionFiltersByTier(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	if err := db.UpsertQuestions(ctx, []content.Question{
		{SourcePath: "02-go.md", Topic: "Go", Prompt: "Tier two?", ExpectedAnswer: "Follow-up.", Priority: 10, Tier: 2},
		{SourcePath: "03-api.md", Topic: "API", Prompt: "Tier one?", ExpectedAnswer: "Likely.", Priority: 10, Tier: 1},
	}); err != nil {
		t.Fatalf("upsert questions: %v", err)
	}

	next, err := db.NextQuestion(ctx, QuestionFilter{Tier: 1})
	if err != nil {
		t.Fatalf("next tier 1 question: %v", err)
	}
	if next.Prompt != "Tier one?" || next.Tier != 1 {
		t.Fatalf("expected tier 1 question, got %#v", next)
	}

	next, err = db.NextQuestion(ctx, QuestionFilter{Tier: 2})
	if err != nil {
		t.Fatalf("next tier 2 question: %v", err)
	}
	if next.Prompt != "Tier two?" || next.Tier != 2 {
		t.Fatalf("expected tier 2 question, got %#v", next)
	}
}

func TestNextQuestionFiltersByCategory(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	if err := db.UpsertQuestions(ctx, []content.Question{
		{SourcePath: "02-go.md", Topic: "Go", Category: "golang", Kind: "qa", Prompt: "Go question?", ExpectedAnswer: "Go answer.", Priority: 10, Tier: 3},
		{SourcePath: "03-api.md", Topic: "API", Category: "api", Kind: "qa", Prompt: "API question?", ExpectedAnswer: "API answer.", Priority: 10, Tier: 3},
	}); err != nil {
		t.Fatalf("upsert questions: %v", err)
	}

	next, err := db.NextQuestion(ctx, QuestionFilter{Tier: 3, Category: "golang"})
	if err != nil {
		t.Fatalf("next golang tier 3 question: %v", err)
	}
	if next.Prompt != "Go question?" || next.Category != "golang" || next.Kind != "qa" {
		t.Fatalf("expected golang question with metadata, got %#v", next)
	}
}
