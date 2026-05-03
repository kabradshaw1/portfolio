# ADR: Local Study MCP Service for Interview Practice

**Date:** 2026-04-30
**Status:** Accepted

## Context

The `docs/interview-prep/micro1-go-developer` material is structured enough to
support agent-led practice: base questions, fast answers, and follow-up prompts.
The immediate need is interview preparation within roughly 24 hours, not a
deployed portfolio feature.

The repo already demonstrates MCP through `go/ai-service`, including the
official Go MCP SDK, stdio mode, streamable HTTP, and MCP client discovery. A
study workflow needs similar agent-tool interaction, but the domain is different
from ecommerce assistant behavior. It needs local persistence for answers,
feedback, and progress, while keeping runtime memory and setup cost low.

Key constraints:

- The service must be usable quickly from a local MCP-capable agent.
- It should not require Kubernetes, Postgres, Docker Compose, frontend work, or
  a local LLM.
- It should persist answer history so progress can be reviewed over repeated
  practice sessions.
- It should allow follow-up questions to exist before final expected answers are
  written.
- It should avoid bloating `go/ai-service` with interview-study concerns.

## Decision

Create a separate local-only Go MCP stdio service for study practice, backed by
SQLite. The first implementation lives under `go/study-service` and is designed
to be launched by an MCP client as a subprocess.

The service exposes study-specific MCP tools:

- `start_study_session`
- `import_material`
- `list_topics`
- `get_next_question`
- `submit_answer`
- `record_feedback`
- `get_progress_summary`
- `add_or_update_expected_answer`

The service also exposes:

- MCP prompt `study_micro1`, a reusable prompt for starting a Micro1 session.
- MCP resource `study://micro1/workflow`, a text workflow describing how an
  agent should quiz, submit answers, record feedback, and continue.

The service should be self-describing enough that a user can tell an
MCP-capable agent "study for micro1" without pasting tool-by-tool instructions.
The agent still owns qualitative conversational behavior and answer critique,
but the service advertises the expected workflow through MCP prompt/resource
metadata and `start_study_session`. That tool returns the study set name,
workflow instructions, topics, progress summary, and next question. The service
owns deterministic state: imported questions, expected answers, raw attempts,
scores, feedback, and progress summaries.

The service should use the official `github.com/modelcontextprotocol/go-sdk`
for consistency with `go/ai-service`, but it should not be embedded in the
`go/ai-service` binary. It should start with stdio only. Streamable HTTP,
Postgres, frontend dashboards, multi-user auth, and Kubernetes manifests are
deferred until there is a clear need.

## Alternatives Rejected

### Add the study tools directly to `go/ai-service`

This would reuse existing MCP plumbing, but it mixes ecommerce assistant
behavior with personal interview-study state. It also increases the service's
configuration and testing surface for a workflow that is currently local-only.

### Build a REST API and frontend first

A browser dashboard would be easier to demonstrate visually, but it slows down
the main goal: getting into a useful quiz loop immediately. REST and frontend
work can be added later if the study workflow proves valuable.

### Use Postgres from the start

Postgres would match the production stack, but it adds local setup and memory
cost. SQLite is enough for single-user local state, requires no daemon, and
keeps the service usable from an MCP client with one command.

### Use a CLI without MCP

A CLI would be the fastest standalone tool, but it would not exercise MCP and
would make the agent integration less realistic. MCP is the better fit because
the agent needs durable, structured tools while the LLM handles the
conversation.

### Deploy the service immediately

Deployment would add Kubernetes manifests, secrets, health checks, and
operational concerns before the workflow has been validated. The first version
should stay local and lightweight.

## Consequences

Positive outcomes:

- Local study can start quickly with a small Go process and a SQLite file.
- MCP practice reinforces the same protocol story already present in
  `go/ai-service`.
- Agents can discover the Micro1 study workflow from MCP metadata instead of
  requiring repeated manual prompt instructions.
- Interview answers, critique, and scores become durable instead of living only
  in chat history.
- Follow-up questions can be imported without complete expected answers and
  filled in over time.
- The implementation remains a clean bounded context instead of expanding
  ecommerce AI service scope.

Trade-offs:

- The service is not a polished standalone product.
- MCP client configuration is required before an agent can use it.
- Client support for MCP prompts and resources varies, so `start_study_session`
  also carries workflow instructions as tool output.
- SQLite is appropriate for local single-user practice but not for future
  multi-user deployment.
- The agent, not the service, performs qualitative answer grading in the first
  version.
- Repo-level Go preflight may need an explicit update later if this service
  should become part of the standard Go sweep.

## Follow-Up Work

- Add more complete expected answers for follow-up questions.
- Add a small command or MCP resource for exporting weak-topic summaries.
- Decide whether `go/study-service` should be included in `make preflight-go`.
- Consider a lightweight dashboard only after the local MCP workflow proves
  useful for repeated sessions.
