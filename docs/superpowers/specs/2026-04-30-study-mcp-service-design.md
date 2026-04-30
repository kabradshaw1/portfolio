# Local Study MCP Service Design

## Goal

Build a local-only Go MCP stdio server that lets an agent quiz Kyle on the
Micro1 Go Developer interview material, persist answers in SQLite, and surface
progress quickly enough to support interview prep within 24 hours.

## Scope

The first version is intentionally local and agent-driven. It will not deploy to
Kubernetes, expose a public HTTP API, or appear in the frontend. It should run as
a lightweight subprocess launched by an MCP-capable client and store state in a
local SQLite database ignored by git.

## Architecture

Create a new Go module under `go/study-service`. The service exposes an MCP
stdio server using the official Go MCP SDK, similar in spirit to the existing
`go/ai-service` MCP adapter but without ecommerce dependencies.

The service has four internal boundaries:

- `content`: parses the interview-prep markdown into question records.
- `store`: owns SQLite schema, imports, answers, feedback, and progress queries.
- `study`: chooses the next question and shapes tool responses.
- `mcp`: registers MCP tools and adapts tool calls to study operations.

The agent owns the conversational loop and qualitative critique. The service
owns durable state, source material, expected answers, follow-up metadata, and
progress history.

## Data Flow

1. Kyle starts an MCP-capable agent configured to launch the local study server.
2. The agent calls `import_material` once, or the server imports on demand.
3. The agent calls `get_next_question`.
4. The agent asks Kyle the returned question.
5. Kyle answers in chat.
6. The agent calls `submit_answer` with the raw answer.
7. The service stores the attempt and returns the expected answer and rubric.
8. The agent critiques the answer and pauses for clarifying questions.
9. The agent calls `record_feedback` with score, missing points, and suggested
   stronger wording.
10. The agent calls `get_next_question` again when Kyle is ready.

## MCP Tools

- `import_material`: import or refresh questions from
  `docs/interview-prep/micro1-go-developer`.
- `list_topics`: return topics and counts.
- `get_next_question`: return an unseen or weak question, preferring priority
  material and low scores.
- `submit_answer`: store a raw answer attempt and return expected answer data.
- `record_feedback`: store agent critique, score, missing points, and suggested
  answer.
- `get_progress_summary`: return recent attempts, weak topics, and score trends.
- `add_or_update_expected_answer`: let the agent or Kyle save answers for
  follow-up questions that currently have no answer.

## SQLite Storage

Use SQLite for local speed and low memory overhead. The database path defaults
to `go/study-service/data/study.db`, with `STUDY_DB_PATH` as an override. The
database file is ignored by git.

Core tables:

- `sources`: imported markdown files and content hashes.
- `questions`: topic, source path, prompt, expected answer, follow-up flag,
  priority, and active status.
- `sessions`: study session start and optional end timestamp.
- `answer_attempts`: question id, session id, raw answer, expected answer
  snapshot, and timestamp.
- `feedback`: attempt id, score from 0 to 3, missing points, inaccurate points,
  suggested answer, and timestamp.

Scores mean:

- `0`: missed or materially incorrect.
- `1`: partially correct.
- `2`: mostly correct but not yet interview-ready.
- `3`: concise, accurate, and interview-ready.

## Markdown Import

The importer should support the existing material format:

- Question headings such as `### 1. How do maps work under concurrency?`
- `Fast answer:` blocks as expected answers.
- `Follow-ups:` bullet lists as follow-up questions.

Follow-up questions may lack expected answers. The service should import them
with a null expected answer and let `add_or_update_expected_answer` fill them in
later. This keeps the first version useful immediately.

## Local Operation

The initial run path is:

```bash
cd go/study-service
go run ./cmd/study-mcp
```

An MCP client can launch the same command as a stdio server. The service should
not start an LLM, web server, Docker container, or Kubernetes resource. Expected
memory use is small, dominated by the Go process and SQLite.

## Testing

Testing should focus on deterministic behavior:

- Markdown parser imports base questions and follow-ups from representative
  fixtures.
- SQLite store creates schema, upserts questions, records attempts, and records
  feedback.
- Study selector prioritizes unseen questions before weak repeated questions.
- MCP handlers validate inputs and return structured results.

Before committing implementation, run `make preflight-go` if feasible. For fast
iteration during development, run targeted `go test ./...` inside
`go/study-service`.

## Deferred

- Browser dashboard.
- Kubernetes manifests.
- Postgres migration.
- Multi-user auth.
- Automated LLM grading inside the service.
- Frontend portfolio integration.
