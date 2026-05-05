# Interview Prep MCP Question Bank Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Split QA and coding exercise material cleanly, add structured repo anchors, and make tier 2 follow-ups contextual questions with durable expected answers.

**Architecture:** Keep the two MCP services separate, but apply the same content concepts in both: repo anchors, parent-linked follow-ups, and payloads that expose anchors. Start with parser tests, then store migrations, service selection behavior, MCP JSON/workflow updates, and finally markdown content migration.

**Tech Stack:** Go, modernc SQLite, Model Context Protocol Go SDK, markdown parser logic in existing `internal/content/parser.go` files, repository markdown under `docs/interview-prep`.

---

## File Structure

- Modify `go/qa-mcp-service/internal/content/parser.go`: parse repo anchors, parse block follow-ups with expected answers, ignore `00-role-map.md`, and skip `## Coding Exercises`.
- Modify `go/qa-mcp-service/internal/content/parser_test.go`: cover QA parser behavior for anchors, role-map exclusion, coding-section skipping, and parent follow-up blocks.
- Modify `go/qa-mcp-service/internal/store/sqlite.go`: add anchor and parent-link storage, load anchors with questions, exclude standalone follow-ups from normal next selection, and add follow-up lookup helpers.
- Modify `go/qa-mcp-service/internal/store/sqlite_test.go`: cover migrations, anchor persistence, parent linkage, base selection, and contextual follow-up selection.
- Modify `go/qa-mcp-service/internal/qa/service.go`: add a submit-and-next service method that can choose a contextual follow-up after strong feedback.
- Modify `go/qa-mcp-service/internal/qa/service_test.go`: cover service routing for strong and weak tier 2 answers.
- Modify `go/qa-mcp-service/internal/mcpserver/server.go`: expose repo anchors and parent context in JSON payloads and update workflow instructions.
- Modify `go/qa-mcp-service/internal/mcpserver/server_test.go`: cover payload shape and workflow text.
- Modify `go/coding-exercises-mcp-service/internal/content/parser.go`: import all direct markdown files, parse repo anchors, and parent-link coding follow-ups.
- Modify `go/coding-exercises-mcp-service/internal/content/parser_test.go`: cover multi-file import and anchors.
- Modify `go/coding-exercises-mcp-service/internal/store/sqlite.go`: mirror anchor and parent-link persistence.
- Modify `go/coding-exercises-mcp-service/internal/store/sqlite_test.go`: mirror persistence and payload behavior tests.
- Modify `go/coding-exercises-mcp-service/internal/mcpserver/server.go`: expose repo anchors and parent context in coding payloads and update workflow instructions.
- Modify `go/coding-exercises-mcp-service/internal/mcpserver/server_test.go`: cover coding payload shape and workflow text.
- Modify `docs/interview-prep/qa/*.md`: remove embedded coding exercise sections, convert follow-up bullets to answer-key blocks, and add repo anchors.
- Modify `docs/interview-prep/coding-exercises/*.md`: add migrated coding exercise material, expected designs, follow-up answers, and repo anchors.

## Task 1: QA Parser Contract

**Files:**
- Modify: `go/qa-mcp-service/internal/content/parser.go`
- Modify: `go/qa-mcp-service/internal/content/parser_test.go`

- [ ] **Step 1: Add failing parser tests for role-map exclusion, coding-section skipping, repo anchors, and follow-up answer blocks**

Add these tests to `go/qa-mcp-service/internal/content/parser_test.go`:

```go
func TestParseDirIgnoresRoleMap(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"00-role-map.md": "# Role Map\n\n### 1. Should this import?\n\nFast answer:\n\n> No.\n",
		"01-portfolio-recall-matrix.md": "# Portfolio Recall Matrix\n\n### Tell me about a Go backend system you built.\n\nFast answer:\n\n> I built decomposed Go services.\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatalf("write fixture %s: %v", name, err)
		}
	}

	questions, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("ParseDir returned error: %v", err)
	}
	if len(questions) != 1 {
		t.Fatalf("expected only portfolio question, got %d: %#v", len(questions), questions)
	}
	if filepath.Base(questions[0].SourcePath) == "00-role-map.md" {
		t.Fatalf("role map imported as QA: %#v", questions[0])
	}
}

func TestParseFileSkipsCodingExercisesSection(t *testing.T) {
	data := []byte(`# REST API And API Gateway Rehearsal

### 1. How do you design a good REST API?

Fast answer:

> Use resource-oriented routes and consistent errors.

## Coding Exercises

### Exercise 1: Idempotent Create Endpoint

Prompt:

> Implement an endpoint.

Fast design:

> Use an idempotency key.
`)

	questions, err := ParseFile("03-rest-api-gateway-questions.md", data)
	if err != nil {
		t.Fatalf("ParseFile returned error: %v", err)
	}
	if len(questions) != 1 {
		t.Fatalf("expected only QA prompt, got %d: %#v", len(questions), questions)
	}
	if strings.Contains(questions[0].Prompt, "Idempotent Create Endpoint") {
		t.Fatalf("coding exercise leaked into QA: %#v", questions[0])
	}
}

func TestParseFileExtractsRepoAnchorsAndFollowUpAnswer(t *testing.T) {
	data := []byte(`# Go Language Fundamentals Rehearsal

### 3. How do maps work under concurrency?

Fast answer:

> A Go map is not safe for concurrent writes without synchronization.

Repo anchors:
- `go/pkg/resilience` - Shows shared concurrency-safe helpers.

Follow-ups:

#### Follow-up: When is sync.Map appropriate?

Fast answer:

> sync.Map fits read-heavy shared maps with stable keys or disjoint key ownership.

Repo anchors:
- `go/analytics-service` - Shows concurrent consumer state that should stay explicit.
`)

	questions, err := ParseFile("02-go-language-fundamentals.md", data)
	if err != nil {
		t.Fatalf("ParseFile returned error: %v", err)
	}
	if len(questions) != 2 {
		t.Fatalf("expected base plus follow-up, got %d: %#v", len(questions), questions)
	}
	base := questions[0]
	if len(base.RepoAnchors) != 1 || base.RepoAnchors[0].Path != "go/pkg/resilience" {
		t.Fatalf("unexpected base anchors: %#v", base.RepoAnchors)
	}
	follow := questions[1]
	if !follow.IsFollowUp {
		t.Fatalf("expected follow-up: %#v", follow)
	}
	if follow.ParentPrompt != "How do maps work under concurrency?" {
		t.Fatalf("unexpected parent prompt: %q", follow.ParentPrompt)
	}
	if follow.ExpectedAnswer == "" {
		t.Fatalf("expected follow-up answer to be parsed")
	}
	if len(follow.RepoAnchors) != 1 || follow.RepoAnchors[0].Path != "go/analytics-service" {
		t.Fatalf("unexpected follow-up anchors: %#v", follow.RepoAnchors)
	}
}
```

- [ ] **Step 2: Run QA parser tests and verify they fail**

Run:

```bash
cd go/qa-mcp-service
go test ./internal/content -run 'TestParseDirIgnoresRoleMap|TestParseFileSkipsCodingExercisesSection|TestParseFileExtractsRepoAnchorsAndFollowUpAnswer' -count=1
```

Expected: fail because `RepoAnchor`, `RepoAnchors`, and `ParentPrompt` do not exist and the parser still imports role-map/coding sections.

- [ ] **Step 3: Add parser data fields**

In `go/qa-mcp-service/internal/content/parser.go`, extend `Question` and add `RepoAnchor`:

```go
type RepoAnchor struct {
	Path string
	Note string
}

type Question struct {
	SourcePath     string
	Topic          string
	Category       string
	Kind           string
	Prompt         string
	ExpectedAnswer string
	IsFollowUp     bool
	ParentPrompt   string
	RepoAnchors    []RepoAnchor
	Priority       int
	Tier           int
}
```

- [ ] **Step 4: Update ParseDir role-map filtering**

Change the `ParseDir` file filter to skip both `00-role-map.md` and `08-coding-exercises.md`:

```go
name := entry.Name()
if entry.IsDir() || filepath.Ext(name) != ".md" || name == "00-role-map.md" || name == "08-coding-exercises.md" {
	continue
}
```

- [ ] **Step 5: Add helper functions for repo anchors**

Add these helpers near `cleanPrompt`:

```go
func cleanAnchorLine(line string) (RepoAnchor, bool) {
	line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "- "))
	if line == "" {
		return RepoAnchor{}, false
	}
	if !strings.HasPrefix(line, "`") {
		return RepoAnchor{}, false
	}
	rest := strings.TrimPrefix(line, "`")
	parts := strings.SplitN(rest, "`", 2)
	if len(parts) != 2 {
		return RepoAnchor{}, false
	}
	path := strings.TrimSpace(parts[0])
	note := strings.TrimSpace(parts[1])
	note = strings.TrimSpace(strings.TrimPrefix(note, "-"))
	if path == "" {
		return RepoAnchor{}, false
	}
	return RepoAnchor{Path: path, Note: note}, true
}

func cloneAnchors(in []RepoAnchor) []RepoAnchor {
	if len(in) == 0 {
		return nil
	}
	out := make([]RepoAnchor, len(in))
	copy(out, in)
	return out
}
```

- [ ] **Step 6: Replace parser state with modes for answers, anchors, and block follow-ups**

Update `ParseFile` so it tracks `anchorLines`, parent prompt, and a `skipSection` flag. The implementation should follow this shape:

```go
var current *Question
var lastBasePrompt string
var lastBaseAnchors []RepoAnchor
var promptLines []string
var answerLines []string
var anchorLines []string
mode := ""
skipSection := false

flush := func() {
	if current == nil {
		return
	}
	if prompt := cleanPrompt(promptLines); prompt != "" {
		current.Prompt = prompt
		current.Tier = tierForQuestion(current.SourcePath, current.Prompt, current.IsFollowUp)
	}
	current.ExpectedAnswer = cleanAnswer(answerLines)
	var anchors []RepoAnchor
	for _, raw := range anchorLines {
		if anchor, ok := cleanAnchorLine(raw); ok {
			anchors = append(anchors, anchor)
		}
	}
	if len(anchors) == 0 && current.IsFollowUp {
		anchors = cloneAnchors(lastBaseAnchors)
	}
	current.RepoAnchors = anchors
	if current.Prompt != "" {
		questions = append(questions, *current)
		if !current.IsFollowUp {
			lastBasePrompt = current.Prompt
			lastBaseAnchors = cloneAnchors(current.RepoAnchors)
		}
	}
	current = nil
	promptLines = nil
	answerLines = nil
	anchorLines = nil
	mode = ""
}
```

In the line loop, add these cases:

```go
if strings.HasPrefix(line, "## ") {
	flush()
	skipSection = line == "## Coding Exercises"
	continue
}
if skipSection {
	continue
}
if strings.HasPrefix(line, "#### Follow-up: ") {
	flush()
	prompt := strings.TrimSpace(strings.TrimPrefix(line, "#### Follow-up: "))
	current = &Question{
		SourcePath:   path,
		Topic:        topic,
		Category:     categoryForPath(path),
		Kind:         kindForQuestion(path, true),
		Prompt:       prompt,
		IsFollowUp:   true,
		ParentPrompt: lastBasePrompt,
		Priority:     priorityForPath(path),
		Tier:         tierForQuestion(path, prompt, true),
	}
	continue
}
```

Keep legacy bullet follow-ups for now, but set `ParentPrompt` and inherited anchors:

```go
questions = append(questions, Question{
	SourcePath:   path,
	Topic:        topic,
	Category:     categoryForPath(path),
	Kind:         kindForQuestion(path, true),
	Prompt:       prompt,
	IsFollowUp:   true,
	ParentPrompt: lastBasePrompt,
	RepoAnchors:  cloneAnchors(lastBaseAnchors),
	Priority:     priorityForPath(path),
	Tier:         tierForQuestion(path, prompt, true),
})
```

Add anchor mode:

```go
case line == "Repo anchors:":
	mode = "anchors"
case mode == "anchors" && strings.HasPrefix(line, "- "):
	anchorLines = append(anchorLines, raw)
```

- [ ] **Step 7: Run QA parser tests and verify they pass**

Run:

```bash
cd go/qa-mcp-service
go test ./internal/content -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit Task 1**

```bash
git add go/qa-mcp-service/internal/content/parser.go go/qa-mcp-service/internal/content/parser_test.go
git commit -m "feat: parse QA repo anchors and contextual follow-ups"
```

If git cannot create `.git/index.lock`, record the exact error and continue without committing.

## Task 2: QA Store Anchors And Follow-Up Selection

**Files:**
- Modify: `go/qa-mcp-service/internal/store/sqlite.go`
- Modify: `go/qa-mcp-service/internal/store/sqlite_test.go`

- [ ] **Step 1: Add failing store tests**

Add tests to `go/qa-mcp-service/internal/store/sqlite_test.go`:

```go
func TestUpsertQuestionsPersistsRepoAnchorsAndParentLink(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	err := db.UpsertQuestions(ctx, []content.Question{
		{
			SourcePath: "qa.md",
			Topic: "Go",
			Category: "golang",
			Kind: "qa",
			Prompt: "How do maps work under concurrency?",
			ExpectedAnswer: "Use synchronization.",
			Tier: 1,
			RepoAnchors: []content.RepoAnchor{{Path: "go/pkg/resilience", Note: "Shows shared helpers."}},
		},
		{
			SourcePath: "qa.md",
			Topic: "Go",
			Category: "golang",
			Kind: "qa",
			Prompt: "When is sync.Map appropriate?",
			ExpectedAnswer: "Read-heavy stable-key maps.",
			IsFollowUp: true,
			ParentPrompt: "How do maps work under concurrency?",
			Tier: 2,
		},
	})
	if err != nil {
		t.Fatalf("UpsertQuestions returned error: %v", err)
	}

	base, err := db.NextQuestion(ctx, QuestionFilter{Tier: 1, Category: "golang"})
	if err != nil {
		t.Fatalf("NextQuestion returned error: %v", err)
	}
	if len(base.RepoAnchors) != 1 || base.RepoAnchors[0].Path != "go/pkg/resilience" {
		t.Fatalf("unexpected base anchors: %#v", base.RepoAnchors)
	}

	follow, err := db.NextFollowUp(ctx, base.ID)
	if err != nil {
		t.Fatalf("NextFollowUp returned error: %v", err)
	}
	if follow.ParentQuestionID == nil || *follow.ParentQuestionID != base.ID {
		t.Fatalf("follow-up missing parent id: %#v", follow)
	}
	if follow.ParentPrompt != base.Prompt {
		t.Fatalf("unexpected parent prompt: %q", follow.ParentPrompt)
	}
	if len(follow.RepoAnchors) != 1 || follow.RepoAnchors[0].Path != "go/pkg/resilience" {
		t.Fatalf("expected inherited anchors: %#v", follow.RepoAnchors)
	}
}

func TestNextQuestionDoesNotReturnStandaloneFollowUps(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	if err := db.UpsertQuestions(ctx, []content.Question{
		{SourcePath: "qa.md", Topic: "Go", Category: "golang", Kind: "qa", Prompt: "Base?", ExpectedAnswer: "Base.", Tier: 2},
		{SourcePath: "qa.md", Topic: "Go", Category: "golang", Kind: "qa", Prompt: "Follow?", ExpectedAnswer: "Follow.", IsFollowUp: true, ParentPrompt: "Base?", Tier: 2},
	}); err != nil {
		t.Fatalf("UpsertQuestions returned error: %v", err)
	}

	q, err := db.NextQuestion(ctx, QuestionFilter{Tier: 2, Category: "golang"})
	if err != nil {
		t.Fatalf("NextQuestion returned error: %v", err)
	}
	if q.IsFollowUp {
		t.Fatalf("NextQuestion returned standalone follow-up: %#v", q)
	}
}
```

- [ ] **Step 2: Run store tests and verify they fail**

Run:

```bash
cd go/qa-mcp-service
go test ./internal/store -run 'TestUpsertQuestionsPersistsRepoAnchorsAndParentLink|TestNextQuestionDoesNotReturnStandaloneFollowUps' -count=1
```

Expected: fail because `RepoAnchors`, `ParentQuestionID`, `ParentPrompt`, and `NextFollowUp` do not exist in store.

- [ ] **Step 3: Add store types**

In `go/qa-mcp-service/internal/store/sqlite.go`, add:

```go
type RepoAnchor struct {
	Path string `json:"path"`
	Note string `json:"note"`
}
```

Extend `Question`:

```go
ParentQuestionID *int64       `json:"parent_question_id,omitempty"`
ParentPrompt     string       `json:"parent_prompt,omitempty"`
RepoAnchors      []RepoAnchor `json:"repo_anchors,omitempty"`
```

- [ ] **Step 4: Add migration tables and columns**

In `Migrate`, add `parent_question_id` and `parent_prompt` columns through `ensureColumn`, then create anchors table:

```go
if err := d.ensureColumn(ctx, "questions", "parent_question_id", "INTEGER"); err != nil {
	return err
}
if err := d.ensureColumn(ctx, "questions", "parent_prompt", "TEXT NOT NULL DEFAULT ''"); err != nil {
	return err
}
if _, err := d.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS question_repo_anchors (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	question_id INTEGER NOT NULL REFERENCES questions(id) ON DELETE CASCADE,
	path TEXT NOT NULL,
	note TEXT NOT NULL DEFAULT '',
	position INTEGER NOT NULL DEFAULT 0,
	UNIQUE(question_id, path, note)
);
`); err != nil {
	return fmt.Errorf("create question_repo_anchors: %w", err)
}
```

- [ ] **Step 5: Update upsert to persist parent prompt and anchors**

Update the upsert SQL to include `parent_prompt`. After each question upsert, query the question ID and replace anchors:

```go
var id int64
if err := tx.QueryRowContext(ctx, `
SELECT id FROM questions WHERE source_path = ? AND prompt = ? AND is_follow_up = ?;
`, q.SourcePath, q.Prompt, boolInt(q.IsFollowUp)).Scan(&id); err != nil {
	return fmt.Errorf("load upserted question %q: %w", q.Prompt, err)
}
if _, err := tx.ExecContext(ctx, `DELETE FROM question_repo_anchors WHERE question_id = ?;`, id); err != nil {
	return fmt.Errorf("clear anchors for %q: %w", q.Prompt, err)
}
for pos, anchor := range q.RepoAnchors {
	if _, err := tx.ExecContext(ctx, `
INSERT INTO question_repo_anchors (question_id, path, note, position) VALUES (?, ?, ?, ?);
`, id, anchor.Path, anchor.Note, pos); err != nil {
		return fmt.Errorf("insert anchor for %q: %w", q.Prompt, err)
	}
}
```

After all questions are upserted, resolve parent IDs:

```go
if _, err := tx.ExecContext(ctx, `
UPDATE questions AS child
SET parent_question_id = (
	SELECT parent.id
	FROM questions AS parent
	WHERE parent.source_path = child.source_path
	  AND parent.prompt = child.parent_prompt
	  AND parent.is_follow_up = 0
	LIMIT 1
)
WHERE child.is_follow_up = 1 AND child.parent_prompt != '';
`); err != nil {
	return fmt.Errorf("resolve follow-up parents: %w", err)
}
```

- [ ] **Step 6: Add anchor loading helpers**

Add:

```go
func (d *DB) loadAnchors(ctx context.Context, questionID int64) ([]RepoAnchor, error) {
	rows, err := d.db.QueryContext(ctx, `
SELECT path, note FROM question_repo_anchors WHERE question_id = ? ORDER BY position, id;
`, questionID)
	if err != nil {
		return nil, fmt.Errorf("load anchors: %w", err)
	}
	defer rows.Close()
	var anchors []RepoAnchor
	for rows.Next() {
		var anchor RepoAnchor
		if err := rows.Scan(&anchor.Path, &anchor.Note); err != nil {
			return nil, fmt.Errorf("scan anchor: %w", err)
		}
		anchors = append(anchors, anchor)
	}
	return anchors, rows.Err()
}
```

Call this helper in `NextQuestion` and `NextFollowUp`.

- [ ] **Step 7: Exclude follow-ups from normal next selection**

In `NextQuestion`, add:

```sql
AND q.is_follow_up = 0
```

Keep the existing tier/category filters.

- [ ] **Step 8: Add NextFollowUp**

Add:

```go
func (d *DB) NextFollowUp(ctx context.Context, parentQuestionID int64) (Question, error) {
	row := d.db.QueryRowContext(ctx, `
SELECT child.id, child.source_path, child.topic, child.category, child.kind, child.prompt,
       child.expected_answer, child.is_follow_up, child.priority, child.tier,
       child.parent_question_id, parent.prompt
FROM questions child
JOIN questions parent ON parent.id = child.parent_question_id
LEFT JOIN answer_attempts aa ON aa.question_id = child.id
LEFT JOIN feedback f ON f.attempt_id = aa.id
WHERE child.active = 1 AND child.is_follow_up = 1 AND child.parent_question_id = ?
GROUP BY child.id
ORDER BY
	CASE WHEN COUNT(aa.id) = 0 THEN 0 ELSE 1 END,
	COALESCE(MIN(f.score), 4) ASC,
	MAX(aa.created_at) ASC,
	child.id ASC
LIMIT 1;
`, parentQuestionID)
	var q Question
	var followUp int
	var parentID sql.NullInt64
	if err := row.Scan(&q.ID, &q.SourcePath, &q.Topic, &q.Category, &q.Kind, &q.Prompt, &q.ExpectedAnswer, &followUp, &q.Priority, &q.Tier, &parentID, &q.ParentPrompt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Question{}, ErrNotFound
		}
		return Question{}, fmt.Errorf("next follow-up: %w", err)
	}
	q.IsFollowUp = followUp == 1
	if parentID.Valid {
		id := parentID.Int64
		q.ParentQuestionID = &id
	}
	anchors, err := d.loadAnchors(ctx, q.ID)
	if err != nil {
		return Question{}, err
	}
	q.RepoAnchors = anchors
	return q, nil
}
```

- [ ] **Step 9: Run store tests**

Run:

```bash
cd go/qa-mcp-service
go test ./internal/store -count=1
```

Expected: PASS.

- [ ] **Step 10: Commit Task 2**

```bash
git add go/qa-mcp-service/internal/store/sqlite.go go/qa-mcp-service/internal/store/sqlite_test.go
git commit -m "feat: store QA anchors and contextual follow-ups"
```

If git cannot create `.git/index.lock`, record the exact error and continue without committing.

## Task 3: QA Service And MCP Tier 2 Flow

**Files:**
- Modify: `go/qa-mcp-service/internal/qa/service.go`
- Modify: `go/qa-mcp-service/internal/qa/service_test.go`
- Modify: `go/qa-mcp-service/internal/mcpserver/server.go`
- Modify: `go/qa-mcp-service/internal/mcpserver/server_test.go`

- [ ] **Step 1: Add failing service test for contextual follow-up after strong tier 2 answer**

In `go/qa-mcp-service/internal/qa/service_test.go`, extend the fake store with `NextFollowUp`, then add:

```go
func TestPrepareNextReturnsFollowUpForStrongTierTwoAnswer(t *testing.T) {
	parentID := int64(7)
	fake := &fakeStore{
		attempt: store.AnswerAttempt{ID: 11, QuestionID: parentID, Answer: "good", ExpectedAnswerSnapshot: "expected"},
		followUp: store.Question{ID: 8, Prompt: "When is sync.Map appropriate?", IsFollowUp: true, ParentQuestionID: &parentID, ParentPrompt: "How do maps work?"},
	}
	svc := New(fake, "unused")

	turn, err := svc.SubmitAnswerAndPrepareNext(context.Background(), SubmitAndNextInput{
		QuestionID: parentID,
		Answer: "good",
		Tier: 2,
		Category: "golang",
		CurrentScore: 3,
	})
	if err != nil {
		t.Fatalf("SubmitAnswerAndPrepareNext returned error: %v", err)
	}
	if !turn.NextQuestion.IsFollowUp || turn.NextQuestion.ID != 8 {
		t.Fatalf("expected contextual follow-up, got %#v", turn.NextQuestion)
	}
}
```

- [ ] **Step 2: Add failing service test for weak tier 2 answer**

Add:

```go
func TestPrepareNextSkipsFollowUpForWeakTierTwoAnswer(t *testing.T) {
	fake := &fakeStore{
		attempt: store.AnswerAttempt{ID: 11, QuestionID: 7, Answer: "weak", ExpectedAnswerSnapshot: "expected"},
		next: store.Question{ID: 9, Prompt: "How should errors be handled in Go?", Tier: 2},
	}
	svc := New(fake, "unused")

	turn, err := svc.SubmitAnswerAndPrepareNext(context.Background(), SubmitAndNextInput{
		QuestionID: 7,
		Answer: "weak",
		Tier: 2,
		Category: "golang",
		CurrentScore: 1,
	})
	if err != nil {
		t.Fatalf("SubmitAnswerAndPrepareNext returned error: %v", err)
	}
	if turn.NextQuestion.IsFollowUp || turn.NextQuestion.ID != 9 {
		t.Fatalf("expected next base question, got %#v", turn.NextQuestion)
	}
}
```

- [ ] **Step 3: Run QA service tests and verify they fail**

Run:

```bash
cd go/qa-mcp-service
go test ./internal/qa -run 'TestPrepareNextReturnsFollowUpForStrongTierTwoAnswer|TestPrepareNextSkipsFollowUpForWeakTierTwoAnswer' -count=1
```

Expected: fail because the new service method and store interface do not exist.

- [ ] **Step 4: Add QA service types and method**

In `go/qa-mcp-service/internal/qa/service.go`, extend the store interface:

```go
NextFollowUp(context.Context, int64) (store.Question, error)
```

Add:

```go
type SubmitAndNextInput struct {
	QuestionID    int64
	Answer        string
	Tier          int
	Category      string
	CurrentScore  int
}

type SubmitAndNextResult struct {
	Review       AnswerReview    `json:"review"`
	NextQuestion store.Question  `json:"next_question"`
}
```

Add method:

```go
func (s *Service) SubmitAnswerAndPrepareNext(ctx context.Context, in SubmitAndNextInput) (SubmitAndNextResult, error) {
	review, err := s.SubmitAnswer(ctx, in.QuestionID, in.Answer)
	if err != nil {
		return SubmitAndNextResult{}, err
	}
	if in.Tier == 2 && in.CurrentScore >= 2 {
		if followUp, err := s.store.NextFollowUp(ctx, in.QuestionID); err == nil {
			return SubmitAndNextResult{Review: review, NextQuestion: followUp}, nil
		} else if !errors.Is(err, store.ErrNotFound) {
			return SubmitAndNextResult{}, err
		}
	}
	next, err := s.store.NextQuestion(ctx, store.QuestionFilter{Tier: in.Tier, Category: in.Category})
	if err != nil {
		return SubmitAndNextResult{}, err
	}
	return SubmitAndNextResult{Review: review, NextQuestion: next}, nil
}
```

Import `errors`.

- [ ] **Step 5: Update MCP submit-and-next schema and handler**

In `go/qa-mcp-service/internal/mcpserver/server.go`, extend `QAService`:

```go
SubmitAnswerAndPrepareNext(context.Context, qa.SubmitAndNextInput) (qa.SubmitAndNextResult, error)
```

Add `CurrentScore` to the handler input:

```go
CurrentScore int `json:"current_score,omitempty"`
```

Validate it:

```go
if in.Tier == 2 && (in.CurrentScore < 0 || in.CurrentScore > 3) {
	return toolError("current_score must be between 0 and 3"), nil
}
```

Replace the manual `SubmitAnswer` plus `GetNextQuestion` call with:

```go
turn, err := service.SubmitAnswerAndPrepareNext(ctx, qa.SubmitAndNextInput{
	QuestionID: in.QuestionID,
	Answer: in.Answer,
	Tier: in.Tier,
	Category: category,
	CurrentScore: in.CurrentScore,
})
if err != nil {
	return toolError(err.Error()), nil
}
return jsonResult(qaTurn{
	Review: turn.Review,
	NextQuestion: turn.NextQuestion,
	Tier: in.Tier,
	Category: category,
	PreviousFeedbackRecorded: previousFeedbackRecorded,
}), nil
```

- [ ] **Step 6: Update workflow instructions**

In `qaWorkflowInstructions`, replace the tier/follow-up wording with:

```text
For tier 2, ask a base question first. After grading the base answer, pass current_score to submit_qa_answer_and_prepare_next. If the score is 2 or 3 and a parent-linked follow-up exists, the tool may return that follow-up with parent context. Ask that follow-up as a continuation of the parent. If the score is 0 or 1, teach the gap and use the returned base question instead.
```

Add:

```text
Use next_question.repo_anchors as portfolio citations. In Explanation and Interview answer, cite the concrete repo paths that show where the concept applies.
```

- [ ] **Step 7: Update MCP tests**

In `go/qa-mcp-service/internal/mcpserver/server_test.go`, update fake service and assertions:

```go
func (f *fakeQA) SubmitAnswerAndPrepareNext(_ context.Context, in qa.SubmitAndNextInput) (qa.SubmitAndNextResult, error) {
	f.submittedQuestionID = in.QuestionID
	f.submittedAnswer = in.Answer
	f.nextFilter = store.QuestionFilter{Tier: in.Tier, Category: in.Category}
	return qa.SubmitAndNextResult{
		Review: qa.AnswerReview{AttemptID: 9, QuestionID: in.QuestionID, Answer: in.Answer, ExpectedAnswer: "Use synchronization."},
		NextQuestion: store.Question{ID: 4, Prompt: "When is sync.Map appropriate?", IsFollowUp: true, RepoAnchors: []store.RepoAnchor{{Path: "go/pkg/resilience", Note: "Shows helpers."}}},
	}, nil
}
```

Assert JSON includes `repo_anchors`:

```go
next := raw["next_question"].(map[string]any)
anchors := next["repo_anchors"].([]any)
if anchors[0].(map[string]any)["path"] != "go/pkg/resilience" {
	t.Fatalf("unexpected repo anchors: %#v", anchors)
}
```

- [ ] **Step 8: Run QA service and MCP tests**

Run:

```bash
cd go/qa-mcp-service
go test ./internal/qa ./internal/mcpserver -count=1
```

Expected: PASS.

- [ ] **Step 9: Commit Task 3**

```bash
git add go/qa-mcp-service/internal/qa/service.go go/qa-mcp-service/internal/qa/service_test.go go/qa-mcp-service/internal/mcpserver/server.go go/qa-mcp-service/internal/mcpserver/server_test.go
git commit -m "feat: serve contextual QA follow-ups"
```

If git cannot create `.git/index.lock`, record the exact error and continue without committing.

## Task 4: Coding Exercises Parser And Store Parity

**Files:**
- Modify: `go/coding-exercises-mcp-service/internal/content/parser.go`
- Modify: `go/coding-exercises-mcp-service/internal/content/parser_test.go`
- Modify: `go/coding-exercises-mcp-service/internal/store/sqlite.go`
- Modify: `go/coding-exercises-mcp-service/internal/store/sqlite_test.go`

- [ ] **Step 1: Add failing coding parser tests**

In `go/coding-exercises-mcp-service/internal/content/parser_test.go`, add:

```go
func TestParseDirImportsAllMarkdownFilesInSortedOrder(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"09-concurrency.md": "# Concurrency\n\n### 1. Worker pool\n\nPrompt:\n\n> Build a worker pool.\n\nFast design:\n\n> Use workers and cancellation.\n",
		"08-coding-exercises.md": "# Timed\n\n### 1. Retry client\n\nPrompt:\n\n> Build a retry client.\n\nFast design:\n\n> Use bounded retries.\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatalf("write fixture %s: %v", name, err)
		}
	}

	questions, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("ParseDir returned error: %v", err)
	}
	if len(questions) != 2 {
		t.Fatalf("expected two exercises, got %d: %#v", len(questions), questions)
	}
	if filepath.Base(questions[0].SourcePath) != "08-coding-exercises.md" {
		t.Fatalf("expected lexical order, got %q", questions[0].SourcePath)
	}
}

func TestParseFileExtractsCodingRepoAnchors(t *testing.T) {
	data := []byte(`# Timed Coding Exercises

### 5. Retryable HTTP client

Prompt:

> Build a client that retries retryable errors and respects context cancellation.

Fast design:

> Use context-aware attempts and backoff.

Repo anchors:
- `go/pkg/resilience` - Contains retry helpers used by services.
`)

	questions, err := ParseFile("08-coding-exercises.md", data)
	if err != nil {
		t.Fatalf("ParseFile returned error: %v", err)
	}
	if len(questions) != 1 {
		t.Fatalf("expected one exercise, got %d: %#v", len(questions), questions)
	}
	if len(questions[0].RepoAnchors) != 1 || questions[0].RepoAnchors[0].Path != "go/pkg/resilience" {
		t.Fatalf("unexpected anchors: %#v", questions[0].RepoAnchors)
	}
}
```

- [ ] **Step 2: Run coding parser tests and verify they fail**

Run:

```bash
cd go/coding-exercises-mcp-service
go test ./internal/content -run 'TestParseDirImportsAllMarkdownFilesInSortedOrder|TestParseFileExtractsCodingRepoAnchors' -count=1
```

Expected: fail because `ParseDir` only imports `08-coding-exercises.md` and anchors do not exist.

- [ ] **Step 3: Add coding parser fields and anchor helpers**

In `go/coding-exercises-mcp-service/internal/content/parser.go`, add:

```go
type RepoAnchor struct {
	Path string
	Note string
}
```

Extend `Question`:

```go
ParentPrompt string
RepoAnchors  []RepoAnchor
```

Add helper functions:

```go
func cleanAnchorLine(line string) (RepoAnchor, bool) {
	line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "- "))
	if line == "" || !strings.HasPrefix(line, "`") {
		return RepoAnchor{}, false
	}
	rest := strings.TrimPrefix(line, "`")
	parts := strings.SplitN(rest, "`", 2)
	if len(parts) != 2 {
		return RepoAnchor{}, false
	}
	path := strings.TrimSpace(parts[0])
	note := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(parts[1]), "-"))
	if path == "" {
		return RepoAnchor{}, false
	}
	return RepoAnchor{Path: path, Note: note}, true
}

func cloneAnchors(in []RepoAnchor) []RepoAnchor {
	if len(in) == 0 {
		return nil
	}
	out := make([]RepoAnchor, len(in))
	copy(out, in)
	return out
}
```

Update `ParseFile` to track `anchorLines`, `lastBasePrompt`, and
`lastBaseAnchors`. In `flush`, parse anchors from `anchorLines`, inherit parent
anchors for follow-ups without explicit anchors, and update `lastBasePrompt`
and `lastBaseAnchors` after flushing a base exercise. Add a line-loop case for
`Repo anchors:` and a line-loop case for `#### Follow-up: ` that creates a
follow-up `Question` with `IsFollowUp: true`, `ParentPrompt: lastBasePrompt`,
`Kind: kindForQuestion(path, true)`, and `Tier: tierForQuestion(path, prompt,
true)`.

For `ParseDir`, use:

```go
for _, entry := range entries {
	if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
		continue
	}
	paths = append(paths, filepath.Join(root, entry.Name()))
}
sort.Strings(paths)
```

Import `sort`.

- [ ] **Step 4: Run coding parser tests**

Run:

```bash
cd go/coding-exercises-mcp-service
go test ./internal/content -count=1
```

Expected: PASS.

- [ ] **Step 5: Add coding store persistence test**

In `go/coding-exercises-mcp-service/internal/store/sqlite_test.go`, add:

```go
func TestUpsertQuestionsPersistsCodingRepoAnchorsAndParentLink(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	err := db.UpsertQuestions(ctx, []content.Question{
		{
			SourcePath: "coding.md",
			Topic: "Timed Coding Exercises",
			Category: "coding",
			Kind: "coding_exercise",
			Prompt: "Build a retry client.",
			ExpectedAnswer: "Use bounded retries and context cancellation.",
			Tier: 1,
			RepoAnchors: []content.RepoAnchor{{Path: "go/pkg/resilience", Note: "Contains retry helpers."}},
		},
		{
			SourcePath: "coding.md",
			Topic: "Timed Coding Exercises",
			Category: "coding",
			Kind: "qa",
			Prompt: "Which errors should be retried?",
			ExpectedAnswer: "Retry transient transport, 429, and 5xx errors when the request is safe.",
			IsFollowUp: true,
			ParentPrompt: "Build a retry client.",
			Tier: 2,
		},
	})
	if err != nil {
		t.Fatalf("UpsertQuestions returned error: %v", err)
	}

	base, err := db.NextQuestion(ctx, QuestionFilter{Tier: 1, Category: "coding"})
	if err != nil {
		t.Fatalf("NextQuestion returned error: %v", err)
	}
	if len(base.RepoAnchors) != 1 || base.RepoAnchors[0].Path != "go/pkg/resilience" {
		t.Fatalf("unexpected anchors: %#v", base.RepoAnchors)
	}
	follow, err := db.NextFollowUp(ctx, base.ID)
	if err != nil {
		t.Fatalf("NextFollowUp returned error: %v", err)
	}
	if follow.ParentQuestionID == nil || *follow.ParentQuestionID != base.ID {
		t.Fatalf("missing parent link: %#v", follow)
	}
	if len(follow.RepoAnchors) != 1 || follow.RepoAnchors[0].Path != "go/pkg/resilience" {
		t.Fatalf("expected inherited anchors: %#v", follow.RepoAnchors)
	}
}
```

- [ ] **Step 6: Add coding store implementation**

In `go/coding-exercises-mcp-service/internal/store/sqlite.go`, add:

```go
type RepoAnchor struct {
	Path string `json:"path"`
	Note string `json:"note"`
}
```

Extend store `Question`:

```go
ParentQuestionID *int64       `json:"parent_question_id,omitempty"`
ParentPrompt     string       `json:"parent_prompt,omitempty"`
RepoAnchors      []RepoAnchor `json:"repo_anchors,omitempty"`
```

In `Migrate`, add `parent_question_id`, `parent_prompt`, and
`question_repo_anchors` exactly as the QA store does:

```go
if err := d.ensureColumn(ctx, "questions", "parent_question_id", "INTEGER"); err != nil {
	return err
}
if err := d.ensureColumn(ctx, "questions", "parent_prompt", "TEXT NOT NULL DEFAULT ''"); err != nil {
	return err
}
if _, err := d.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS question_repo_anchors (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	question_id INTEGER NOT NULL REFERENCES questions(id) ON DELETE CASCADE,
	path TEXT NOT NULL,
	note TEXT NOT NULL DEFAULT '',
	position INTEGER NOT NULL DEFAULT 0,
	UNIQUE(question_id, path, note)
);
`); err != nil {
	return fmt.Errorf("create question_repo_anchors: %w", err)
}
```

Update `UpsertQuestions` to insert `parent_prompt`, replace anchors for each
upserted question, and resolve `parent_question_id` after the import batch. Add
`loadAnchors(ctx, questionID int64) ([]RepoAnchor, error)` that selects
`path, note` from `question_repo_anchors` ordered by `position, id`. Add
`NextFollowUp(ctx, parentQuestionID int64) (Question, error)` that joins
`questions child` to `questions parent`, filters `child.is_follow_up = 1` and
`child.parent_question_id = ?`, orders unattempted follow-ups before attempted
follow-ups, scans parent context, loads anchors, and returns `store.ErrNotFound`
when no follow-up exists.

- [ ] **Step 7: Run coding store tests**

Run:

```bash
cd go/coding-exercises-mcp-service
go test ./internal/store -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit Task 4**

```bash
git add go/coding-exercises-mcp-service/internal/content/parser.go go/coding-exercises-mcp-service/internal/content/parser_test.go go/coding-exercises-mcp-service/internal/store/sqlite.go go/coding-exercises-mcp-service/internal/store/sqlite_test.go
git commit -m "feat: parse and store coding exercise anchors"
```

If git cannot create `.git/index.lock`, record the exact error and continue without committing.

## Task 5: Coding MCP Payloads And Workflow

**Files:**
- Modify: `go/coding-exercises-mcp-service/internal/coding/service.go`
- Modify: `go/coding-exercises-mcp-service/internal/mcpserver/server.go`
- Modify: `go/coding-exercises-mcp-service/internal/mcpserver/server_test.go`

- [ ] **Step 1: Update coding MCP tests for repo anchors**

In `go/coding-exercises-mcp-service/internal/mcpserver/server_test.go`, update fake next question payloads to include anchors:

```go
return store.Question{
	ID: 3,
	Topic: "Coding",
	Prompt: "Build a retry client.",
	ExpectedAnswer: "Use bounded retries.",
	Kind: "coding_exercise",
	RepoAnchors: []store.RepoAnchor{{Path: "go/pkg/resilience", Note: "Contains retry helpers."}},
	Priority: 10,
	Tier: filter.Tier,
}, nil
```

Add assertion:

```go
anchors := raw["repo_anchors"].([]any)
if anchors[0].(map[string]any)["path"] != "go/pkg/resilience" {
	t.Fatalf("unexpected repo anchors: %#v", anchors)
}
```

- [ ] **Step 2: Run coding MCP tests and verify they fail if payload omits anchors**

Run:

```bash
cd go/coding-exercises-mcp-service
go test ./internal/mcpserver -count=1
```

Expected: fail until store `Question` has JSON-visible `repo_anchors` and helper payload filtering preserves it.

- [ ] **Step 3: Preserve anchors when hiding expected answers**

Find the function that builds public exercise payloads in `go/coding-exercises-mcp-service/internal/mcpserver/server.go`. Ensure it copies `RepoAnchors`, `ParentQuestionID`, and `ParentPrompt` while clearing `ExpectedAnswer`.

The result should follow this shape:

```go
func publicQuestion(question store.Question) store.Question {
	question.ExpectedAnswer = ""
	return question
}
```

If the existing function manually builds a struct or map, add these fields there instead of changing the public response type.

- [ ] **Step 4: Update coding workflow text**

In `codingWorkflowInstructions`, add:

```text
Use next_exercise.repo_anchors to decide which portfolio files are relevant to inspect or cite. When reviewing an implementation, mention the anchor paths that show where the same concept appears in the repo.
```

Add:

```text
If a coding follow-up is returned, ask it only as a continuation of its parent exercise or review. Do not treat follow-ups as standalone random prompts.
```

- [ ] **Step 5: Run coding MCP tests**

Run:

```bash
cd go/coding-exercises-mcp-service
go test ./internal/mcpserver -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit Task 5**

```bash
git add go/coding-exercises-mcp-service/internal/coding/service.go go/coding-exercises-mcp-service/internal/mcpserver/server.go go/coding-exercises-mcp-service/internal/mcpserver/server_test.go
git commit -m "feat: expose coding exercise repo anchors"
```

If git cannot create `.git/index.lock`, record the exact error and continue without committing.

## Task 6: Markdown Content Migration

**Files:**
- Modify: `docs/interview-prep/qa/02-go-language-fundamentals.md`
- Modify: `docs/interview-prep/qa/03-rest-api-gateway-questions.md`
- Modify: `docs/interview-prep/qa/04-third-party-integrations.md`
- Modify: `docs/interview-prep/qa/05-distributed-systems-scalability.md`
- Modify: `docs/interview-prep/qa/06-ai-agent-systems.md`
- Modify: `docs/interview-prep/qa/07-database-observability-security.md`
- Modify: `docs/interview-prep/qa/09-go-performance-and-concurrency-drills.md`
- Modify: `docs/interview-prep/qa/01-portfolio-recall-matrix.md`
- Modify: files under `docs/interview-prep/coding-exercises/`

- [ ] **Step 1: Identify all embedded QA coding sections**

Run:

```bash
rg -n '^## Coding Exercises|^### Exercise' docs/interview-prep/qa
```

Expected: list every QA topic file still containing coding exercise material.

- [ ] **Step 2: Move coding sections into coding-exercises markdown**

For each `## Coding Exercises` block found in QA, remove the block from the QA file and add the exercises to a matching coding-exercises file. Use topic files when the moved set is large:

```text
docs/interview-prep/coding-exercises/02-go-language-fundamentals.md
docs/interview-prep/coding-exercises/03-rest-api-gateway.md
docs/interview-prep/coding-exercises/04-third-party-integrations.md
docs/interview-prep/coding-exercises/05-distributed-systems.md
docs/interview-prep/coding-exercises/06-ai-agent-systems.md
docs/interview-prep/coding-exercises/07-database-observability-security.md
docs/interview-prep/coding-exercises/09-go-performance-concurrency.md
```

Each moved exercise must use:

```markdown
### N. Exercise title

Prompt:

> Implementation prompt.

Fast design:

> Expected design, concurrency behavior, edge cases, and tests.

Repo anchors:
- `repo/path` - Why this path applies.
```

- [ ] **Step 3: Convert QA follow-up bullets to answer-key blocks**

For each QA `Follow-ups:` list, convert bullets:

```markdown
Follow-ups:

- When is sync.Map appropriate?
```

to:

```markdown
Follow-ups:

#### Follow-up: When is sync.Map appropriate?

Fast answer:

> Use `sync.Map` for read-heavy shared maps with stable keys or disjoint key
> ownership. For most service state in this repo, explicit `sync.RWMutex`
> keeps invariants clearer and easier to race-test.

Repo anchors:
- `go/...` - Shows the repo-specific synchronization choice.
```

Do this for all imported follow-ups, prioritizing tier 1 parent questions first.

- [ ] **Step 4: Add repo anchors to tier 1 and portfolio QA**

Use targeted `rg` to find concrete repo paths for each concept. Examples:

```bash
rg -n "idempot|webhook|outbox|circuit|retry|context|rate|trace|OpenTelemetry|SSE|tool" go services java k8s .github
```

Add `Repo anchors:` blocks under each question or follow-up. Keep each block to 1-3 anchors.

- [ ] **Step 5: Verify QA has no embedded coding exercises**

Run:

```bash
rg -n '^## Coding Exercises|^### Exercise' docs/interview-prep/qa
```

Expected: no output.

- [ ] **Step 6: Verify follow-ups use block form**

Run:

```bash
rg -n '^- ' docs/interview-prep/qa
```

Expected: ordinary prose bullets may exist, but no bullets directly under `Follow-ups:` should remain as imported follow-up prompts.

- [ ] **Step 7: Run parser tests against migrated content**

Run:

```bash
cd go/qa-mcp-service
go test ./internal/content -count=1
cd ../coding-exercises-mcp-service
go test ./internal/content -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit Task 6**

```bash
git add docs/interview-prep/qa docs/interview-prep/coding-exercises
git commit -m "docs: split interview QA and coding exercise banks"
```

If git cannot create `.git/index.lock`, record the exact error and continue without committing.

## Task 7: End-To-End Verification

**Files:**
- No new files expected.
- Verify all changed Go and markdown files.

- [ ] **Step 1: Run QA service tests**

```bash
cd go/qa-mcp-service
go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 2: Run coding exercises service tests**

```bash
cd go/coding-exercises-mcp-service
go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 3: Run Go preflight**

```bash
make preflight-go
```

Expected: PASS. If blocked by local toolchain or environment limits, capture the exact blocker.

- [ ] **Step 4: Smoke import QA material**

```bash
cd go/qa-mcp-service
go run ./cmd/qa-mcp
```

Expected: service starts, logs import to stderr, and exits only when interrupted. Use Ctrl-C after seeing startup/import logs.

- [ ] **Step 5: Smoke import coding exercise material**

```bash
cd go/coding-exercises-mcp-service
go run ./cmd/coding-exercises-mcp
```

Expected: service starts, logs import to stderr, and exits only when interrupted. Use Ctrl-C after seeing startup/import logs.

- [ ] **Step 6: Check final diff**

```bash
git status --short
git diff --stat
```

Expected: only planned files changed. `prompt.md` may appear as pre-existing unrelated work and should not be included unless Kyle explicitly asks.

- [ ] **Step 7: Commit verification fixes if needed**

If verification required fixes, commit the fixes:

```bash
git add go/qa-mcp-service go/coding-exercises-mcp-service docs/interview-prep
git commit -m "test: verify interview prep MCP question banks"
```

If git cannot create `.git/index.lock`, record the exact error and leave changes uncommitted.

## Self-Review Checklist

- Spec coverage: Tasks cover content separation, role-map exclusion, portfolio recall retention, repo anchors, mandatory follow-up answers, contextual tier 2 follow-ups, MCP payloads, workflow instructions, parser tests, store tests, and preflight.
- Placeholder scan: The plan uses concrete files, commands, and code snippets; no placeholder implementation steps are required.
- Type consistency: The plan uses `RepoAnchor`, `RepoAnchors`, `ParentPrompt`, `ParentQuestionID`, `NextFollowUp`, `SubmitAndNextInput`, and `SubmitAnswerAndPrepareNext` consistently across parser, store, service, and MCP layers.
