# QA MCP Service

Local-only MCP stdio server for chat-based interview Q&A practice.

## User Experience

The QA MCP service is the durable state source for interview question practice.
It imports QA markdown into SQLite, tracks attempts and feedback, and returns
the next question from the requested tier and category. In a Codex session, ask
for QA practice in plain language; the agent should call `start_qa_session`,
quiz one question at a time, grade your answer, store feedback on the next
turn, and keep the session inside the requested filters.

Examples:

```text
QA tier 1.
QA tier 3 golang.
QA golang.
QA tier 2 api.
QA distributed.
QA performance_concurrency.
```

Categories:

| Say this | Category filter | QA section |
| --- | --- | --- |
| `portfolio` | `portfolio` | Portfolio Recall Matrix |
| `golang` | `golang` | Go Language Fundamentals |
| `api` | `api` | REST API And API Gateway |
| `integrations` | `integrations` | Third-Party API Integration |
| `distributed` | `distributed` | Distributed Systems And Scalability |
| `ai` | `ai` | AI Agent Systems |
| `db_observability_security` | `db_observability_security` | Database, Observability, And Security |
| `performance_concurrency` | `performance_concurrency` | Go Performance And Concurrency Drills |
| `mock_interview` | `mock_interview` | Mock Interview Drills |

## Run Directly

This is mainly useful for smoke testing. In normal use, Codex or another MCP
client starts the process and communicates with it over stdio.

```bash
go run ./cmd/qa-mcp
```

The service logs startup and health information to stderr so stdout stays
reserved for MCP protocol messages.

## Register With Codex

From a regular shell, register the stdio server once:

```bash
codex mcp add qa -- zsh -lc 'cd /Users/kylebradshaw/repos/gen_ai_engineer/go/qa-mcp-service && exec go run ./cmd/qa-mcp'
```

Verify registration:

```bash
codex mcp list
codex mcp get qa
```

Environment:

- `QA_DB_PATH`: SQLite path, defaults to `data/qa.db`.
- `QA_MATERIAL_PATH`: markdown directory, defaults to `../../docs/interview-prep/qa`.

Equivalent Codex config:

```toml
[mcp_servers.qa]
command = "zsh"
args = [
  "-lc",
  "cd /Users/kylebradshaw/repos/gen_ai_engineer/go/qa-mcp-service && exec go run ./cmd/qa-mcp",
]
```

## Agent Usage

The server is self-describing enough that the user should only need to say:

```text
QA tier 1.
```

Tier 1 is the focused interview path. Tier 2 contains likely follow-ups and
secondary drills. Tier 3 contains deep-drill material. Questions are not
removed; the tier only controls which pool the next-question selector uses.
Category filters narrow the pool to one QA section, such as `golang` for Go
fundamentals.

The MCP server exposes:

- Prompt: `qa`
- Resource: `qa://workflow`
- Tools:
  - `start_qa_session`
  - `import_qa_material`
  - `list_qa_topics`
  - `get_next_qa_question`
  - `submit_qa_answer`
  - `submit_qa_answer_and_prepare_next`
  - `record_qa_feedback`
  - `get_qa_progress_summary`
  - `add_or_update_qa_expected_answer`

Agents should start with `qa` or `start_qa_session`. The returned workflow
tells the agent to ask one question at a time, wait for the user's answer, call
`submit_qa_answer_and_prepare_next`, compare against the expected answer,
prepare feedback for storage on the next turn, stay within the requested tier
and category, and ask the next question in the same response.
