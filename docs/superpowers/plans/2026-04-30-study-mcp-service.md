# Study MCP Service Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a local Go MCP stdio server that imports interview-prep markdown, quizzes through agent tools, and stores answer progress in SQLite.

**Architecture:** Add a standalone `go/study-service` module with `content`, `store`, `study`, and `mcpserver` packages. The server is local-only, stdio-only, and uses SQLite with a gitignored database file.

**Tech Stack:** Go 1.26.1, `github.com/modelcontextprotocol/go-sdk`, `modernc.org/sqlite`, standard `database/sql`.

---

## File Structure

- Create `go/study-service/go.mod`: standalone module and dependencies.
- Create `go/study-service/.gitignore`: ignore local DB files.
- Create `go/study-service/cmd/study-mcp/main.go`: config, DB open, import, MCP stdio startup.
- Create `go/study-service/internal/content/parser.go`: parse markdown questions, fast answers, and follow-ups.
- Create `go/study-service/internal/content/parser_test.go`: parser fixtures.
- Create `go/study-service/internal/store/sqlite.go`: schema, import, attempts, feedback, progress queries.
- Create `go/study-service/internal/store/sqlite_test.go`: in-memory SQLite coverage.
- Create `go/study-service/internal/study/service.go`: orchestration and next-question selection.
- Create `go/study-service/internal/study/service_test.go`: selection and answer flow tests.
- Create `go/study-service/internal/mcpserver/server.go`: MCP tool registration and JSON responses.
- Create `go/study-service/internal/mcpserver/server_test.go`: handler-level validation using fake study service.
- Create `go/study-service/README.md`: local run and MCP client config notes.

### Task 1: Module Skeleton

**Files:**
- Create: `go/study-service/go.mod`
- Create: `go/study-service/.gitignore`
- Create: `go/study-service/README.md`

- [ ] **Step 1: Create module files**

`go.mod`:

```go
module github.com/kabradshaw1/portfolio/go/study-service

go 1.26.1

require (
	github.com/modelcontextprotocol/go-sdk v1.5.0
	modernc.org/sqlite v1.39.1
)
```

`.gitignore`:

```gitignore
data/
*.db
*.db-shm
*.db-wal
```

`README.md`:

```markdown
# Study MCP Service

Local-only MCP stdio server for interview practice.

Run:

```bash
go run ./cmd/study-mcp
```

Environment:

- `STUDY_DB_PATH`: SQLite path, defaults to `data/study.db`.
- `STUDY_MATERIAL_PATH`: markdown directory, defaults to `../../docs/interview-prep/micro1-go-developer`.
```

- [ ] **Step 2: Download dependencies**

Run: `cd go/study-service && go mod tidy`

Expected: `go.sum` is created and dependencies resolve.

- [ ] **Step 3: Commit skeleton**

```bash
git add go/study-service
git commit -m "feat: add study service module"
```

### Task 2: Markdown Parser

**Files:**
- Create: `go/study-service/internal/content/parser.go`
- Create: `go/study-service/internal/content/parser_test.go`

- [ ] **Step 1: Write parser tests**

Cover a heading with `Fast answer:` and `Follow-ups:`. Assert one base question
with expected answer and two follow-up questions with empty expected answers.

- [ ] **Step 2: Implement parser**

Define:

```go
type Question struct {
	SourcePath     string
	Topic          string
	Prompt         string
	ExpectedAnswer string
	IsFollowUp      bool
	Priority        int
}

func ParseFile(path string, data []byte) ([]Question, error)
func ParseDir(root string) ([]Question, error)
```

Use line-oriented parsing:

- `# ` sets the topic.
- `### ` starts a base question.
- `Fast answer:` starts expected-answer capture.
- `Follow-ups:` starts follow-up bullet capture.
- `- ` lines inside follow-ups become follow-up questions.

- [ ] **Step 3: Verify parser**

Run: `cd go/study-service && go test ./internal/content -v`

Expected: PASS.

- [ ] **Step 4: Commit parser**

```bash
git add go/study-service/internal/content
git commit -m "feat: parse study markdown"
```

### Task 3: SQLite Store

**Files:**
- Create: `go/study-service/internal/store/sqlite.go`
- Create: `go/study-service/internal/store/sqlite_test.go`

- [ ] **Step 1: Write store tests**

Cover:

- `Migrate` creates tables.
- `UpsertQuestions` inserts base and follow-up questions idempotently.
- `CreateSession`, `SubmitAnswer`, and `RecordFeedback` persist records.
- `ProgressSummary` returns recent attempts and weak topics.

- [ ] **Step 2: Implement schema and methods**

Define:

```go
type DB struct { db *sql.DB }
func Open(path string) (*DB, error)
func OpenSQL(db *sql.DB) *DB
func (d *DB) Migrate(ctx context.Context) error
func (d *DB) UpsertQuestions(ctx context.Context, questions []content.Question) error
func (d *DB) CreateSession(ctx context.Context) (int64, error)
func (d *DB) ListTopics(ctx context.Context) ([]Topic, error)
func (d *DB) NextQuestion(ctx context.Context) (Question, error)
func (d *DB) SubmitAnswer(ctx context.Context, in SubmitAnswerInput) (AnswerAttempt, error)
func (d *DB) RecordFeedback(ctx context.Context, in FeedbackInput) error
func (d *DB) UpdateExpectedAnswer(ctx context.Context, id int64, answer string) error
func (d *DB) ProgressSummary(ctx context.Context) (ProgressSummary, error)
```

Use a unique key on `(source_path, prompt, is_follow_up)` so imports can be
rerun safely.

- [ ] **Step 3: Verify store**

Run: `cd go/study-service && go test ./internal/store -v`

Expected: PASS.

- [ ] **Step 4: Commit store**

```bash
git add go/study-service/internal/store
git commit -m "feat: store study progress in sqlite"
```

### Task 4: Study Service

**Files:**
- Create: `go/study-service/internal/study/service.go`
- Create: `go/study-service/internal/study/service_test.go`

- [ ] **Step 1: Write service tests**

Cover:

- Import calls parser and store.
- Next question returns question, expected-answer presence, and topic.
- Submit answer creates a session lazily when no session exists.
- Feedback score validation rejects values outside `0..3`.

- [ ] **Step 2: Implement service**

Define:

```go
type Service struct {
	store Store
	materialPath string
	sessionID int64
}

func New(store Store, materialPath string) *Service
func (s *Service) ImportMaterial(ctx context.Context) (ImportResult, error)
func (s *Service) ListTopics(ctx context.Context) ([]Topic, error)
func (s *Service) GetNextQuestion(ctx context.Context) (Question, error)
func (s *Service) SubmitAnswer(ctx context.Context, questionID int64, answer string) (AnswerReview, error)
func (s *Service) RecordFeedback(ctx context.Context, input FeedbackInput) error
func (s *Service) ProgressSummary(ctx context.Context) (ProgressSummary, error)
func (s *Service) UpdateExpectedAnswer(ctx context.Context, questionID int64, answer string) error
```

- [ ] **Step 3: Verify service**

Run: `cd go/study-service && go test ./internal/study -v`

Expected: PASS.

- [ ] **Step 4: Commit service**

```bash
git add go/study-service/internal/study
git commit -m "feat: orchestrate study sessions"
```

### Task 5: MCP Server And Command

**Files:**
- Create: `go/study-service/internal/mcpserver/server.go`
- Create: `go/study-service/internal/mcpserver/server_test.go`
- Create: `go/study-service/cmd/study-mcp/main.go`

- [ ] **Step 1: Write MCP handler tests**

Use a fake study service and assert tool handlers decode arguments, validate
required fields, call the correct fake method, and return JSON text content.

- [ ] **Step 2: Implement MCP server**

Register tools:

- `import_material`
- `list_topics`
- `get_next_question`
- `submit_answer`
- `record_feedback`
- `get_progress_summary`
- `add_or_update_expected_answer`

Return MCP errors as `CallToolResult{IsError: true}` with a text message. Return
successful results as a single JSON `TextContent`.

- [ ] **Step 3: Implement command**

`main.go` should:

- Resolve `STUDY_DB_PATH`, defaulting to `data/study.db`.
- Resolve `STUDY_MATERIAL_PATH`, defaulting to `../../docs/interview-prep/micro1-go-developer`.
- Create the DB directory.
- Open and migrate SQLite.
- Import material once at startup.
- Run the MCP server over `sdkmcp.StdioTransport`.

- [ ] **Step 4: Verify command package**

Run: `cd go/study-service && go test ./...`

Expected: PASS.

- [ ] **Step 5: Commit MCP server**

```bash
git add go/study-service
git commit -m "feat: expose study tools over mcp"
```

### Task 6: Final Verification

**Files:**
- Modify: `go/study-service/README.md`

- [ ] **Step 1: Update README with MCP config example**

Add:

```json
{
  "mcpServers": {
    "study": {
      "command": "go",
      "args": ["run", "./cmd/study-mcp"],
      "cwd": "/Users/kylebradshaw/repos/gen_ai_engineer/go/study-service"
    }
  }
}
```

- [ ] **Step 2: Run targeted verification**

Run: `cd go/study-service && go test ./...`

Expected: PASS.

- [ ] **Step 3: Run repo Go preflight if feasible**

Run: `make preflight-go`

Expected: PASS, or report the local blocker clearly.

- [ ] **Step 4: Commit final docs if changed**

```bash
git add go/study-service/README.md
git commit -m "docs: document local study mcp usage"
```

## Self-Review Notes

- Spec coverage: all requested local-only MCP, SQLite, markdown import, answer
  storage, feedback, progress, and follow-up-answer iteration requirements are
  represented.
- Scope control: deployment, frontend, Kubernetes, Postgres, and in-service LLM
  grading are intentionally deferred.
- Risk: MCP client configuration may differ by agent client. The README should
  include the command shape, while exact client-specific wiring can be adjusted
  after the binary runs.
