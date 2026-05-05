package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kabradshaw1/portfolio/go/coding-exercises-mcp-service/internal/content"
	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("not found")

type DB struct {
	db *sql.DB
}

type Topic struct {
	Name          string `json:"name"`
	Category      string `json:"category"`
	QuestionCount int    `json:"question_count"`
	AnsweredCount int    `json:"answered_count"`
}

type Question struct {
	ID               int64        `json:"id"`
	SourcePath       string       `json:"source_path"`
	Topic            string       `json:"topic"`
	Category         string       `json:"category"`
	Kind             string       `json:"kind"`
	Prompt           string       `json:"prompt"`
	ExpectedAnswer   string       `json:"expected_answer"`
	ParentQuestionID *int64       `json:"parent_question_id,omitempty"`
	ParentPrompt     string       `json:"parent_prompt,omitempty"`
	RepoAnchors      []RepoAnchor `json:"repo_anchors,omitempty"`
	IsFollowUp       bool         `json:"is_follow_up"`
	Priority         int          `json:"priority"`
	Tier             int          `json:"tier"`
}

type RepoAnchor struct {
	Path string `json:"path"`
	Note string `json:"note"`
}

type QuestionFilter struct {
	Tier     int    `json:"tier,omitempty"`
	Category string `json:"category,omitempty"`
}

type SubmitAnswerInput struct {
	SessionID  int64
	QuestionID int64
	Answer     string
}

type AnswerAttempt struct {
	ID                     int64     `json:"id"`
	SessionID              int64     `json:"session_id"`
	QuestionID             int64     `json:"question_id"`
	Answer                 string    `json:"answer"`
	ExpectedAnswerSnapshot string    `json:"expected_answer_snapshot"`
	CreatedAt              time.Time `json:"created_at"`
}

type FeedbackInput struct {
	AttemptID        int64
	Score            int
	MissingPoints    string
	InaccuratePoints string
	SuggestedAnswer  string
}

type RecentAttempt struct {
	AttemptID  int64     `json:"attempt_id"`
	QuestionID int64     `json:"question_id"`
	Topic      string    `json:"topic"`
	Prompt     string    `json:"prompt"`
	Answer     string    `json:"answer"`
	Score      *int      `json:"score,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type WeakTopic struct {
	Topic        string  `json:"topic"`
	AverageScore float64 `json:"average_score"`
	Attempts     int     `json:"attempts"`
}

type ProgressSummary struct {
	TotalAttempts  int             `json:"total_attempts"`
	AverageScore   float64         `json:"average_score"`
	RecentAttempts []RecentAttempt `json:"recent_attempts"`
	WeakTopics     []WeakTopic     `json:"weak_topics"`
}

func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	return &DB{db: sqlDB}, nil
}

func OpenSQL(db *sql.DB) *DB {
	return &DB{db: db}
}

func (d *DB) Close() error {
	return d.db.Close()
}

func (d *DB) Migrate(ctx context.Context) error {
	_, err := d.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS questions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	source_path TEXT NOT NULL,
	topic TEXT NOT NULL,
	category TEXT NOT NULL DEFAULT '',
	kind TEXT NOT NULL DEFAULT 'qa',
	prompt TEXT NOT NULL,
	expected_answer TEXT NOT NULL DEFAULT '',
	parent_question_id INTEGER REFERENCES questions(id),
	parent_prompt TEXT NOT NULL DEFAULT '',
	is_follow_up INTEGER NOT NULL DEFAULT 0,
	priority INTEGER NOT NULL DEFAULT 0,
	tier INTEGER NOT NULL DEFAULT 3,
	active INTEGER NOT NULL DEFAULT 1,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(source_path, prompt, is_follow_up)
);

CREATE TABLE IF NOT EXISTS question_repo_anchors (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	question_id INTEGER NOT NULL REFERENCES questions(id) ON DELETE CASCADE,
	path TEXT NOT NULL,
	note TEXT NOT NULL DEFAULT '',
	position INTEGER NOT NULL DEFAULT 0,
	UNIQUE(question_id, path, note)
);

CREATE TABLE IF NOT EXISTS sessions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	started_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	ended_at TEXT
);

CREATE TABLE IF NOT EXISTS answer_attempts (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	session_id INTEGER NOT NULL REFERENCES sessions(id),
	question_id INTEGER NOT NULL REFERENCES questions(id),
	answer TEXT NOT NULL,
	expected_answer_snapshot TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS feedback (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	attempt_id INTEGER NOT NULL REFERENCES answer_attempts(id),
	score INTEGER NOT NULL CHECK(score >= 0 AND score <= 3),
	missing_points TEXT NOT NULL DEFAULT '',
	inaccurate_points TEXT NOT NULL DEFAULT '',
	suggested_answer TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`)
	if err != nil {
		return fmt.Errorf("migrate sqlite: %w", err)
	}
	if err := d.dedupeFeedback(ctx); err != nil {
		return err
	}
	if _, err := d.db.ExecContext(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS feedback_attempt_id_unique ON feedback(attempt_id);`); err != nil {
		return fmt.Errorf("create feedback attempt unique index: %w", err)
	}
	if err := d.ensureColumn(ctx, "questions", "tier", "INTEGER NOT NULL DEFAULT 3"); err != nil {
		return err
	}
	if err := d.ensureColumn(ctx, "questions", "category", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := d.ensureColumn(ctx, "questions", "kind", "TEXT NOT NULL DEFAULT 'qa'"); err != nil {
		return err
	}
	if err := d.ensureColumn(ctx, "questions", "parent_question_id", "INTEGER"); err != nil {
		return err
	}
	if err := d.ensureColumn(ctx, "questions", "parent_prompt", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	return nil
}

func (d *DB) dedupeFeedback(ctx context.Context) error {
	_, err := d.db.ExecContext(ctx, `
DELETE FROM feedback
WHERE id NOT IN (
	SELECT MAX(id)
	FROM feedback
	GROUP BY attempt_id
);
`)
	if err != nil {
		return fmt.Errorf("dedupe feedback: %w", err)
	}
	return nil
}

func (d *DB) ensureColumn(ctx context.Context, table, column, definition string) error {
	rows, err := d.db.QueryContext(ctx, `PRAGMA table_info(`+table+`);`)
	if err != nil {
		return fmt.Errorf("inspect %s columns: %w", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return fmt.Errorf("scan %s column: %w", table, err)
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("inspect %s columns: %w", table, err)
	}
	if _, err := d.db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s;", table, column, definition)); err != nil {
		return fmt.Errorf("add %s.%s column: %w", table, column, err)
	}
	return nil
}

func (d *DB) UpsertQuestions(ctx context.Context, questions []content.Question) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin question import: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	sourcePaths := uniqueSourcePaths(questions)
	for _, sourcePath := range sourcePaths {
		if _, err := tx.ExecContext(ctx, `
UPDATE questions SET active = 0, updated_at = CURRENT_TIMESTAMP WHERE source_path = ? AND active = 1;
`, sourcePath); err != nil {
			return fmt.Errorf("deactivate stale questions for %s: %w", sourcePath, err)
		}
	}

	anchorsByQuestion := baseAnchorsByQuestion(questions)

	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO questions (source_path, topic, category, kind, prompt, expected_answer, parent_prompt, is_follow_up, priority, tier, active, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, CURRENT_TIMESTAMP)
ON CONFLICT(source_path, prompt, is_follow_up) DO UPDATE SET
	topic = excluded.topic,
	category = excluded.category,
	kind = excluded.kind,
	parent_prompt = excluded.parent_prompt,
	expected_answer = CASE
		WHEN excluded.expected_answer != '' THEN excluded.expected_answer
		ELSE questions.expected_answer
	END,
	priority = excluded.priority,
	tier = excluded.tier,
	active = 1,
	updated_at = CURRENT_TIMESTAMP;
`)
	if err != nil {
		return fmt.Errorf("prepare question upsert: %w", err)
	}
	defer stmt.Close()

	for _, q := range questions {
		if q.Prompt == "" {
			continue
		}
		tier := q.Tier
		if tier == 0 {
			tier = 3
		}
		category := q.Category
		if category == "" {
			category = "general"
		}
		kind := q.Kind
		if kind == "" {
			kind = "qa"
		}
		if _, err := stmt.ExecContext(ctx, q.SourcePath, q.Topic, category, kind, q.Prompt, q.ExpectedAnswer, q.ParentPrompt, boolInt(q.IsFollowUp), q.Priority, tier); err != nil {
			return fmt.Errorf("upsert question %q: %w", q.Prompt, err)
		}
		questionID, err := questionID(ctx, tx, q.SourcePath, q.Prompt, q.IsFollowUp)
		if err != nil {
			return err
		}
		anchors := q.RepoAnchors
		if q.IsFollowUp && len(anchors) == 0 && q.ParentPrompt != "" {
			anchors = anchorsByQuestion[questionKey{sourcePath: q.SourcePath, prompt: q.ParentPrompt}]
		}
		if err := replaceAnchors(ctx, tx, questionID, anchors); err != nil {
			return fmt.Errorf("replace anchors for %q: %w", q.Prompt, err)
		}
	}
	if err := resolveParentQuestionIDs(ctx, tx, sourcePaths); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit question import: %w", err)
	}
	return nil
}

type questionKey struct {
	sourcePath string
	prompt     string
}

func baseAnchorsByQuestion(questions []content.Question) map[questionKey][]content.RepoAnchor {
	out := make(map[questionKey][]content.RepoAnchor)
	for _, q := range questions {
		if q.IsFollowUp || len(q.RepoAnchors) == 0 {
			continue
		}
		out[questionKey{sourcePath: q.SourcePath, prompt: q.Prompt}] = q.RepoAnchors
	}
	return out
}

func questionID(ctx context.Context, tx *sql.Tx, sourcePath, prompt string, isFollowUp bool) (int64, error) {
	var id int64
	if err := tx.QueryRowContext(ctx, `
SELECT id FROM questions WHERE source_path = ? AND prompt = ? AND is_follow_up = ?;
`, sourcePath, prompt, boolInt(isFollowUp)).Scan(&id); err != nil {
		return 0, fmt.Errorf("load upserted question id for %q: %w", prompt, err)
	}
	return id, nil
}

func replaceAnchors(ctx context.Context, tx *sql.Tx, questionID int64, anchors []content.RepoAnchor) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM question_repo_anchors WHERE question_id = ?;`, questionID); err != nil {
		return err
	}
	for i, anchor := range anchors {
		if anchor.Path == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO question_repo_anchors (question_id, path, note, position)
VALUES (?, ?, ?, ?)
ON CONFLICT(question_id, path, note) DO UPDATE SET position = excluded.position;
`, questionID, anchor.Path, anchor.Note, i); err != nil {
			return err
		}
	}
	return nil
}

func resolveParentQuestionIDs(ctx context.Context, tx *sql.Tx, sourcePaths []string) error {
	for _, sourcePath := range sourcePaths {
		if _, err := tx.ExecContext(ctx, `
UPDATE questions
SET parent_question_id = (
	SELECT parent.id
	FROM questions parent
	WHERE parent.source_path = questions.source_path
		AND parent.prompt = questions.parent_prompt
		AND parent.is_follow_up = 0
		AND parent.active = 1
	LIMIT 1
)
WHERE source_path = ? AND is_follow_up = 1 AND active = 1;
`, sourcePath); err != nil {
			return fmt.Errorf("resolve parent questions for %s: %w", sourcePath, err)
		}
	}
	return nil
}

func uniqueSourcePaths(questions []content.Question) []string {
	seen := make(map[string]bool)
	var paths []string
	for _, q := range questions {
		if q.SourcePath == "" || seen[q.SourcePath] {
			continue
		}
		seen[q.SourcePath] = true
		paths = append(paths, q.SourcePath)
	}
	return paths
}

func (d *DB) CreateSession(ctx context.Context) (int64, error) {
	res, err := d.db.ExecContext(ctx, `INSERT INTO sessions DEFAULT VALUES`)
	if err != nil {
		return 0, fmt.Errorf("create session: %w", err)
	}
	return res.LastInsertId()
}

func (d *DB) ListTopics(ctx context.Context) ([]Topic, error) {
	rows, err := d.db.QueryContext(ctx, `
SELECT q.topic, q.category, COUNT(*) AS question_count, COUNT(DISTINCT aa.question_id) AS answered_count
FROM questions q
LEFT JOIN answer_attempts aa ON aa.question_id = q.id
WHERE q.active = 1
GROUP BY q.topic, q.category
ORDER BY q.topic;
`)
	if err != nil {
		return nil, fmt.Errorf("list topics: %w", err)
	}
	defer rows.Close()

	var topics []Topic
	for rows.Next() {
		var topic Topic
		if err := rows.Scan(&topic.Name, &topic.Category, &topic.QuestionCount, &topic.AnsweredCount); err != nil {
			return nil, fmt.Errorf("scan topic: %w", err)
		}
		topics = append(topics, topic)
	}
	return topics, rows.Err()
}

func (d *DB) NextQuestion(ctx context.Context, filter QuestionFilter) (Question, error) {
	query := `
SELECT q.id, q.source_path, q.topic, q.category, q.kind, q.prompt, q.expected_answer,
	q.parent_question_id, q.parent_prompt, q.is_follow_up, q.priority, q.tier
FROM questions q
LEFT JOIN answer_attempts aa ON aa.question_id = q.id
LEFT JOIN feedback f ON f.attempt_id = aa.id
WHERE q.active = 1 AND q.is_follow_up = 0`
	var args []any
	if filter.Tier > 0 {
		query += ` AND q.tier = ?`
		args = append(args, filter.Tier)
	}
	if filter.Category != "" {
		query += ` AND q.category = ?`
		args = append(args, filter.Category)
	}
	query += `
GROUP BY q.id
ORDER BY
	CASE WHEN COUNT(aa.id) = 0 THEN 0 ELSE 1 END,
	q.tier ASC,
	q.priority DESC,
	COALESCE(MIN(f.score), 4) ASC,
	MAX(aa.created_at) ASC,
	q.id ASC
LIMIT 1;
`
	row := d.db.QueryRowContext(ctx, query, args...)
	var q Question
	var followUp int
	var parentQuestionID sql.NullInt64
	if err := row.Scan(&q.ID, &q.SourcePath, &q.Topic, &q.Category, &q.Kind, &q.Prompt, &q.ExpectedAnswer, &parentQuestionID, &q.ParentPrompt, &followUp, &q.Priority, &q.Tier); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Question{}, ErrNotFound
		}
		return Question{}, fmt.Errorf("next question: %w", err)
	}
	if parentQuestionID.Valid {
		q.ParentQuestionID = &parentQuestionID.Int64
	}
	q.IsFollowUp = followUp == 1
	anchors, err := d.loadAnchors(ctx, q.ID)
	if err != nil {
		return Question{}, err
	}
	q.RepoAnchors = anchors
	return q, nil
}

func (d *DB) NextFollowUp(ctx context.Context, parentQuestionID int64) (Question, error) {
	row := d.db.QueryRowContext(ctx, `
SELECT q.id, q.source_path, q.topic, q.category, q.kind, q.prompt, q.expected_answer,
	q.parent_question_id, q.parent_prompt, q.is_follow_up, q.priority, q.tier
FROM questions q
LEFT JOIN answer_attempts aa ON aa.question_id = q.id
LEFT JOIN feedback f ON f.attempt_id = aa.id
WHERE q.active = 1 AND q.is_follow_up = 1 AND q.parent_question_id = ?
GROUP BY q.id
ORDER BY
	CASE WHEN COUNT(aa.id) = 0 THEN 0 ELSE 1 END,
	COALESCE(MIN(f.score), 4) ASC,
	MAX(aa.created_at) ASC,
	q.id ASC
LIMIT 1;
`, parentQuestionID)

	var q Question
	var followUp int
	var parentID sql.NullInt64
	if err := row.Scan(&q.ID, &q.SourcePath, &q.Topic, &q.Category, &q.Kind, &q.Prompt, &q.ExpectedAnswer, &parentID, &q.ParentPrompt, &followUp, &q.Priority, &q.Tier); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Question{}, ErrNotFound
		}
		return Question{}, fmt.Errorf("next follow-up: %w", err)
	}
	if parentID.Valid {
		q.ParentQuestionID = &parentID.Int64
	}
	q.IsFollowUp = followUp == 1
	anchors, err := d.loadAnchors(ctx, q.ID)
	if err != nil {
		return Question{}, err
	}
	q.RepoAnchors = anchors
	return q, nil
}

func (d *DB) loadAnchors(ctx context.Context, questionID int64) ([]RepoAnchor, error) {
	rows, err := d.db.QueryContext(ctx, `
SELECT path, note
FROM question_repo_anchors
WHERE question_id = ?
ORDER BY position ASC, id ASC;
`, questionID)
	if err != nil {
		return nil, fmt.Errorf("load question anchors: %w", err)
	}
	defer rows.Close()

	var anchors []RepoAnchor
	for rows.Next() {
		var anchor RepoAnchor
		if err := rows.Scan(&anchor.Path, &anchor.Note); err != nil {
			return nil, fmt.Errorf("scan question anchor: %w", err)
		}
		anchors = append(anchors, anchor)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load question anchors: %w", err)
	}
	return anchors, nil
}

func (d *DB) SubmitAnswer(ctx context.Context, in SubmitAnswerInput) (AnswerAttempt, error) {
	var expected string
	if err := d.db.QueryRowContext(ctx, `SELECT expected_answer FROM questions WHERE id = ? AND active = 1`, in.QuestionID).Scan(&expected); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AnswerAttempt{}, ErrNotFound
		}
		return AnswerAttempt{}, fmt.Errorf("load expected answer: %w", err)
	}
	res, err := d.db.ExecContext(ctx, `
INSERT INTO answer_attempts (session_id, question_id, answer, expected_answer_snapshot)
VALUES (?, ?, ?, ?);
`, in.SessionID, in.QuestionID, in.Answer, expected)
	if err != nil {
		return AnswerAttempt{}, fmt.Errorf("submit answer: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return AnswerAttempt{}, fmt.Errorf("answer id: %w", err)
	}
	return AnswerAttempt{
		ID:                     id,
		SessionID:              in.SessionID,
		QuestionID:             in.QuestionID,
		Answer:                 in.Answer,
		ExpectedAnswerSnapshot: expected,
		CreatedAt:              time.Now().UTC(),
	}, nil
}

func (d *DB) RecordFeedback(ctx context.Context, in FeedbackInput) error {
	if in.Score < 0 || in.Score > 3 {
		return fmt.Errorf("score must be between 0 and 3")
	}
	var exists int
	if err := d.db.QueryRowContext(ctx, `SELECT 1 FROM answer_attempts WHERE id = ?;`, in.AttemptID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("load answer attempt: %w", err)
	}
	_, err := d.db.ExecContext(ctx, `
INSERT INTO feedback (attempt_id, score, missing_points, inaccurate_points, suggested_answer)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(attempt_id) DO UPDATE SET
	score = excluded.score,
	missing_points = excluded.missing_points,
	inaccurate_points = excluded.inaccurate_points,
	suggested_answer = excluded.suggested_answer;
`, in.AttemptID, in.Score, in.MissingPoints, in.InaccuratePoints, in.SuggestedAnswer)
	if err != nil {
		return fmt.Errorf("record feedback: %w", err)
	}
	return nil
}

func (d *DB) UpdateExpectedAnswer(ctx context.Context, id int64, answer string) error {
	res, err := d.db.ExecContext(ctx, `
UPDATE questions SET expected_answer = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND active = 1;
`, answer, id)
	if err != nil {
		return fmt.Errorf("update expected answer: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("expected answer rows affected: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (d *DB) ProgressSummary(ctx context.Context) (ProgressSummary, error) {
	var summary ProgressSummary
	if err := d.db.QueryRowContext(ctx, `
SELECT COUNT(aa.id), COALESCE(AVG(f.score), 0)
FROM answer_attempts aa
LEFT JOIN feedback f ON f.attempt_id = aa.id;
`).Scan(&summary.TotalAttempts, &summary.AverageScore); err != nil {
		return ProgressSummary{}, fmt.Errorf("summary totals: %w", err)
	}

	recent, err := d.recentAttempts(ctx)
	if err != nil {
		return ProgressSummary{}, err
	}
	weak, err := d.weakTopics(ctx)
	if err != nil {
		return ProgressSummary{}, err
	}
	summary.RecentAttempts = recent
	summary.WeakTopics = weak
	return summary, nil
}

func (d *DB) recentAttempts(ctx context.Context) ([]RecentAttempt, error) {
	rows, err := d.db.QueryContext(ctx, `
SELECT aa.id, aa.question_id, q.topic, q.prompt, aa.answer, f.score, aa.created_at
FROM answer_attempts aa
JOIN questions q ON q.id = aa.question_id
LEFT JOIN feedback f ON f.attempt_id = aa.id
ORDER BY aa.id DESC
LIMIT 10;
`)
	if err != nil {
		return nil, fmt.Errorf("recent attempts: %w", err)
	}
	defer rows.Close()

	var out []RecentAttempt
	for rows.Next() {
		var item RecentAttempt
		var score sql.NullInt64
		var created string
		if err := rows.Scan(&item.AttemptID, &item.QuestionID, &item.Topic, &item.Prompt, &item.Answer, &score, &created); err != nil {
			return nil, fmt.Errorf("scan recent attempt: %w", err)
		}
		if score.Valid {
			s := int(score.Int64)
			item.Score = &s
		}
		item.CreatedAt = parseSQLiteTime(created)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (d *DB) weakTopics(ctx context.Context) ([]WeakTopic, error) {
	rows, err := d.db.QueryContext(ctx, `
SELECT q.topic, AVG(f.score), COUNT(f.id)
FROM feedback f
JOIN answer_attempts aa ON aa.id = f.attempt_id
JOIN questions q ON q.id = aa.question_id
GROUP BY q.topic
ORDER BY AVG(f.score) ASC, COUNT(f.id) DESC
LIMIT 5;
`)
	if err != nil {
		return nil, fmt.Errorf("weak topics: %w", err)
	}
	defer rows.Close()

	var out []WeakTopic
	for rows.Next() {
		var topic WeakTopic
		if err := rows.Scan(&topic.Topic, &topic.AverageScore, &topic.Attempts); err != nil {
			return nil, fmt.Errorf("scan weak topic: %w", err)
		}
		out = append(out, topic)
	}
	return out, rows.Err()
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func parseSQLiteTime(value string) time.Time {
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}
