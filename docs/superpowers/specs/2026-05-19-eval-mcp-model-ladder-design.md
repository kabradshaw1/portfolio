# Eval MCP Model Ladder Design

## TL;DR

Build a focused MCP-driven eval workflow that demonstrates a professional RAG
quality story: compare local, efficient API, and premium API answer-generation
tiers under controlled conditions, keep the judge fixed, capture provenance, and
record measured trade-offs. The goal is not a full experimentation platform. The
goal is a reproducible portfolio narrative backed by eval history.

## Problem

The eval service can already run datasets, persist results, compare runs, attach
runs to experiments, capture retrieval configuration, and inspect worst cases.
The current portfolio story is weaker than the underlying system because
experiments are mostly retrieval toggles, and model changes require manual
runtime configuration outside the MCP workflow.

That creates three problems:

- The most meaningful quality variable, the answer model, is not first-class in
  MCP-run experiments.
- Manual model changes are easy to misrecord and hard to explain as
  reproducible engineering.
- Small retrieval-only changes may produce small gains, but they do not tell a
  compelling quality, cost, and latency trade-off story.

## Goals

- Let the eval MCP launch answer-model tier comparisons without manual chat
  redeploys between runs.
- Keep the judge model fixed for all candidates so scoring stays comparable.
- Persist enough model and retrieval provenance to explain every run after the
  fact.
- Produce an experiment history suitable for a portfolio walkthrough:
  baseline, candidates, metric deltas, worst cases, and a recorded conclusion.
- Keep implementation scope narrow enough to finish and verify locally.

## Non-Goals

- No public anonymous run creation.
- No full dashboard redesign.
- No automatic model selection or autonomous paid spend.
- No broad experiment-matrix platform with scheduling, budgets, and statistical
  analysis.
- No raw API keys persisted in eval run records.

## Recommended Experiment Story

Run a three-tier model ladder:

1. Local baseline: current Ollama/Qwen configuration.
2. Efficient API tier: a low-cost hosted model.
3. Premium API tier: a stronger hosted model.

Use the same dataset, collection, judge model, and retrieval corpus across all
runs. Compare two retrieval settings:

- Baseline retrieval: `top_k=5`, rerank off.
- Quality retrieval: `top_k=8`, rerank on.

If cost and runtime allow it, run two repeats per cell:

```text
3 answer tiers x 2 retrieval configs x 2 repeats = 12 eval runs
```

The portfolio conclusion should frame the result as an engineering trade-off:
which tier improves quality, where retrieval helps or hurts, and whether the
extra cost and latency are justified.

## Architecture

### Eval API

Extend `StartEvaluationRequest` with optional answer-generation override fields:

- `answer_provider`
- `answer_base_url`
- `answer_model`
- `answer_api_key_secret`
- `answer_tier`

`answer_api_key_secret` is a logical secret name, not a secret value. The eval
service resolves it from configured environment variables or a local secret map.
If no answer override is provided, the run uses the current chat service
configuration.

The eval service keeps judge configuration separate:

- Judge provider/model remain service-level eval configuration.
- Judge provider/model are captured in run config.
- Candidate answer model changes do not change the judge.

### Chat/RAG Call Path

Per-run answer overrides should affect answer generation only. Retrieval
configuration continues to flow through existing `retrieval_config` and `rerank`
fields.

The preferred implementation is to pass a narrow, explicit model override to the
chat service for internal eval calls. The chat service should accept the
override only when the request is authenticated as internal eval traffic. Normal
public chat traffic should continue using configured defaults.

### Config Capture

Each evaluation run should persist:

- Requested answer tier/provider/model.
- Effective answer provider/model.
- Judge provider/model.
- Requested and effective retrieval config.
- Requested rerank value.
- Effective collection.
- Best-effort per-run latency and token usage summaries when providers expose
  them.
- Capture errors, if any.

Run config must avoid storing raw secrets.

Latency and token usage should be treated as operational evidence, not primary
quality metrics. If a provider does not expose usage data, the run remains valid
and records the missing usage metadata explicitly.

### Eval MCP

Expose answer-model fields on `start_eval_run`. The MCP should also make the
workflow easier to use by documenting a model-ladder pattern in `eval://workflow`
and the eval prompt text.

The MCP does not need a complex sweep tool for this iteration. A disciplined
sequence of labeled `start_eval_run`, `wait_for_eval_run`, `compare_eval_runs`,
`get_worst_eval_cases`, and `record_eval_experiment_conclusion` is enough.

## Data Flow

1. User asks Codex to run a model ladder experiment through eval MCP.
2. MCP starts or resumes an eval experiment.
3. MCP starts one labeled eval run with answer-model override, retrieval config,
   rerank setting, and baseline ID when applicable.
4. Eval API validates dataset, collection, experiment attachment, retrieval
   config, and model override fields.
5. Eval service captures requested and effective config.
6. Eval service runs each golden item through retrieval and answer generation.
7. Eval service judges all answers with the fixed judge model.
8. MCP waits for completion, compares runs, inspects worst cases, and records an
   approved conclusion.

## Error Handling

- If MCP is not authenticated for eval writes, run creation should fail with a
  clear message that points to MCP auth setup rather than presenting the failure
  as a generic rate limit.
- If an answer model override is invalid or missing credentials, the eval run
  should fail early before any dataset items run.
- If config capture partially fails, the eval run may continue but records the
  capture error.
- If a candidate run fails, comparison tooling should exclude it from metric
  deltas and surface the failure reason.
- If a hosted API model is unavailable or over budget, the workflow should still
  support local-only runs.
- If usage or latency metadata is incomplete, summaries should mark those fields
  as unavailable rather than estimating them silently.

## Security

- Keep anonymous eval run creation disabled.
- Do not store raw API keys in requests, run configs, logs, or experiment
  evidence.
- Restrict per-run answer override support to internal eval-authenticated calls.
- Treat paid-provider usage as opt-in through configured secrets.

## Testing

### Python Eval Tests

- Request model validation accepts supported override fields and rejects unknown
  fields.
- Eval start persists requested answer model metadata.
- Config capture includes effective answer model and judge model.
- Eval results or run config include available latency and token usage metadata.
- Missing API secret reference fails before item execution.
- Existing retrieval-config and rerank behavior still works.

### Chat Service Tests

- Internal eval requests can override answer provider/model.
- Non-internal requests cannot override answer provider/model.
- Retrieval behavior remains controlled by `top_k` and rerank inputs.

### Go Eval MCP Tests

- `start_eval_run` schema includes answer-model fields.
- MCP forwards answer-model fields to the eval API.
- MCP validation rejects raw secret-looking values where a secret reference is
  expected.
- Workflow resource mentions the model-ladder pattern and fixed-judge rule.

### Verification

- Run `make preflight-python` for eval/chat changes.
- Run `make preflight-go` for MCP changes.
- Run one local or QA smoke experiment with at least two model tiers before
  claiming the workflow works end to end.

## Success Criteria

- A user can launch model-tier eval runs from the MCP without manually changing
  chat deployment config between runs.
- Completed run records show answer tier/model, judge model, retrieval config,
  rerank setting, aggregate scores, and available usage or latency metadata.
- The MCP workflow can produce a clear comparison and worst-case summary.
- The final portfolio narrative can truthfully say:

  > I built an MCP-driven RAG eval workflow that compares model quality tiers
  > under controlled conditions and records the quality, cost, and latency
  > trade-offs behind each decision.
