# Study MCP Service

Local-only MCP stdio server for interview practice.

Run:

```bash
go run ./cmd/study-mcp
```

Environment:

- `STUDY_DB_PATH`: SQLite path, defaults to `data/study.db`.
- `STUDY_MATERIAL_PATH`: markdown directory, defaults to `../../docs/interview-prep/micro1-go-developer`.

Example MCP server config while this branch is in its feature worktree:

```json
{
  "mcpServers": {
    "study": {
      "command": "go",
      "args": ["run", "./cmd/study-mcp"],
      "cwd": "/Users/kylebradshaw/repos/gen_ai_engineer/.codex/worktrees/study-mcp-service/go/study-service"
    }
  }
}
```

After merging into the main repo checkout, change `cwd` to:

```text
/Users/kylebradshaw/repos/gen_ai_engineer/go/study-service
```

Suggested agent instruction:

```text
Use the study MCP server to quiz me. Ask one question at a time. After I answer,
call submit_answer, compare my answer to the expected answer, point out missing
or inaccurate parts, let me ask clarifying questions, then call record_feedback
with a 0-3 score before moving on.
```
