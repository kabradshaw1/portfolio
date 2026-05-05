# Interview Prep MCP Question Bank Handoff

## Current Branch And Git State

- Work started on `main` by mistake.
- `git add` and branch creation may be blocked in this sandbox because writing
  inside `.git` failed earlier with:
  `fatal: Unable to create '.git/index.lock': Operation not permitted`.
- Do not include the existing `prompt.md` modification unless Kyle explicitly
  asks; it was present before this work.

Current intentional files from this effort:

- `docs/superpowers/specs/2026-05-04-interview-prep-mcp-question-bank-design.md`
- `docs/superpowers/plans/2026-05-04-interview-prep-mcp-question-bank.md`
- `docs/superpowers/handoffs/2026-05-04-interview-prep-mcp-question-bank.md`
- `go/qa-mcp-service/internal/content/parser.go`
- `go/qa-mcp-service/internal/content/parser_test.go`
- `go/qa-mcp-service/internal/store/sqlite.go`
- `go/qa-mcp-service/internal/store/sqlite_test.go`

## Approved Design

Spec file:

`docs/superpowers/specs/2026-05-04-interview-prep-mcp-question-bank-design.md`

Plan file:

`docs/superpowers/plans/2026-05-04-interview-prep-mcp-question-bank.md`

Goal:

- Keep QA and coding exercise question banks separate.
- Exclude `00-role-map.md` from QA MCP imports.
- Keep `01-portfolio-recall-matrix.md` as imported portfolio QA.
- Add structured `repo_anchors`.
- Require qualified follow-up expected answers.
- Make tier 2 follow-ups contextual continuations after strong parent answers,
  not random standalone prompts.

## Completed Work

### Task 1: QA Parser Contract

Changed:

- `go/qa-mcp-service/internal/content/parser.go`
- `go/qa-mcp-service/internal/content/parser_test.go`

Implemented:

- Added `content.RepoAnchor`.
- Added `Question.ParentPrompt`.
- Added `Question.RepoAnchors`.
- `ParseDir` skips `00-role-map.md` and `08-coding-exercises.md`.
- `ParseFile` skips `## Coding Exercises` sections until the next `##` heading
  or EOF.
- Parses `Repo anchors:` bullet lines in the form:
  ``- `path` - note``.
- Parses `#### Follow-up: ...` blocks as follow-up questions with parent
  prompt and `Fast answer`.
- Legacy bullet follow-ups still import and inherit cloned parent anchors.
- Follow-ups without explicit anchors inherit parent anchors.

Verification:

- Worker ran:
  `cd go/qa-mcp-service && go test ./internal/content -count=1`
- Result: PASS.
- Spec compliance review: APPROVED.
- Code quality review: APPROVED.

### Task 2: QA Store Anchors And Follow-Up Selection

Changed:

- `go/qa-mcp-service/internal/store/sqlite.go`
- `go/qa-mcp-service/internal/store/sqlite_test.go`

Implemented:

- Added store `RepoAnchor` with JSON fields.
- Added `Question.ParentQuestionID`, `Question.ParentPrompt`, and
  `Question.RepoAnchors`.
- Migration creates/adds:
  - `questions.parent_question_id`
  - `questions.parent_prompt`
  - `question_repo_anchors`
- `UpsertQuestions` persists `parent_prompt`, replaces anchors, inherits parent
  anchors for follow-ups without explicit anchors, and resolves
  `parent_question_id`.
- `NextQuestion` filters out standalone follow-ups and loads anchors.
- Added `NextFollowUp(ctx, parentQuestionID)` with unattempted-first,
  weakest-attempted, oldest-attempted, lowest-id ordering.
- Added tests for anchor persistence, parent link resolution, and excluding
  standalone follow-ups from `NextQuestion`.

Verification:

- Default `go test ./internal/store -count=1` was blocked by sandbox Go cache
  permissions under `~/Library/Caches/go-build`.
- Worker reran with writable cache:
  `GOCACHE=/private/tmp/codex-go-build-cache go test ./internal/store -count=1`
- Result: PASS.
- Spec compliance review: APPROVED.
- Code quality review was started but then intentionally stopped for this
  handoff. It still needs to be performed.

## Remaining Work

Continue from the plan at Task 2 code quality review, then proceed in order:

1. Finish Task 2 code quality review.
2. Task 3: QA service and MCP tier 2 flow.
3. Task 4: Coding exercises parser and store parity.
4. Task 5: Coding MCP payloads and workflow.
5. Task 6: Markdown content migration.
6. Task 7: End-to-end verification.

Use `GOCACHE=/private/tmp/codex-go-build-cache` for Go tests if the sandbox
cannot write the default Go build cache.

## Important Caveats

- The working tree includes an unrelated `prompt.md` modification. Preserve it.
- Git operations that write inside `.git` may fail in this environment. If so,
  ask Kyle to run branch/stage/commit commands locally or retry in a session
  with `.git` write access.
- The plan includes commit steps, but commits have not been possible from this
  sandbox.

## Suggested Next Commands

Check state:

```bash
git branch --show-current
git status --short
```

Run focused QA checks:

```bash
cd go/qa-mcp-service
GOCACHE=/private/tmp/codex-go-build-cache go test ./internal/content ./internal/store -count=1
```

If `.git` write access is available, move work off `main`:

```bash
git switch -c feature/interview-prep-mcp-question-bank
```

When committing, do not stage `prompt.md` unless explicitly requested.
