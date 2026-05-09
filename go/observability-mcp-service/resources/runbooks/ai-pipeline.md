# AI Pipeline Investigation

Use `investigate_ai_pipeline` when agent, RAG, embedding, Qdrant, chat, or
Ollama behavior looks unhealthy.

Signals checked:

- Agent turns by outcome and duration.
- Tool call rates and p95 latency.
- Cache hit and miss metrics when present.
- RAG stage latency and RAG errors.
- Ollama latency and token throughput when exported.
- Recent AI, chat, ingestion, and debug logs.

Interpretation:

- RAG stage latency isolates whether retrieval, embedding, or generation is the
  dominant delay.
- Tool call errors often explain agent turns that end without useful output.
- Empty metrics should be treated as missing instrumentation until confirmed.

Useful next tools:

- `get_service_evidence` for `go-ai-service`, `chat`, `ingestion`, or `debug`.
- `search_logs` with a focused pattern from a failing request.
