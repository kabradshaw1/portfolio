# AI Internal MCP Hub Design

## Summary

Rework the `/ai` page so it can keep growing as an AI systems hub without
turning into a long undifferentiated document. The page should use compact
anchor navigation and bounded feature panels that preview each subsystem, prove
there is real engineering behind it, and link to deeper routes for the full
story.

The immediate focus is the internal MCP section. Observability MCP and Eval MCP
should be highly featured because they are command surfaces over large
ecosystems. QA MCP remains visible as a supporting internal tool.

## Goals

- Make `/ai` easier to scan as more AI systems are added.
- Show that Observability MCP is a read-only operational investigation surface
  over a mature observability platform.
- Show that Eval MCP is the operator control plane for RAG evaluation work.
- Avoid duplicating the full content already available on `/observability` and
  `/ai/eval`.
- Give recruiters clear paths to the deeper pages when they want more detail.

## Non-Goals

- Do not turn `/ai` into full documentation for observability or evaluation.
- Do not hide major content behind tabs.
- Do not add a new route for internal MCPs in this iteration.
- Do not change MCP service behavior or public API behavior.

## Page Structure

Add a compact anchor navigation block near the top of the `/ai` content. The
navigation should link to the major sections on the page:

- MCP Server
- Internal MCPs
- RAG Evaluation
- Debugging Assistant
- Connect a Client

The navigation should be responsive. Desktop may use a compact sticky or
near-top layout. Mobile should render as a wrapped inline anchor list rather
than a sidebar. The purpose is orientation and quick jumping, not a tabbed
interface.

## Internal MCP Showcase

Replace the current internal MCP block with a bounded showcase using the
pattern "preview, proof, pathway":

- Preview: what the MCP does.
- Proof: tool names, connected systems, metrics, or workflow details.
- Pathway: a link to the deeper route when one exists.

### Observability MCP

Observability MCP should be a large featured panel. It should communicate that
the MCP is a read-only investigation interface over the running platform, not a
surface-level list of tools.

Content to foreground:

- Fronts Prometheus, Loki, Jaeger, Grafana gateway mode, and embedded runbooks.
- Produces bounded evidence bundles for system health and incident triage.
- Supports targeted workflows for checkout, AI pipeline, eval-run, streaming
  analytics, service-level evidence, log search, and trace lookup.
- Preserves operational safety by staying read-only.

Proof points to show in the panel:

- `Prometheus`
- `Loki`
- `Jaeger`
- `Grafana`
- `5 dashboards`
- `16 alert rules`

Representative tool chips:

- `get_system_health`
- `investigate_checkout`
- `investigate_ai_pipeline`
- `investigate_eval_run`
- `investigate_streaming_analytics`
- `get_service_evidence`
- `search_logs`
- `get_trace`

Pathway link:

- Link to `/observability` with copy similar to "See the full observability
  platform."

### Eval MCP

Eval MCP should be a second large featured panel. It should communicate that the
service is the operator control plane for repeatable RAG evaluation
experiments, not just a generic dataset API.

Content to foreground:

- Coordinates Eval API datasets, evaluation runs, experiments, and conclusions.
- Reads RAG collection information from the ingestion service.
- Captures retrieval configuration such as `top_k`.
- Supports rerank comparisons and baseline/candidate analysis.
- Surfaces worst cases so weak queries can be inspected before a change is
  treated as an improvement.
- Pairs with Observability MCP through eval-run-specific runtime evidence.

Proof points to show in the panel:

- `Eval API`
- `RAG collections`
- `dataset fixtures`
- `evaluation runs`
- `experiments`
- `rerank`
- `top_k`

Representative tool chips:

- `start_eval_run`
- `wait_for_eval_run`
- `compare_eval_runs`
- `get_worst_eval_cases`
- `get_rag_collection_config`
- `record_eval_experiment_conclusion`
- `summarize_eval_experiment`

Pathway link:

- Link to `/ai/eval` with copy similar to "Open the RAG evaluation workflow."

### QA MCP

QA MCP should remain visible but compact. It is useful context, but it should
not compete visually with Observability MCP and Eval MCP.

Content to foreground:

- Structured practice sessions.
- Expected-answer feedback.
- Weak-topic tracking.
- Review attempts and scoring.

## Growth Pattern

Future AI tools and services added to `/ai` should follow the same "preview,
proof, pathway" pattern. This keeps the page useful as a hub while preventing
the page from becoming a duplicate of deeper feature pages.

When a subsystem has a deeper route, `/ai` should summarize the agent/MCP layer
and link to that route for implementation details, dashboards, workflows, and
longer narrative.

## Testing

Update mocked `/ai` e2e coverage to verify:

- The new section navigation labels render.
- `#internal-mcps` still exists and is visible.
- The Observability MCP panel names Prometheus, Loki, Jaeger, and Grafana.
- The Observability MCP panel links to `/observability`.
- The Eval MCP panel names Eval API and RAG collections.
- The Eval MCP panel links to `/ai/eval`.
- Representative tool names for Observability MCP and Eval MCP are visible.

Targeted verification should include:

- `npx playwright test e2e/mocked/ai-mcp-section.spec.ts`
- `make preflight-frontend`
- `make preflight-e2e`
