# QA And Coding MCP Separation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the mixed `go/study-service` MCP server with two separate local MCP services: `qa` and `coding-exercises`.

**Architecture:** Build `go/qa-mcp-service` and `go/coding-exercises-mcp-service` as independent Go modules and stdio MCP servers. Start by copying the proven `go/study-service` implementation, then narrow each copy so QA only imports chat-answer questions and coding-exercises only imports implementation prompts with review-oriented tools and workflow text.

**Tech Stack:** Go, `github.com/modelcontextprotocol/go-sdk/mcp`, SQLite via `modernc.org/sqlite`, markdown material under `docs/interview-prep/`, repo preflight via `make preflight-go`.

---

## File Structure

Create or modify these paths:

- Create: `docs/interview-prep/qa/`
  - Owns chat-based Q&A markdown material.
- Create: `docs/interview-prep/coding-exercises/`
  - Owns implementation exercise markdown material.
- Create: `go/qa-mcp-service/`
  - Independent MCP stdio service for Q&A study.
- Create: `go/qa-mcp-service/cmd/qa-mcp/main.go`
  - QA service entrypoint.
- Create: `go/qa-mcp-service/internal/content/parser.go`
  - QA markdown parser; excludes coding material.
- Create: `go/qa-mcp-service/internal/store/sqlite.go`
  - QA SQLite persistence.
- Create: `go/qa-mcp-service/internal/qa/service.go`
  - QA business service.
- Create: `go/qa-mcp-service/internal/mcpserver/server.go`
  - QA MCP prompt, resource, tools, schemas, and workflow.
- Create: `go/coding-exercises-mcp-service/`
  - Independent MCP stdio service for coding exercises.
- Create: `go/coding-exercises-mcp-service/cmd/coding-exercises-mcp/main.go`
  - Coding exercises service entrypoint.
- Create: `go/coding-exercises-mcp-service/internal/content/parser.go`
  - Coding exercise markdown parser; excludes QA material.
- Create: `go/coding-exercises-mcp-service/internal/store/sqlite.go`
  - Coding exercise SQLite persistence.
- Create: `go/coding-exercises-mcp-service/internal/coding/service.go`
  - Coding exercise business service.
- Create: `go/coding-exercises-mcp-service/internal/mcpserver/server.go`
  - Coding exercise MCP prompt, resource, tools, schemas, and workflow.
- Modify: `go/study-service/README.md`
  - Mark the mixed service as deprecated after both new services pass tests, or remove the directory if all registration docs are replaced.

Each new service should have tests matching the existing `go/study-service` test layout:

- `cmd/*/main_test.go`
- `internal/content/parser_test.go`
- `internal/store/sqlite_test.go`
- `internal/qa/service_test.go` or `internal/coding/service_test.go`
- `internal/mcpserver/server_test.go`

---

## Task 1: Split The Material Banks

**Files:**
- Create: `docs/interview-prep/qa/`
- Create: `docs/interview-prep/coding-exercises/`
- Move into QA: existing Micro1 markdown except `08-coding-exercises.md`
- Move into coding exercises: `docs/interview-prep/micro1-go-developer/08-coding-exercises.md`

- [ ] **Step 1: Create destination directories**

Run:

```bash
mkdir -p docs/interview-prep/qa docs/interview-prep/coding-exercises
```

Expected: command exits 0.

- [ ] **Step 2: Move QA markdown with git**

Run:

```bash
for file in docs/interview-prep/micro1-go-developer/*.md; do
  base="$(basename "$file")"
  if [ "$base" != "08-coding-exercises.md" ]; then
    git mv "$file" "docs/interview-prep/qa/$base"
  fi
done
```

Expected: all non-coding Micro1 markdown files are staged as renames into `docs/interview-prep/qa/`.

- [ ] **Step 3: Move coding markdown with git**

Run:

```bash
git mv docs/interview-prep/micro1-go-developer/08-coding-exercises.md docs/interview-prep/coding-exercises/08-coding-exercises.md
```

Expected: `08-coding-exercises.md` is staged as a rename into `docs/interview-prep/coding-exercises/`.

- [ ] **Step 4: Verify bank contents**

Run:

```bash
rg -n "Timed Coding Exercises|Exercise Set" docs/interview-prep/qa docs/interview-prep/coding-exercises
```

Expected: matches only under `docs/interview-prep/coding-exercises/`.

Run:

```bash
rg -n "Tell me about|How do you|What is" docs/interview-prep/coding-exercises
```

Expected: matches only coding follow-ups or coding prompt text from `08-coding-exercises.md`, not the general QA bank.

- [ ] **Step 5: Commit material split**

Run:

```bash
git add docs/interview-prep/qa docs/interview-prep/coding-exercises docs/interview-prep/micro1-go-developer
git commit -m "Split QA and coding exercise material banks"
```

Expected: commit succeeds.

---

## Task 2: Create The QA MCP Service Skeleton

**Files:**
- Create: `go/qa-mcp-service/**`
- Source: `go/study-service/**`

- [ ] **Step 1: Copy the existing service into the QA service**

Run:

```bash
cp -R go/study-service go/qa-mcp-service
rm -rf go/qa-mcp-service/data
mv go/qa-mcp-service/cmd/study-mcp go/qa-mcp-service/cmd/qa-mcp
```

Expected: `go/qa-mcp-service` exists with the same source layout as `go/study-service`, but the command directory is `cmd/qa-mcp`.

- [ ] **Step 2: Rename module imports**

Run:

```bash
perl -pi -e 's#github.com/kabradshaw1/portfolio/go/study-service#github.com/kabradshaw1/portfolio/go/qa-mcp-service#g' $(rg -l "github.com/kabradshaw1/portfolio/go/study-service" go/qa-mcp-service)
perl -pi -e 's#module github.com/kabradshaw1/portfolio/go/study-service#module github.com/kabradshaw1/portfolio/go/qa-mcp-service#' go/qa-mcp-service/go.mod
```

Expected: `rg "study-service" go/qa-mcp-service/go.mod go/qa-mcp-service/internal go/qa-mcp-service/cmd` returns no module import references.

- [ ] **Step 3: Rename package directory from study to qa**

Run:

```bash
mv go/qa-mcp-service/internal/study go/qa-mcp-service/internal/qa
perl -pi -e 's#/internal/study#/internal/qa#g; s/\bstudy\./qa./g; s/package study/package qa/g' $(rg -l "internal/study|package study|study\\." go/qa-mcp-service)
```

Expected: `rg "internal/study|package study" go/qa-mcp-service` returns no matches.

- [ ] **Step 4: Update command package tests and imports**

Run:

```bash
perl -pi -e 's/study-mcp/qa-mcp/g; s/Study MCP/QA MCP/g; s/study service/QA service/g' $(rg -l "study-mcp|Study MCP|study service" go/qa-mcp-service)
```

Expected: command references use `qa-mcp`.

- [ ] **Step 5: Run QA service tests to expose remaining mixed naming**

Run:

```bash
cd go/qa-mcp-service && go test ./...
```

Expected: tests may fail because MCP tool names and workflow text still use mixed study names. Keep the output for Task 3.

---

## Task 3: Narrow QA Behavior And Tests

**Files:**
- Modify: `go/qa-mcp-service/internal/content/parser.go`
- Modify: `go/qa-mcp-service/internal/content/parser_test.go`
- Modify: `go/qa-mcp-service/internal/mcpserver/server.go`
- Modify: `go/qa-mcp-service/internal/mcpserver/server_test.go`
- Modify: `go/qa-mcp-service/internal/qa/service.go`
- Modify: `go/qa-mcp-service/README.md`

- [ ] **Step 1: Add a failing parser test that rejects coding material**

Edit `go/qa-mcp-service/internal/content/parser_test.go` and add:

```go
func TestParseDirExcludesCodingExerciseMaterial(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "01-qa.md"), []byte(`# QA

### 1. How do maps work under concurrency?

Fast answer:

> Use synchronization.
`), 0o600); err != nil {
		t.Fatalf("write qa fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "08-coding-exercises.md"), []byte(`# Timed Coding Exercises

### 1. Concurrent map with race prevention

Prompt:

> Build a thread-safe counter map.

Fast design:

> Use sync.RWMutex.
`), 0o600); err != nil {
		t.Fatalf("write coding fixture: %v", err)
	}

	questions, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("ParseDir returned error: %v", err)
	}
	if len(questions) != 1 {
		t.Fatalf("expected only QA question, got %d: %#v", len(questions), questions)
	}
	if questions[0].Category == "coding" || questions[0].Kind == "coding_exercise" {
		t.Fatalf("QA parser imported coding material: %#v", questions[0])
	}
}
```

- [ ] **Step 2: Run the failing parser test**

Run:

```bash
cd go/qa-mcp-service && go test ./internal/content -run TestParseDirExcludesCodingExerciseMaterial -count=1
```

Expected: FAIL because the copied parser still imports `08-coding-exercises.md`.

- [ ] **Step 3: Implement QA parser exclusion**

Edit `go/qa-mcp-service/internal/content/parser.go` in `ParseDir` so the file loop skips coding material:

```go
if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" || entry.Name() == "08-coding-exercises.md" {
	continue
}
```

Also change `kindForQuestion` so QA never emits coding kind:

```go
func kindForQuestion(path string, followUp bool) string {
	return "qa"
}
```

- [ ] **Step 4: Run QA parser tests**

Run:

```bash
cd go/qa-mcp-service && go test ./internal/content -count=1
```

Expected: PASS after removing or rewriting copied tests that expected coding exercises in the QA parser. Keep tests that prove base questions, follow-ups, tiers, categories, and sorted import still work.

- [ ] **Step 5: Add failing MCP workflow tests for QA-only language**

Edit `go/qa-mcp-service/internal/mcpserver/server_test.go` so the session/resource tests assert:

```go
if !strings.Contains(payload.Instructions, "submit_qa_answer_and_prepare_next") {
	t.Fatalf("expected QA workflow to mention submit_qa_answer_and_prepare_next, got %q", payload.Instructions)
}
if strings.Contains(payload.Instructions, "coding_exercise") ||
	strings.Contains(strings.ToLower(payload.Instructions), "implementation review") ||
	strings.Contains(strings.ToLower(payload.Instructions), "inspect files") {
	t.Fatalf("QA workflow should not contain coding review language: %q", payload.Instructions)
}
```

Update prompt tests to expect `start_qa_session`.

- [ ] **Step 6: Run failing MCP tests**

Run:

```bash
cd go/qa-mcp-service && go test ./internal/mcpserver -count=1
```

Expected: FAIL because copied tool names and workflow still use mixed study wording.

- [ ] **Step 7: Rename QA MCP tools and workflow**

Edit `go/qa-mcp-service/internal/mcpserver/server.go`:

- Server implementation name: `qa-mcp-service`
- Prompt name: `qa`
- Resource URI: `qa://workflow`
- Tool names:
  - `start_qa_session`
  - `import_qa_material`
  - `list_qa_topics`
  - `get_next_qa_question`
  - `submit_qa_answer`
  - `submit_qa_answer_and_prepare_next`
  - `record_qa_feedback`
  - `get_qa_progress_summary`
  - `add_or_update_qa_expected_answer`

Replace workflow text with QA-only instructions that include:

```text
After the user answers a QA question, call exactly one MCP tool:
submit_qa_answer_and_prepare_next with question_id, answer, tier, category,
and any previous_feedback payload you prepared for the prior answer.
```

Keep these teaching sections:

```text
Score: X/3
Explanation:
Interview answer:
Minimum answer, only when useful:
Memory hook, only when useful:
```

- [ ] **Step 8: Update QA defaults**

Edit `go/qa-mcp-service/cmd/qa-mcp/main.go` so defaults are:

```go
const (
	defaultDBPath       = "data/qa.db"
	defaultMaterialPath = "../../docs/interview-prep/qa"
)
```

If the copied file uses inline literals instead of constants, replace the fallback values directly.

- [ ] **Step 9: Update QA README**

Edit `go/qa-mcp-service/README.md` so registration uses:

```bash
codex mcp add qa -- zsh -lc 'cd /Users/kylebradshaw/repos/gen_ai_engineer/go/qa-mcp-service && exec go run ./cmd/qa-mcp'
```

Document:

```text
QA_DB_PATH defaults to data/qa.db.
QA_MATERIAL_PATH defaults to ../../docs/interview-prep/qa.
```

- [ ] **Step 10: Run QA service tests**

Run:

```bash
cd go/qa-mcp-service && go test ./...
```

Expected: PASS.

- [ ] **Step 11: Commit QA service**

Run:

```bash
git add go/qa-mcp-service
git commit -m "Add QA MCP service"
```

Expected: commit succeeds.

---

## Task 4: Create The Coding Exercises MCP Service Skeleton

**Files:**
- Create: `go/coding-exercises-mcp-service/**`
- Source: `go/study-service/**`

- [ ] **Step 1: Copy the existing service into the coding service**

Run:

```bash
cp -R go/study-service go/coding-exercises-mcp-service
rm -rf go/coding-exercises-mcp-service/data
mv go/coding-exercises-mcp-service/cmd/study-mcp go/coding-exercises-mcp-service/cmd/coding-exercises-mcp
```

Expected: `go/coding-exercises-mcp-service` exists with command directory `cmd/coding-exercises-mcp`.

- [ ] **Step 2: Rename module imports**

Run:

```bash
perl -pi -e 's#github.com/kabradshaw1/portfolio/go/study-service#github.com/kabradshaw1/portfolio/go/coding-exercises-mcp-service#g' $(rg -l "github.com/kabradshaw1/portfolio/go/study-service" go/coding-exercises-mcp-service)
perl -pi -e 's#module github.com/kabradshaw1/portfolio/go/study-service#module github.com/kabradshaw1/portfolio/go/coding-exercises-mcp-service#' go/coding-exercises-mcp-service/go.mod
```

Expected: copied imports point at `go/coding-exercises-mcp-service`.

- [ ] **Step 3: Rename package directory from study to coding**

Run:

```bash
mv go/coding-exercises-mcp-service/internal/study go/coding-exercises-mcp-service/internal/coding
perl -pi -e 's#/internal/study#/internal/coding#g; s/\bstudy\./coding./g; s/package study/package coding/g' $(rg -l "internal/study|package study|study\\." go/coding-exercises-mcp-service)
```

Expected: `rg "internal/study|package study" go/coding-exercises-mcp-service` returns no matches.

- [ ] **Step 4: Run copied tests to expose mixed behavior**

Run:

```bash
cd go/coding-exercises-mcp-service && go test ./...
```

Expected: tests may fail until Task 5 narrows parser, vocabulary, defaults, and tool names.

---

## Task 5: Narrow Coding Exercise Behavior And Tests

**Files:**
- Modify: `go/coding-exercises-mcp-service/internal/content/parser.go`
- Modify: `go/coding-exercises-mcp-service/internal/content/parser_test.go`
- Modify: `go/coding-exercises-mcp-service/internal/mcpserver/server.go`
- Modify: `go/coding-exercises-mcp-service/internal/mcpserver/server_test.go`
- Modify: `go/coding-exercises-mcp-service/internal/coding/service.go`
- Modify: `go/coding-exercises-mcp-service/README.md`

- [ ] **Step 1: Add a failing parser test that imports only coding exercises**

Replace general QA parser tests in `go/coding-exercises-mcp-service/internal/content/parser_test.go` with:

```go
func TestParseDirIncludesOnlyCodingExerciseMaterial(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "02-go-language-fundamentals.md"), []byte(`# Go

### 1. How do maps work under concurrency?

Fast answer:

> Use synchronization.
`), 0o600); err != nil {
		t.Fatalf("write qa fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "08-coding-exercises.md"), []byte(`# Timed Coding Exercises

### 1. Concurrent map with race prevention

Prompt:

> Build a thread-safe counter map.

Fast design:

> Use sync.RWMutex.
`), 0o600); err != nil {
		t.Fatalf("write coding fixture: %v", err)
	}

	exercises, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("ParseDir returned error: %v", err)
	}
	if len(exercises) != 1 {
		t.Fatalf("expected one coding exercise, got %d: %#v", len(exercises), exercises)
	}
	if exercises[0].Kind != "coding_exercise" || exercises[0].Category != "coding" {
		t.Fatalf("expected coding exercise, got %#v", exercises[0])
	}
}
```

- [ ] **Step 2: Run the failing coding parser test**

Run:

```bash
cd go/coding-exercises-mcp-service && go test ./internal/content -run TestParseDirIncludesOnlyCodingExerciseMaterial -count=1
```

Expected: FAIL because the copied parser imports both QA and coding files.

- [ ] **Step 3: Implement coding parser exclusion**

Edit `go/coding-exercises-mcp-service/internal/content/parser.go` in `ParseDir` so the file loop only accepts coding material:

```go
if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" || entry.Name() != "08-coding-exercises.md" {
	continue
}
```

Keep `kindForQuestion` returning `coding_exercise` for base exercise prompts and `qa` for follow-ups:

```go
func kindForQuestion(path string, followUp bool) string {
	if followUp {
		return "qa"
	}
	return "coding_exercise"
}
```

- [ ] **Step 4: Run coding parser tests**

Run:

```bash
cd go/coding-exercises-mcp-service && go test ./internal/content -count=1
```

Expected: PASS.

- [ ] **Step 5: Add failing MCP workflow tests for review-only language**

Edit `go/coding-exercises-mcp-service/internal/mcpserver/server_test.go` so session/resource tests assert:

```go
if !strings.Contains(payload.Instructions, "submit_coding_review_and_prepare_next") {
	t.Fatalf("expected coding workflow to mention submit_coding_review_and_prepare_next, got %q", payload.Instructions)
}
if !strings.Contains(strings.ToLower(payload.Instructions), "inspect") ||
	!strings.Contains(strings.ToLower(payload.Instructions), "files") ||
	!strings.Contains(strings.ToLower(payload.Instructions), "tests") {
	t.Fatalf("coding workflow should require file and test inspection: %q", payload.Instructions)
}
if strings.Contains(strings.ToLower(payload.Instructions), "prose answer in chat") {
	t.Fatalf("coding workflow should not ask for a prose answer: %q", payload.Instructions)
}
```

Update prompt tests to expect `start_coding_exercise_session`.

- [ ] **Step 6: Run failing coding MCP tests**

Run:

```bash
cd go/coding-exercises-mcp-service && go test ./internal/mcpserver -count=1
```

Expected: FAIL because copied workflow and tools still use study/answer names.

- [ ] **Step 7: Rename coding MCP tools and workflow**

Edit `go/coding-exercises-mcp-service/internal/mcpserver/server.go`:

- Server implementation name: `coding-exercises-mcp-service`
- Prompt name: `coding_exercises`
- Resource URI: `coding-exercises://workflow`
- Tool names:
  - `start_coding_exercise_session`
  - `import_coding_exercise_material`
  - `list_coding_exercise_topics`
  - `get_next_coding_exercise`
  - `submit_coding_review`
  - `submit_coding_review_and_prepare_next`
  - `record_coding_review_feedback`
  - `get_coding_exercise_progress_summary`
  - `add_or_update_coding_exercise_expected_design`

Use `review_summary` in MCP schemas and handler input structs where the copied service used `answer`:

```go
type input struct {
	ExerciseID    int64  `json:"exercise_id"`
	ReviewSummary string `json:"review_summary"`
}
```

Map `review_summary` to the existing store text field internally if the table remains `answer_attempts`.

- [ ] **Step 8: Replace coding workflow text**

In `codingWorkflowInstructions()`, include this exact behavior:

```text
If next_exercise.kind is "coding_exercise", present next_exercise.prompt as an implementation task. Tell the user to implement it in the repo or workspace and respond when ready for review. Do not ask for a prose answer in chat.

When the user is ready, inspect the relevant files and tests before calling submit_coding_review_and_prepare_next. The review_summary must mention the files reviewed, tests run, correctness against the prompt, concurrency safety, idiomatic Go, edge cases, test quality, and simplicity.
```

- [ ] **Step 9: Update coding defaults**

Edit `go/coding-exercises-mcp-service/cmd/coding-exercises-mcp/main.go` so defaults are:

```go
const (
	defaultDBPath       = "data/coding-exercises.db"
	defaultMaterialPath = "../../docs/interview-prep/coding-exercises"
)
```

If the copied file uses inline literals, replace those fallback values directly.

- [ ] **Step 10: Update coding README**

Edit `go/coding-exercises-mcp-service/README.md` so registration uses:

```bash
codex mcp add coding-exercises -- zsh -lc 'cd /Users/kylebradshaw/repos/gen_ai_engineer/go/coding-exercises-mcp-service && exec go run ./cmd/coding-exercises-mcp'
```

Document:

```text
CODING_EXERCISES_DB_PATH defaults to data/coding-exercises.db.
CODING_EXERCISES_MATERIAL_PATH defaults to ../../docs/interview-prep/coding-exercises.
```

- [ ] **Step 11: Run coding service tests**

Run:

```bash
cd go/coding-exercises-mcp-service && go test ./...
```

Expected: PASS.

- [ ] **Step 12: Commit coding service**

Run:

```bash
git add go/coding-exercises-mcp-service
git commit -m "Add coding exercises MCP service"
```

Expected: commit succeeds.

---

## Task 6: Deprecate The Mixed Study Service

**Files:**
- Modify: `go/study-service/README.md`
- Optional delete after local confirmation: `go/study-service/**`

- [ ] **Step 1: Mark mixed service deprecated**

Edit the top of `go/study-service/README.md`:

```markdown
# Study MCP Service

> Deprecated: this mixed QA/coding service has been replaced by
> `go/qa-mcp-service` and `go/coding-exercises-mcp-service`. Register `qa`
> for chat-based interview Q&A and `coding-exercises` for implementation
> review practice.
```

- [ ] **Step 2: Search for old registration guidance**

Run:

```bash
rg -n "study-mcp|mcp add study|go/study-service|start_study_session|submit_answer_and_prepare_next" README.md go docs -g '!docs/superpowers/**'
```

Expected: remaining matches are either in deprecated `go/study-service` text or in source/tests that will be removed in Step 3.

- [ ] **Step 3: Remove mixed service if no registration path needs it**

Run:

```bash
git rm -r go/study-service
```

Expected: the old mixed service is staged for removal. If a local MCP registration still points at it, update the user-facing registration docs first, then remove it.

- [ ] **Step 4: Run targeted search after removal**

Run:

```bash
rg -n "study-mcp|mcp add study|go/study-service|start_study_session|submit_answer_and_prepare_next" README.md go docs -g '!docs/superpowers/**'
```

Expected: no active registration instructions point at the old mixed service.

- [ ] **Step 5: Commit deprecation/removal**

Run:

```bash
git add go/study-service README.md go docs
git commit -m "Remove mixed study MCP service"
```

Expected: commit succeeds. If only README deprecation is chosen instead of removal, use commit message `Deprecate mixed study MCP service`.

---

## Task 7: Full Verification

**Files:**
- All new and modified Go service files
- Moved markdown material

- [ ] **Step 1: Run QA tests**

Run:

```bash
cd go/qa-mcp-service && go test ./...
```

Expected: PASS.

- [ ] **Step 2: Run coding tests**

Run:

```bash
cd go/coding-exercises-mcp-service && go test ./...
```

Expected: PASS.

- [ ] **Step 3: Run Go preflight**

Run:

```bash
make preflight-go
```

Expected: PASS. If the preflight script does not discover the new modules, add them to the repo's Go preflight service list and rerun `make preflight-go`.

- [ ] **Step 4: Smoke test QA MCP startup**

Run:

```bash
cd go/qa-mcp-service && timeout 3s go run ./cmd/qa-mcp
```

Expected: process starts and waits for MCP stdio input. `timeout` exits non-zero after 3 seconds; that timeout is acceptable if stderr shows normal startup and no panic.

- [ ] **Step 5: Smoke test coding MCP startup**

Run:

```bash
cd go/coding-exercises-mcp-service && timeout 3s go run ./cmd/coding-exercises-mcp
```

Expected: process starts and waits for MCP stdio input. `timeout` exits non-zero after 3 seconds; that timeout is acceptable if stderr shows normal startup and no panic.

- [ ] **Step 6: Final status check**

Run:

```bash
git status --short
```

Expected: clean working tree after all commits.

---

## Self-Review Notes

- Spec coverage: material banks, two services, QA behavior, coding behavior, storage defaults, error handling, tests, and old-service cleanup are mapped to tasks above.
- Red-flag scan: the plan contains no unresolved filler language.
- Type consistency: QA uses `answer`; coding uses `review_summary` at the MCP boundary and can map to existing text persistence internally.
