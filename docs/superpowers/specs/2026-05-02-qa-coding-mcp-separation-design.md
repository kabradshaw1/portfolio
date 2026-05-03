# QA And Coding MCP Separation - Design Spec

**Date:** 2026-05-02
**Status:** Design - ready for implementation plan

---

## Context

`go/study-service` currently serves two different practice modes through one
local MCP stdio service:

- Q&A interview study, where the user answers in chat and the agent gives
  teaching feedback.
- Coding exercises, where the user should implement code in the repo or
  workspace and the agent should review the implementation.

The current service already imports coding prompts as `kind=coding_exercise`,
but Q&A and coding still share the same MCP prompt, workflow resource, session
startup, answer submission, and progress model. That shared interface makes the
coding mode easy for agents to treat like another prose answer in chat.

The target design is two independent MCP services with separate question banks:

- `qa`
- `coding-exercises`

---

## Decision

Create two completely separate local MCP stdio services:

- `go/qa-mcp-service`
- `go/coding-exercises-mcp-service`

Each service owns its MCP prompt, workflow resource, tools, default material
path, default SQLite database, README, tests, and user-facing behavior. The
existing `go/study-service` is migration source material during implementation.
After equivalent behavior is covered in the new services, it should be removed
or clearly deprecated so agents do not keep registering the old mixed workflow.

Do not extract a shared library in the first implementation. Copy the narrow
parser/store/session code that each service needs, then let the two services
diverge naturally. Extract shared code later only if duplication becomes stable
and truly mechanical.

---

## Question Banks

Use separate material directories:

- `docs/interview-prep/qa/`
- `docs/interview-prep/coding-exercises/`

The QA bank contains interview questions, expected answers, follow-ups, tiers,
and categories for chat-based study. It must not import timed coding exercises.

The coding bank contains implementation prompts, repo anchors, expected design
notes, follow-ups, and review guidance. It must not import general Q&A prompts.

The initial migration can move the current Micro1 Q&A markdown into
`docs/interview-prep/qa/` and move the current timed coding exercise markdown
into `docs/interview-prep/coding-exercises/`. Parser behavior can remain
markdown-based, but each service should have service-specific parser tests that
prove it excludes the other bank.

---

## QA MCP Service

`go/qa-mcp-service` preserves the current Q&A behavior.

The service is a durable state source for chat-based interview practice:

1. Start a QA study session.
2. Return the next QA question from the requested tier/category.
3. Ask the user to answer in chat.
4. Store the user's answer.
5. Return the expected answer snapshot.
6. Let the agent explain misses, give a polished interview answer, and prepare
   feedback.
7. Record prior feedback on the next answer submission.
8. Return the next QA question in the same tool response so the agent can keep
   the session moving.

The workflow instructions should keep the teaching style that is already
working:

- `Score: X/3`
- `Explanation:`
- `Interview answer:`
- optional `Minimum answer:`
- optional `Memory hook:`

The QA workflow must not mention implementation review, workspace file
inspection, or coding exercise submission.

### QA Tools

Use QA-specific tool names so agents cannot confuse the service with coding
practice:

- `start_qa_session`
- `import_qa_material`
- `list_qa_topics`
- `get_next_qa_question`
- `submit_qa_answer`
- `submit_qa_answer_and_prepare_next`
- `record_qa_feedback`
- `get_qa_progress_summary`
- `add_or_update_qa_expected_answer`

The schemas can mirror the current `study-service` schemas, with names and
descriptions rewritten for QA.

### QA Storage

Default SQLite path:

- `data/qa.db`

The schema can start from the current `questions`, `sessions`,
`answer_attempts`, and `feedback` tables. Go type names should be QA-specific
where that improves readability, but the stored data model can remain simple:
questions, attempts, expected answer snapshots, and feedback scores.

---

## Coding Exercises MCP Service

`go/coding-exercises-mcp-service` is not a prose quiz service. Its workflow
should force a review loop instead of a chat-answer loop.

The service is a durable state source for implementation practice:

1. Start a coding exercise session.
2. Return one coding exercise from the requested tier/category.
3. Present the prompt as an implementation task.
4. Tell the user to implement in the repo/workspace and respond when ready for
   review.
5. When the user says the implementation is ready, the agent inspects relevant
   files and tests.
6. The agent submits a review summary as the attempt result.
7. The service returns the expected design/review notes and the next exercise.
8. The agent grades the implementation and records prior feedback on the next
   review submission.

The coding workflow should grade against:

- Correctness against the prompt
- Idiomatic Go
- Concurrency safety
- Edge cases
- Test quality
- Simplicity
- Prompt-specific expected design notes

The coding workflow must not ask the user to answer the exercise in chat. It
should tell the agent to inspect files, run relevant tests when appropriate,
mention files reviewed, and treat the submitted attempt as a code review
summary.

### Coding Tools

Use coding-specific tool names:

- `start_coding_exercise_session`
- `import_coding_exercise_material`
- `list_coding_exercise_topics`
- `get_next_coding_exercise`
- `submit_coding_review`
- `submit_coding_review_and_prepare_next`
- `record_coding_review_feedback`
- `get_coding_exercise_progress_summary`
- `add_or_update_coding_exercise_expected_design`

The service should avoid generic `answer` naming in user-facing descriptions.
The payload can still store a text summary internally, but schemas and docs
should call it `review_summary` where practical.

### Coding Storage

Default SQLite path:

- `data/coding-exercises.db`

The initial schema can resemble the QA schema, but Go types and MCP payloads
should use coding vocabulary:

- exercise
- review summary
- expected design
- review feedback

No cross-service progress aggregation is required for this spec.

---

## Migration Strategy

Implementation should preserve working behavior while splitting the boundary:

1. Create `go/qa-mcp-service` from the current `go/study-service` Q&A behavior.
2. Create `go/coding-exercises-mcp-service` from the current coding exercise
   parser data and a new coding-specific MCP workflow.
3. Split the markdown banks into `docs/interview-prep/qa/` and
   `docs/interview-prep/coding-exercises/`.
4. Update READMEs with separate `codex mcp add` commands using distinct MCP
   server names.
5. Add tests that prove each service excludes the other bank and exposes only
   its own workflow language.
6. Once both services pass preflight, remove or deprecate `go/study-service` so
   the mixed service is no longer the default registration target.

---

## Error Handling

Both services should keep the current MCP error style: validation failures are
returned as MCP tool errors rather than transport errors.

Required validation:

- Supported study set/service name only.
- Tier is empty, 1, 2, or 3.
- Category is normalized before filtering.
- Question/exercise IDs are required for submissions.
- QA answers must be non-empty.
- Coding review summaries must be non-empty.
- Feedback scores must be between 0 and 3.

If no matching question or exercise exists for a filter, the tool should return
a clear MCP error. The implementation plan can decide whether to reuse the
current `store.ErrNotFound` behavior or add a friendlier message at the MCP
handler layer.

---

## Testing

Unit tests should prove the split at the service boundary.

QA service tests:

- Parser/import excludes coding exercises.
- `start_qa_session` returns QA workflow instructions and a QA question.
- QA workflow instructions include the teaching sections.
- QA workflow instructions do not mention implementation review or file
  inspection.
- `submit_qa_answer_and_prepare_next` records prior feedback and preserves
  tier/category filters.

Coding service tests:

- Parser/import excludes QA material.
- `start_coding_exercise_session` returns coding workflow instructions and a
  coding exercise.
- Coding workflow instructions tell the agent to wait for implementation and
  inspect files/tests.
- Coding workflow instructions do not ask for a prose answer in chat.
- `submit_coding_review_and_prepare_next` records prior review feedback and
  preserves tier/category filters.

Migration tests:

- Existing parser behavior for current Micro1 Q&A prompts is represented in
  `qa-mcp-service` tests.
- Existing parser behavior for timed coding exercises is represented in
  `coding-exercises-mcp-service` tests.

Before committing implementation changes, run:

```bash
make preflight-go
```

For a spec-only change, no Go preflight is required.

---

## Out Of Scope

- Web UI for study sessions.
- Cross-service progress aggregation.
- Shared package extraction.
- Kubernetes deployment for these local-only MCP services.
- Automated code grading beyond agent-led file/test inspection.
- Changing the Q&A teaching style beyond removing coding-service language.

---

## Success Criteria

- Codex can register and run a `qa` MCP server that never serves coding
  exercises.
- Codex can register and run a `coding-exercises` MCP server that never serves
  Q&A prompts.
- QA sessions keep the current answer-in-chat teaching loop.
- Coding sessions present implementation tasks and wait for repo/workspace
  review.
- Each service has an independent material bank and SQLite DB.
- Tests prevent the two workflows from collapsing back into one generic study
  workflow.
