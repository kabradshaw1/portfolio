# Internal MCP Narrative Design

## Goal

Update the frontend MCP showcase so it mentions three internal MCPs that support
the engineering workflow behind the portfolio:

- Observability MCP
- QA MCP
- Eval MCP service

The addition should read as a narrative description, not as a new public
integration guide.

## Placement

Add a new subsection to `frontend/src/components/ai/MCPSection.tsx` after the
existing public MCP tool catalog and before "Try it interactively."

This keeps the public Go MCP server story intact while making room for the
internal MCPs as operational tooling.

## Content

Use the heading `Internal MCPs for Engineering Operations`.

The subsection should explain that these MCPs are Codex-facing tools used to
operate, evaluate, and improve the project. It should include short narrative
descriptions for each MCP:

- Observability MCP: builds bounded evidence from service health, logs, metrics,
  and traces for incident triage and recovery verification.
- QA MCP: manages structured QA practice, weak-topic tracking, answer attempts,
  and feedback against expected answers.
- Eval MCP service: manages RAG evaluation datasets and runs, compares
  candidates, surfaces worst cases, and helps decide whether retrieval changes
  improved quality.

Do not add public endpoint snippets or client setup instructions for these
internal MCPs.

## UI Shape

Use plain prose plus a compact three-item list. Match the existing `/ai` page
style: restrained typography, muted explanatory text, and no new decorative card
layout.

## Tests

Update the existing mocked MCP section e2e coverage so `/ai` asserts that the
new subsection renders and includes the three MCP names.
