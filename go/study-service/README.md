# Study MCP Service

Local-only MCP stdio server for interview practice.

## User Experience

The study MCP is the durable state source for Micro1 interview practice. It
imports the markdown study guide into SQLite, tracks attempts and feedback, and
returns the next question from the requested tier and category. In a Codex
session, ask for a study mode in plain language; the agent should call
`start_study_session`, quiz one question at a time, grade your answer, store
feedback on the next turn, and keep the session inside the requested filters.

Normal Q&A questions are answered in chat. Coding exercises are different: the
agent should present the exercise, wait while you implement code in the repo or
workspace, inspect the relevant files and tests when you say you are ready, then
submit a review summary as the answer and grade the implementation.

Examples:

```text
Study micro1 tier 1.
Study micro1 tier 3 golang.
Study micro1 golang.
Study micro1 tier 2 api.
Study micro1 tier 1 coding.
Study micro1 distributed.
Study micro1 performance_concurrency.
```

Categories:

| Say this | Category filter | Study-guide section |
| --- | --- | --- |
| `portfolio` | `portfolio` | Portfolio Recall Matrix |
| `golang` | `golang` | Go Language Fundamentals |
| `api` | `api` | REST API And API Gateway |
| `integrations` | `integrations` | Third-Party API Integration |
| `distributed` | `distributed` | Distributed Systems And Scalability |
| `ai` | `ai` | AI Agent Systems |
| `db_observability_security` | `db_observability_security` | Database, Observability, And Security |
| `coding` | `coding` | Timed Coding Exercises |
| `performance_concurrency` | `performance_concurrency` | Go Performance And Concurrency Drills |
| `mock_interview` | `mock_interview` | Mock Interview Drills |

## Run Directly

This is mainly useful for smoke testing. In normal use, Codex or another MCP
client starts the process and communicates with it over stdio.

```bash
go run ./cmd/study-mcp
```

The service logs startup and health information to stderr so stdout stays
reserved for MCP protocol messages.

## Register With Codex

From a regular shell, register the stdio server once:

```bash
codex mcp add study -- zsh -lc 'cd /Users/kylebradshaw/repos/gen_ai_engineer/go/study-service && exec go run ./cmd/study-mcp'
```

Verify registration:

```bash
codex mcp list
codex mcp get study
```

Environment:

- `STUDY_DB_PATH`: SQLite path, defaults to `data/study.db`.
- `STUDY_MATERIAL_PATH`: markdown directory, defaults to `../../docs/interview-prep/micro1-go-developer`.

Equivalent Codex config:

```toml
[mcp_servers.study]
command = "zsh"
args = [
  "-lc",
  "cd /Users/kylebradshaw/repos/gen_ai_engineer/go/study-service && exec go run ./cmd/study-mcp",
]
```

## Agent Usage

The server is self-describing enough that the user should only need to say:

```text
Study tier 1 for micro1.
```

Tier 1 is the focused interview path: likely role questions plus coding
exercise prompts. Tier 2 contains likely follow-ups and secondary drills. Tier 3
contains deep-drill material. Questions are not removed; the tier only controls
which pool the next-question selector uses. Category filters narrow the pool to
one study-guide section, such as `golang` for Go fundamentals.

The MCP server exposes:

- Prompt: `study_micro1`
- Resource: `study://micro1/workflow`
- Tools:
  - `start_study_session`
  - `import_material`
  - `list_topics`
  - `get_next_question`
  - `submit_answer`
  - `submit_answer_and_prepare_next`
  - `record_feedback`
  - `get_progress_summary`
  - `add_or_update_expected_answer`

Agents should start with `study_micro1` or `start_study_session`. The returned
workflow tells the agent to ask one question at a time, wait for the user's
answer, call `submit_answer_and_prepare_next`, compare against the expected
answer, prepare feedback for storage on the next turn, stay within the requested
tier and category, and ask the next question in the same response. When the next
question has `kind=coding_exercise`, the agent should switch to repo-inspection
review instead of asking for an immediate prose answer.
