# Coding Exercises MCP Service

Local-only MCP stdio server for coding exercise practice and review.

## User Experience

The coding exercises MCP is the durable state source for repo-based coding
practice. It imports coding exercise markdown into SQLite, tracks review
attempts and feedback, and returns the next exercise from the requested tier and
category.

Agents should present coding exercises as implementation tasks, wait while the
user changes code in the repo or workspace, inspect the relevant files and
tests, then submit a `review_summary` through the MCP tools. The user should
not be asked for a prose answer in chat for base coding exercise prompts.

## Run Directly

This is mainly useful for smoke testing. In normal use, Codex or another MCP
client starts the process and communicates with it over stdio.

```bash
go run ./cmd/coding-exercises-mcp
```

The service logs startup and health information to stderr so stdout stays
reserved for MCP protocol messages.

## Register With Codex

From a regular shell, register the stdio server once:

```bash
codex mcp add coding-exercises -- zsh -lc 'cd /Users/kylebradshaw/repos/gen_ai_engineer/go/coding-exercises-mcp-service && exec go run ./cmd/coding-exercises-mcp'
```

Verify registration:

```bash
codex mcp list
codex mcp get coding-exercises
```

Environment:

- `CODING_EXERCISES_DB_PATH`: SQLite path, defaults to `data/coding-exercises.db`.
- `CODING_EXERCISES_MATERIAL_PATH`: markdown directory, defaults to `../../docs/interview-prep/coding-exercises`.

Equivalent Codex config:

```toml
[mcp_servers.coding-exercises]
command = "zsh"
args = [
  "-lc",
  "cd /Users/kylebradshaw/repos/gen_ai_engineer/go/coding-exercises-mcp-service && exec go run ./cmd/coding-exercises-mcp",
]
```

## Agent Usage

The server exposes:

- Prompt: `coding_exercises`
- Resource: `coding-exercises://workflow`
- Tools:
  - `start_coding_exercise_session`
  - `import_coding_exercise_material`
  - `list_coding_exercise_topics`
  - `get_next_coding_exercise`
  - `submit_coding_review`
  - `submit_coding_review_and_prepare_next`
  - `record_coding_review_feedback`
  - `get_coding_exercise_progress_summary`
  - `add_or_update_coding_exercise_expected_design`

Agents should start with `coding_exercises` or
`start_coding_exercise_session`. The returned workflow tells the agent to
present one coding task at a time, inspect files and tests after the user says
the implementation is ready, submit a review summary, prepare feedback for
storage on the next turn, stay within the requested tier and category, and then
present the next implementation task.
