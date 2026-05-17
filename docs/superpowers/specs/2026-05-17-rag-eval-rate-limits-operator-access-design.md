# RAG Eval Rate Limits And Operator Access Design

## Purpose

RAG eval workflows need generous access for Kyle/operator experiment runs without
removing guardrails for normal users, anonymous clients, or accidental traffic
bursts. The immediate symptom was eval MCP polling receiving `429 Rate limit
exceeded: 30 per 1 minute`, but the larger risk is that concurrent eval, chat,
rerank, and judge traffic can overload the shared Ollama instance.

This design keeps rate limiting enabled, makes limits configurable without code
changes, distinguishes operator traffic from ordinary traffic, and adds runtime
admission control around expensive Ollama work.

## Current Enforcement Points

- Eval API uses SlowAPI in `services/eval/app/main.py`, keyed by remote IP.
  `POST /evaluations` is limited to `5/minute`; dataset and experiment writes
  are mostly `10/minute`; read and polling endpoints such as
  `GET /evaluations/{eval_id}` are `30/minute`.
- Chat API uses SlowAPI in `services/chat/app/main.py`, keyed by remote IP.
  `POST /chat` is `20/minute`; `POST /search` is `30/minute`.
- Eval MCP polling is in `go/eval-mcp-service/internal/evalworkflow/service.go`.
  `wait_for_eval_run` polls `GET /evaluations/{eval_id}` every
  `EVAL_MCP_POLL_INTERVAL`, defaulting to `1s`. The eval API client does not
  treat `429` specially and does not parse `Retry-After`.
- Python chat/eval paths call Ollama through `services/shared/llm/ollama.py`
  without local concurrency admission. Go `ai-service` has its own request
  rate limiter and chat admission limiter, but that does not protect Python
  RAG eval/chat traffic.
- FastAPI auth currently returns only `user_id`. Auth tokens include `sub` and
  `email`, but no role/operator claim.

## Traffic To Protect

The system should protect both API request surfaces and downstream expensive
work:

- Eval run creation, because one request can fan out into many search, chat,
  and judge calls.
- Eval polling and dashboard/history reads, because MCP clients can poll often.
- Chat `/chat`, because it performs embedding, retrieval, optional rerank, and
  LLM generation.
- Chat `/search`, because it performs embedding, retrieval, and optional rerank.
- Reranking, because it is CPU-heavy and eval workflows can repeat it per query.
- Ollama embedding, generation, and judge calls, because Ollama is the shared
  bottleneck used by local/operator experiments and interactive chat.

## Identity Model

Introduce a shared FastAPI auth context instead of returning only a string user
id. The context should contain:

- `subject`: JWT `sub`, or `anonymous` when auth is disabled.
- `email`: JWT `email` when present.
- `tier`: one of `operator`, `user`, `anonymous`, or `internal_eval`.

Tier resolution should be configuration-driven:

- `operator` when `subject` is listed in `RAG_OPERATOR_USER_IDS` or `email` is
  listed in `RAG_OPERATOR_EMAILS`.
- `user` for authenticated requests that are not operator requests.
- `anonymous` only when auth is disabled for local smoke tests or explicitly
  allowed local clients.
- `internal_eval` for eval worker calls from eval API to chat, identified by an
  internal shared token/header rather than by client IP.

This avoids a database migration or auth-service role system in the first pass.
If auth later emits role claims, the same resolver can prefer `roles` claims
before falling back to allowlists.

## Quotas

All quota values must be environment-configurable and safe by default.

Recommended defaults:

| Traffic group | Operator | User | Anonymous | Internal eval |
| --- | ---: | ---: | ---: | ---: |
| Eval run creation | 20/min | 5/min | 0/min | n/a |
| Eval reads and polling | 240/min | 30/min | 10/min | n/a |
| Eval dataset/experiment writes | 30/min | 10/min | 0/min | n/a |
| Chat ask | 60/min | 20/min | 0/min | 60/min |
| Search | 120/min | 30/min | 0/min | 120/min |

These defaults intentionally allow operator MCP polling at short intervals while
keeping normal-user and anonymous traffic near the current limits.

## Ollama Admission

Ollama protection should combine request-rate limits with concurrency admission:

- API request-rate limits remain at eval/chat boundaries for abuse and runaway
  client protection.
- Ollama generation has a separate async concurrency limiter, default `2`.
- Ollama embeddings have a separate async concurrency limiter, default `4`.
- Eval judge calls use the generation limiter.
- Optional rerank admission can start with default `2` if CPU contention is
  observed during rerank evals.
- Admission waits up to `OLLAMA_ADMISSION_QUEUE_TIMEOUT`, default `5s`. If no
  permit is available, the API returns overload with `Retry-After`.

Use `429` for quota exhaustion and `503` for resource admission overload. Both
responses should include `Retry-After` when the caller can retry safely.

## MCP 429 Handling

The eval MCP client should make polling adaptive:

- Parse `Retry-After` from eval API responses.
- On `429`, sleep for `Retry-After` when present.
- Without `Retry-After`, use exponential backoff with jitter, capped by
  `EVAL_MCP_MAX_BACKOFF`, default `30s`.
- Preserve `EVAL_MCP_POLL_INTERVAL`, but make the default safer for remote eval
  APIs, preferably `2s`.
- Surface actionable guidance only after timeout or repeated throttling, naming
  the current poll interval and suggested env var.

The MCP client should not silently spin through rate-limit responses, and it
should not require users to disable server-side rate limiting to complete local
operator evals.

## Configuration Knobs

Add settings for:

- `RAG_OPERATOR_EMAILS`
- `RAG_OPERATOR_USER_IDS`
- `RAG_INTERNAL_EVAL_TOKEN`
- `EVAL_RATE_LIMIT_RUN_CREATE_OPERATOR`
- `EVAL_RATE_LIMIT_RUN_CREATE_USER`
- `EVAL_RATE_LIMIT_RUN_CREATE_ANONYMOUS`
- `EVAL_RATE_LIMIT_READ_OPERATOR`
- `EVAL_RATE_LIMIT_READ_USER`
- `EVAL_RATE_LIMIT_READ_ANONYMOUS`
- `EVAL_RATE_LIMIT_WRITE_OPERATOR`
- `EVAL_RATE_LIMIT_WRITE_USER`
- `EVAL_RATE_LIMIT_WRITE_ANONYMOUS`
- `CHAT_RATE_LIMIT_ASK_OPERATOR`
- `CHAT_RATE_LIMIT_ASK_USER`
- `CHAT_RATE_LIMIT_ASK_ANONYMOUS`
- `CHAT_RATE_LIMIT_ASK_INTERNAL_EVAL`
- `CHAT_RATE_LIMIT_SEARCH_OPERATOR`
- `CHAT_RATE_LIMIT_SEARCH_USER`
- `CHAT_RATE_LIMIT_SEARCH_ANONYMOUS`
- `CHAT_RATE_LIMIT_SEARCH_INTERNAL_EVAL`
- `OLLAMA_GENERATE_MAX_IN_FLIGHT`
- `OLLAMA_EMBED_MAX_IN_FLIGHT`
- `RERANK_MAX_IN_FLIGHT`
- `OLLAMA_ADMISSION_QUEUE_TIMEOUT`
- `OLLAMA_OVERLOAD_RETRY_AFTER`
- `EVAL_MCP_MAX_BACKOFF`

Compose and Kubernetes configuration should expose the same knobs when the
affected service already has related env configuration. Secrets such as
`RAG_INTERNAL_EVAL_TOKEN` must be wired through secret mechanisms, not
ConfigMaps.

## Tests

Tests should prove behavior is adjustable without code changes:

- Shared auth tests decode `sub` and `email`, classify operator allowlisted
  identities, classify normal users, and preserve anonymous local behavior when
  `JWT_SECRET` is empty.
- Eval API tests show operator read/poll traffic can exceed the normal
  `30/minute` policy while normal users still receive `429`.
- Eval API tests show run creation uses stricter limits than polling.
- Chat API tests show `/chat` and `/search` use separate tiered quotas.
- Chat API tests show internal eval traffic uses the internal eval tier only
  when the configured shared token matches.
- Ollama admission tests show permits are acquired/released for generate,
  embed, and judge paths, and overload returns `Retry-After`.
- MCP tests show `429` with `Retry-After` is retried after the requested delay,
  and `429` without `Retry-After` uses capped backoff.
- Config tests show env overrides change limits and invalid limit strings fail
  fast.

## Out Of Scope

- Fixing stuck rerank runs.
- Observability allowlisting.
- A full auth-service role/permission migration.
- Distributed Redis-backed Python rate limiting, unless local testing shows
  the current single-process deployment model is insufficient.

The only required coordination with observability is to emit enough structured
metadata for later dashboards: limiter name, endpoint group, tier, decision,
retry-after, and in-flight counts.
