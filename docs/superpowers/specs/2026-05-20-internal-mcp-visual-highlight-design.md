# Internal MCP Visual Highlight Design

## Goal

Make the internal MCP narrative on `/ai` more visually memorable so visitors
spend time understanding how the Observability MCP, QA MCP, and Eval MCP service
support engineering work.

The section should remain visually consistent with the existing shopping
assistant presentation: agent/tool-call language, compact operational panels,
Mermaid-style flow, and concrete examples.

## Placement

Update the existing `Internal MCPs for Engineering Operations` subsection in
`frontend/src/components/ai/MCPSection.tsx`.

Keep it after the public MCP tool catalog and before `Try it interactively`.
Add an anchorable `id` to the heading so the section can be linked directly.

## Visual Structure

Use a combined visual treatment:

1. A compact flow diagram showing:
   - Codex
   - Observability MCP, QA MCP, and Eval MCP service
   - Evidence-backed engineering action

2. A shopping-assistant-style tool-call transcript showing a concrete
   observability example:
   - User asks why checkout stalled in QA
   - Codex calls observability investigation tools
   - The result becomes a concise engineering explanation

3. A short `Why this matters` panel explaining that the MCPs:
   - turn production questions into bounded evidence requests
   - keep practice feedback and weak-topic tracking in the workflow
   - make RAG changes measurable before treating them as improvements

## Style

Follow the existing `/ai` page and shopping assistant style:

- restrained typography
- rounded borders no larger than existing local patterns
- muted foreground text for explanatory copy
- compact panels rather than large marketing cards
- no unrelated decorative illustrations
- responsive layout that stacks cleanly on mobile

Use existing project primitives where practical. A Mermaid diagram is appropriate
for the flow portion because the surrounding AI page already uses Mermaid for
architecture and request-flow explanations.

## Tests

Extend mocked `/ai` e2e coverage to assert:

- the internal MCP section remains visible
- the direct anchor target exists
- the transcript example includes at least one observability tool call
- the `Why this matters` panel renders

Keep existing tests for the three MCP names.
