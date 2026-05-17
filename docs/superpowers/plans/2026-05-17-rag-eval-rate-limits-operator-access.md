# RAG Eval Rate Limits And Operator Access Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add configurable operator-aware rate limits and Ollama admission control for RAG eval, chat, rerank, and MCP polling workflows.

**Architecture:** Replace hardcoded IP-only SlowAPI limits with a small shared FastAPI rate-limit layer keyed by auth tier and route group. Add shared async admission limiters around Python Ollama embedding/generation paths, including eval judge calls. Update the eval MCP client to respect `Retry-After` and use capped backoff when eval API polling receives `429`.

**Tech Stack:** Python/FastAPI, Pydantic settings, pytest/TestClient, httpx, Go eval MCP service, Go unit tests, existing Makefile preflights.

---

## Scope And Worktree

This changes application/runtime behavior and should be implemented from a
feature worktree, not directly on `main` or `qa`.

- Suggested branch: `feature/rag-eval-operator-rate-limits`
- Suggested worktree: `.codex/worktrees/rag-eval-operator-rate-limits`
- Target PR base: `qa`

Use `superpowers:using-git-worktrees` before implementation if the current
workspace is on `main` or `qa`.

## File Map

- Modify: `services/shared/auth.py`
  - Return an auth context with `subject`, `email`, and `tier`.
  - Preserve a compatibility dependency for existing endpoints that only need
    `user_id`.
- Test: `services/shared/tests/test_auth.py`
  - Cover operator, user, anonymous, and malformed token cases.
- Create: `services/shared/rate_limits.py`
  - Implement configurable fixed-window limits by route group and tier.
  - Return `429` with `Retry-After` on quota exhaustion.
- Test: `services/shared/tests/test_rate_limits.py`
  - Cover quota parsing, per-tier keys, retry-after, and invalid config.
- Create: `services/shared/llm/admission.py`
  - Implement async concurrency admission with queue timeout and retry-after.
- Test: `services/shared/tests/test_llm_admission.py`
  - Cover acquire/release, timeout, and config validation.
- Modify: `services/chat/app/config.py`
  - Add chat quota and admission env settings.
- Modify: `services/chat/app/main.py`
  - Replace hardcoded `/chat` and `/search` SlowAPI limits with tiered limits.
  - Accept internal eval token for eval worker requests.
- Modify: `services/chat/app/chain.py`
  - Acquire embedding/generation/rerank admission around expensive work.
- Test: `services/chat/tests/test_config.py`
- Test: `services/chat/tests/test_main.py`
- Test: `services/chat/tests/test_chain.py`
- Modify: `services/eval/app/config.py`
  - Add eval quota, operator allowlist, internal token, and admission env
    settings.
- Modify: `services/eval/app/main.py`
  - Replace hardcoded eval SlowAPI decorators with route-group tier limits.
  - Pass internal eval token to chat through `RAGClient`.
  - Apply judge generation admission.
- Modify: `services/eval/app/rag_client.py`
  - Send internal eval header to chat when configured.
- Modify: `services/eval/app/evaluator.py`
  - Acquire generation admission for judge calls.
- Test: `services/eval/tests/test_config.py`
- Test: `services/eval/tests/test_main.py`
- Test: `services/eval/tests/test_rag_client.py`
- Test: `services/eval/tests/test_evaluator.py`
- Modify: `go/eval-mcp-service/internal/evalapi/client.go`
  - Preserve response headers on HTTP errors, especially `Retry-After`.
- Modify: `go/eval-mcp-service/internal/evalworkflow/service.go`
  - Back off on `429` while polling.
- Modify: `go/eval-mcp-service/internal/config/config.go`
  - Add `EVAL_MCP_MAX_BACKOFF`.
- Test: `go/eval-mcp-service/internal/evalapi/client_test.go`
- Test: `go/eval-mcp-service/internal/evalworkflow/service_test.go`
- Test: `go/eval-mcp-service/internal/config/config_test.go`
- Modify: `docker-compose.yml`
  - Add non-secret rate/admission env defaults for chat/eval.
- Modify: relevant Kubernetes manifests for Python chat/eval services if they
  exist in this repo.
  - Put non-secret knobs in ConfigMaps.
  - Put `RAG_INTERNAL_EVAL_TOKEN` in a Secret only if Python service manifests
    already manage comparable secrets.
- Modify: `go/eval-mcp-service/README.md`
  - Document MCP backoff env vars and behavior.

## Task 1: Create The Feature Worktree

**Files:**
- No source files modified.

- [ ] **Step 1: Confirm current branch and status**

Run:

```bash
pwd
git branch --show-current
git status --short
```

Expected: current repo is `/Users/kylebradshaw/repos/gen_ai_engineer`; if on
`main` or `qa`, create a feature worktree before editing code.

- [ ] **Step 2: Create and enter the worktree**

Run:

```bash
git worktree add .codex/worktrees/rag-eval-operator-rate-limits -b feature/rag-eval-operator-rate-limits
cd .codex/worktrees/rag-eval-operator-rate-limits
pwd
git branch --show-current
git rev-parse --show-toplevel
```

Expected: branch is `feature/rag-eval-operator-rate-limits`; top-level path is
inside `.codex/worktrees/rag-eval-operator-rate-limits`.

- [ ] **Step 3: Load scoped instructions**

Run:

```bash
sed -n '1,220p' AGENTS.md
sed -n '1,220p' services/AGENTS.md
sed -n '1,220p' go/AGENTS.md
```

Expected: instructions are available before code edits.

## Task 2: Add Shared Auth Context

**Files:**
- Modify: `services/shared/auth.py`
- Test: `services/shared/tests/test_auth.py`

- [ ] **Step 1: Write failing auth context tests**

Add tests covering these cases in `services/shared/tests/test_auth.py`:

```python
def test_auth_context_classifies_operator_by_email(monkeypatch):
    monkeypatch.setenv("RAG_OPERATOR_EMAILS", "kyle@example.test")
    dependency = create_auth_context_dependency("secret")
    token = _token("secret", sub="user-1", email="kyle@example.test")
    request = _request(headers={"Authorization": f"Bearer {token}"})

    context = anyio.run(dependency, request, None)

    assert context.subject == "user-1"
    assert context.email == "kyle@example.test"
    assert context.tier == "operator"


def test_auth_context_classifies_normal_user(monkeypatch):
    monkeypatch.setenv("RAG_OPERATOR_EMAILS", "kyle@example.test")
    dependency = create_auth_context_dependency("secret")
    token = _token("secret", sub="user-2", email="other@example.test")
    request = _request(headers={"Authorization": f"Bearer {token}"})

    context = anyio.run(dependency, request, None)

    assert context.subject == "user-2"
    assert context.email == "other@example.test"
    assert context.tier == "user"


def test_auth_context_preserves_anonymous_when_secret_empty():
    dependency = create_auth_context_dependency("")
    request = _request()

    context = anyio.run(dependency, request, None)

    assert context.subject == "anonymous"
    assert context.email is None
    assert context.tier == "anonymous"
```

Use existing test helpers if present; otherwise add small helpers that build a
Starlette `Request` and HS256 JWT.

- [ ] **Step 2: Run the focused test and verify failure**

Run:

```bash
PYTHONPATH=services/shared pytest services/shared/tests/test_auth.py -q
```

Expected: fails because `create_auth_context_dependency` and `AuthContext` do
not exist.

- [ ] **Step 3: Implement auth context**

In `services/shared/auth.py`, add:

```python
from dataclasses import dataclass


@dataclass(frozen=True)
class AuthContext:
    subject: str
    email: str | None
    tier: str
```

Add helper functions:

```python
def _csv_set(value: str | None) -> set[str]:
    return {part.strip().lower() for part in (value or "").split(",") if part.strip()}


def _tier_for(subject: str, email: str | None) -> str:
    operator_subjects = _csv_set(os.getenv("RAG_OPERATOR_USER_IDS"))
    operator_emails = _csv_set(os.getenv("RAG_OPERATOR_EMAILS"))
    if subject.lower() in operator_subjects:
        return "operator"
    if email and email.lower() in operator_emails:
        return "operator"
    return "user"
```

Add `create_auth_context_dependency(secret: str)` that follows the existing
Bearer/cookie resolution and JWT validation, then returns `AuthContext`.

Keep `create_auth_dependency(secret: str)` as a compatibility wrapper:

```python
def create_auth_dependency(secret: str):
    auth_context = create_auth_context_dependency(secret)

    async def require_user_id(...):
        context = await auth_context(...)
        return context.subject

    return require_user_id
```

The exact wrapper signature must match FastAPI dependency injection and existing
tests.

- [ ] **Step 4: Run shared auth tests**

Run:

```bash
PYTHONPATH=services/shared pytest services/shared/tests/test_auth.py -q
```

Expected: pass.

- [ ] **Step 5: Commit**

Run:

```bash
git add services/shared/auth.py services/shared/tests/test_auth.py
git commit -m "feat: add RAG auth context tiers"
```

## Task 3: Add Shared FastAPI Rate Limits

**Files:**
- Create: `services/shared/rate_limits.py`
- Test: `services/shared/tests/test_rate_limits.py`

- [ ] **Step 1: Write failing rate-limit tests**

Create tests for:

```python
def test_tiered_fixed_window_allows_operator_more_than_user():
    limiter = FixedWindowRateLimiter(
        clock=FakeClock(0.0),
        policies={
            "eval_read": {
                "operator": RateLimit(max_requests=3, window_seconds=60),
                "user": RateLimit(max_requests=1, window_seconds=60),
            }
        },
    )
    operator = AuthContext(subject="kyle", email="kyle@example.test", tier="operator")
    user = AuthContext(subject="u1", email="u1@example.test", tier="user")

    assert limiter.check("eval_read", operator).allowed
    assert limiter.check("eval_read", operator).allowed
    assert limiter.check("eval_read", operator).allowed
    assert not limiter.check("eval_read", operator).allowed
    assert limiter.check("eval_read", user).allowed
    denied = limiter.check("eval_read", user)
    assert not denied.allowed
    assert denied.retry_after_seconds == 60
```

Also test invalid strings such as `"abc/minute"` fail during parsing.

- [ ] **Step 2: Run the focused test and verify failure**

Run:

```bash
PYTHONPATH=services/shared pytest services/shared/tests/test_rate_limits.py -q
```

Expected: fails because the module does not exist.

- [ ] **Step 3: Implement the limiter**

Implement:

```python
@dataclass(frozen=True)
class RateLimit:
    max_requests: int
    window_seconds: int


@dataclass(frozen=True)
class RateLimitDecision:
    allowed: bool
    retry_after_seconds: int
```

Implement `parse_rate_limit(value: str) -> RateLimit` for forms like
`"30/minute"`, `"240/minute"`, and `"0/minute"`. A zero limit means always
deny.

Implement `FixedWindowRateLimiter.check(group: str, context: AuthContext)` using
the key `(group, context.tier, context.subject)`. Store counters in memory and
return retry-after as the seconds remaining in the current window.

Implement `rate_limit_dependency(group: str, limiter, auth_context_dependency)`
that raises:

```python
HTTPException(
    status_code=429,
    detail="Rate limit exceeded",
    headers={"Retry-After": str(decision.retry_after_seconds)},
)
```

- [ ] **Step 4: Run rate-limit tests**

Run:

```bash
PYTHONPATH=services/shared pytest services/shared/tests/test_rate_limits.py -q
```

Expected: pass.

- [ ] **Step 5: Commit**

Run:

```bash
git add services/shared/rate_limits.py services/shared/tests/test_rate_limits.py
git commit -m "feat: add shared tiered rate limiter"
```

## Task 4: Add Shared Ollama Admission

**Files:**
- Create: `services/shared/llm/admission.py`
- Test: `services/shared/tests/test_llm_admission.py`

- [ ] **Step 1: Write failing admission tests**

Create tests for:

```python
@pytest.mark.anyio
async def test_admission_rejects_after_queue_timeout():
    limiter = AsyncAdmissionLimiter(max_in_flight=1, queue_timeout_seconds=0.01)
    first = await limiter.acquire()
    with pytest.raises(AdmissionRejected) as exc:
        await limiter.acquire()
    assert exc.value.retry_after_seconds >= 1
    first.release()


@pytest.mark.anyio
async def test_admission_releases_permit():
    limiter = AsyncAdmissionLimiter(max_in_flight=1, queue_timeout_seconds=0.01)
    first = await limiter.acquire()
    first.release()
    second = await limiter.acquire()
    second.release()
```

- [ ] **Step 2: Run the focused test and verify failure**

Run:

```bash
PYTHONPATH=services/shared pytest services/shared/tests/test_llm_admission.py -q
```

Expected: fails because `llm.admission` does not exist.

- [ ] **Step 3: Implement async admission**

Implement `AsyncAdmissionLimiter` with `asyncio.Semaphore`, `asyncio.wait_for`,
and idempotent permit release. Implement `AdmissionRejected` with
`retry_after_seconds`.

Export module-level helpers:

```python
generate_limiter = AsyncAdmissionLimiter.from_env(
    max_key="OLLAMA_GENERATE_MAX_IN_FLIGHT",
    timeout_key="OLLAMA_ADMISSION_QUEUE_TIMEOUT",
    default_max=2,
    default_timeout_seconds=5.0,
)
embed_limiter = AsyncAdmissionLimiter.from_env(
    max_key="OLLAMA_EMBED_MAX_IN_FLIGHT",
    timeout_key="OLLAMA_ADMISSION_QUEUE_TIMEOUT",
    default_max=4,
    default_timeout_seconds=5.0,
)
rerank_limiter = AsyncAdmissionLimiter.from_env(
    max_key="RERANK_MAX_IN_FLIGHT",
    timeout_key="OLLAMA_ADMISSION_QUEUE_TIMEOUT",
    default_max=2,
    default_timeout_seconds=5.0,
)
```

- [ ] **Step 4: Run admission tests**

Run:

```bash
PYTHONPATH=services/shared pytest services/shared/tests/test_llm_admission.py -q
```

Expected: pass.

- [ ] **Step 5: Commit**

Run:

```bash
git add services/shared/llm/admission.py services/shared/tests/test_llm_admission.py
git commit -m "feat: add Python Ollama admission limiter"
```

## Task 5: Wire Tiered Limits Into Chat

**Files:**
- Modify: `services/chat/app/config.py`
- Modify: `services/chat/app/main.py`
- Test: `services/chat/tests/test_config.py`
- Test: `services/chat/tests/test_main.py`

- [ ] **Step 1: Write failing chat tests**

Add tests that configure:

```python
CHAT_RATE_LIMIT_ASK_OPERATOR=3/minute
CHAT_RATE_LIMIT_ASK_USER=1/minute
CHAT_RATE_LIMIT_ASK_ANONYMOUS=0/minute
CHAT_RATE_LIMIT_SEARCH_INTERNAL_EVAL=3/minute
RAG_INTERNAL_EVAL_TOKEN=test-internal-token
```

Test expected behavior:

```python
def test_chat_operator_has_higher_ask_limit(...):
    # operator token can make three /chat JSON requests in one window
    # normal user receives 429 on the second request
    # 429 response includes Retry-After


def test_search_internal_eval_token_uses_internal_eval_limit(...):
    # X-RAG-Internal-Token: test-internal-token gets internal_eval tier
    # missing or wrong token does not get internal_eval tier
```

Patch `rag_query` and `retrieve_chunks` so tests do not call Ollama or Qdrant.

- [ ] **Step 2: Run focused chat tests and verify failure**

Run:

```bash
PYTHONPATH=services/shared:services/chat pytest services/chat/tests/test_main.py services/chat/tests/test_config.py -q
```

Expected: fails because chat still uses hardcoded SlowAPI decorators and lacks
new settings.

- [ ] **Step 3: Add config settings**

In `services/chat/app/config.py`, add string settings for the chat/search
limits and internal token. Defaults:

```python
chat_rate_limit_ask_operator: str = "60/minute"
chat_rate_limit_ask_user: str = "20/minute"
chat_rate_limit_ask_anonymous: str = "0/minute"
chat_rate_limit_ask_internal_eval: str = "60/minute"
chat_rate_limit_search_operator: str = "120/minute"
chat_rate_limit_search_user: str = "30/minute"
chat_rate_limit_search_anonymous: str = "0/minute"
chat_rate_limit_search_internal_eval: str = "120/minute"
rag_internal_eval_token: str = ""
```

- [ ] **Step 4: Replace hardcoded chat limits**

In `services/chat/app/main.py`:

- Remove `@limiter.limit("20/minute")` from `/chat`.
- Remove `@limiter.limit("30/minute")` from `/search`.
- Use `create_auth_context_dependency(settings.jwt_secret)`.
- Build one `FixedWindowRateLimiter` from chat settings.
- Add a dependency or explicit call at endpoint start for group `chat_ask` and
  `chat_search`.
- Classify `internal_eval` only when `X-RAG-Internal-Token` matches
  `settings.rag_internal_eval_token`.

Keep existing auth behavior for browser/user traffic.

- [ ] **Step 5: Run focused chat tests**

Run:

```bash
PYTHONPATH=services/shared:services/chat pytest services/chat/tests/test_main.py services/chat/tests/test_config.py -q
```

Expected: pass.

- [ ] **Step 6: Commit**

Run:

```bash
git add services/chat/app/config.py services/chat/app/main.py services/chat/tests/test_config.py services/chat/tests/test_main.py
git commit -m "feat: add operator-aware chat rate limits"
```

## Task 6: Wire Ollama Admission Into Chat

**Files:**
- Modify: `services/chat/app/chain.py`
- Test: `services/chat/tests/test_chain.py`

- [ ] **Step 1: Write failing chat admission tests**

Add tests that patch `embed_limiter.acquire`, `generate_limiter.acquire`, and
`rerank_limiter.acquire` with async fakes that record calls and return permits.

Expected behavior:

```python
async def test_retrieve_acquires_embedding_admission(...):
    await retrieve_chunks(...)
    assert embed_acquire_called_once


async def test_rag_query_acquires_generation_admission(...):
    events = [event async for event in rag_query(...)]
    assert generate_acquire_called_once


async def test_rerank_acquires_rerank_admission_when_requested(...):
    await retrieve_chunks(..., rerank=True)
    assert rerank_acquire_called_once
```

- [ ] **Step 2: Run focused tests and verify failure**

Run:

```bash
PYTHONPATH=services/shared:services/chat pytest services/chat/tests/test_chain.py -q
```

Expected: fails because admission is not used.

- [ ] **Step 3: Add admission calls**

In `services/chat/app/chain.py`:

- Wrap `provider.embed(texts)` in an `embed_limiter` permit.
- Wrap `provider.generate(...)` in a `generate_limiter` permit.
- Wrap `rerank_chunks(...)` in a `rerank_limiter` permit when rerank is
  requested and enabled.
- Convert `AdmissionRejected` to a clear exception that the endpoint maps to
  overload with `Retry-After`.

- [ ] **Step 4: Run focused chain tests**

Run:

```bash
PYTHONPATH=services/shared:services/chat pytest services/chat/tests/test_chain.py -q
```

Expected: pass.

- [ ] **Step 5: Commit**

Run:

```bash
git add services/chat/app/chain.py services/chat/tests/test_chain.py
git commit -m "feat: protect chat Ollama work with admission"
```

## Task 7: Wire Tiered Limits And Internal Eval Token Into Eval

**Files:**
- Modify: `services/eval/app/config.py`
- Modify: `services/eval/app/main.py`
- Modify: `services/eval/app/rag_client.py`
- Test: `services/eval/tests/test_config.py`
- Test: `services/eval/tests/test_main.py`
- Test: `services/eval/tests/test_rag_client.py`

- [ ] **Step 1: Write failing eval rate-limit tests**

Add tests proving:

- Operator can poll `GET /evaluations/{eval_id}` more times than normal user.
- Normal user receives `429` and `Retry-After` after configured read quota.
- `POST /evaluations` uses the stricter run-create quota.
- `RAGClient` sends `X-RAG-Internal-Token` only when configured.

Use small test limits such as `2/minute` and `1/minute`.

- [ ] **Step 2: Run focused tests and verify failure**

Run:

```bash
PYTHONPATH=services/shared:services/eval pytest services/eval/tests/test_main.py services/eval/tests/test_config.py services/eval/tests/test_rag_client.py -q
```

Expected: fails because eval still uses hardcoded SlowAPI decorators and
`RAGClient` has no internal token support.

- [ ] **Step 3: Add eval config settings**

In `services/eval/app/config.py`, add defaults:

```python
eval_rate_limit_run_create_operator: str = "20/minute"
eval_rate_limit_run_create_user: str = "5/minute"
eval_rate_limit_run_create_anonymous: str = "0/minute"
eval_rate_limit_read_operator: str = "240/minute"
eval_rate_limit_read_user: str = "30/minute"
eval_rate_limit_read_anonymous: str = "10/minute"
eval_rate_limit_write_operator: str = "30/minute"
eval_rate_limit_write_user: str = "10/minute"
eval_rate_limit_write_anonymous: str = "0/minute"
rag_internal_eval_token: str = ""
```

- [ ] **Step 4: Replace hardcoded eval limits**

In `services/eval/app/main.py`:

- Remove hardcoded `@limiter.limit(...)` decorators.
- Use route groups:
  - `eval_run_create`: `POST /evaluations`
  - `eval_write`: dataset/experiment create/update/attach
  - `eval_read`: dataset lists, experiment reads, evaluation lists, history,
    dashboard, compare, and `GET /evaluations/{eval_id}`
- Use auth context tier for all checks.
- Keep auth required where currently required.

- [ ] **Step 5: Pass internal eval token to chat**

In `services/eval/app/rag_client.py`, accept `internal_token: str = ""` and add:

```python
headers = {}
if self._internal_token:
    headers["X-RAG-Internal-Token"] = self._internal_token
```

Apply the header to `/search` and `/chat` calls. In
`services/eval/app/main.py`, construct:

```python
rag_client = RAGClient(
    base_url=settings.chat_service_url,
    internal_token=settings.rag_internal_eval_token,
)
```

- [ ] **Step 6: Run focused eval tests**

Run:

```bash
PYTHONPATH=services/shared:services/eval pytest services/eval/tests/test_main.py services/eval/tests/test_config.py services/eval/tests/test_rag_client.py -q
```

Expected: pass.

- [ ] **Step 7: Commit**

Run:

```bash
git add services/eval/app/config.py services/eval/app/main.py services/eval/app/rag_client.py services/eval/tests/test_config.py services/eval/tests/test_main.py services/eval/tests/test_rag_client.py
git commit -m "feat: add operator-aware eval rate limits"
```

## Task 8: Wire Ollama Admission Into Eval Judge Calls

**Files:**
- Modify: `services/eval/app/evaluator.py`
- Test: `services/eval/tests/test_evaluator.py`

- [ ] **Step 1: Write failing eval admission test**

Add a test that patches `generate_limiter.acquire` and asserts
`judge_generation_scores(...)` acquires and releases a permit around
`llm.chat(...)`.

- [ ] **Step 2: Run focused test and verify failure**

Run:

```bash
PYTHONPATH=services/shared:services/eval pytest services/eval/tests/test_evaluator.py -q
```

Expected: fails because judge calls do not use admission.

- [ ] **Step 3: Add judge admission**

In `services/eval/app/evaluator.py`, wrap `llm.chat(...)` with
`generate_limiter.acquire()`. Ensure release happens in `finally` so failed
judge calls do not leak permits.

- [ ] **Step 4: Run focused evaluator tests**

Run:

```bash
PYTHONPATH=services/shared:services/eval pytest services/eval/tests/test_evaluator.py -q
```

Expected: pass.

- [ ] **Step 5: Commit**

Run:

```bash
git add services/eval/app/evaluator.py services/eval/tests/test_evaluator.py
git commit -m "feat: protect eval judge calls with admission"
```

## Task 9: Add MCP Retry-After And Backoff

**Files:**
- Modify: `go/eval-mcp-service/internal/evalapi/client.go`
- Modify: `go/eval-mcp-service/internal/evalworkflow/service.go`
- Modify: `go/eval-mcp-service/internal/config/config.go`
- Test: `go/eval-mcp-service/internal/evalapi/client_test.go`
- Test: `go/eval-mcp-service/internal/evalworkflow/service_test.go`
- Test: `go/eval-mcp-service/internal/config/config_test.go`

- [ ] **Step 1: Write failing Go client tests**

Add tests:

```go
func TestHTTPErrorIncludesRetryAfter(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Retry-After", "7")
        http.Error(w, "slow down", http.StatusTooManyRequests)
    }))
    defer server.Close()

    client := New(server.URL, "", server.Client())
    _, err := client.GetEvaluation(context.Background(), "eval-1")

    var httpErr *HTTPError
    if !errors.As(err, &httpErr) {
        t.Fatalf("err = %T, want *HTTPError", err)
    }
    if httpErr.RetryAfter != 7*time.Second {
        t.Fatalf("RetryAfter = %s", httpErr.RetryAfter)
    }
}
```

- [ ] **Step 2: Write failing workflow backoff test**

Use a fake API that returns `HTTPError{StatusCode: 429, RetryAfter: 10 *
time.Millisecond}` once, then returns a completed run. Assert `WaitForRun`
continues instead of returning immediately.

- [ ] **Step 3: Run focused Go tests and verify failure**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/evalapi ./internal/evalworkflow ./internal/config
```

Expected: fails because `HTTPError` does not expose retry-after and workflow
does not back off on `429`.

- [ ] **Step 4: Preserve Retry-After in HTTPError**

In `internal/evalapi/client.go`, add:

```go
RetryAfter time.Duration
```

Parse `resp.Header.Get("Retry-After")` as seconds first. If parsing fails, keep
zero duration.

- [ ] **Step 5: Add MCP config**

In `internal/config/config.go`, add `MaxBackoff time.Duration`, env
`EVAL_MCP_MAX_BACKOFF`, default `30s`, and positive-duration validation.

- [ ] **Step 6: Back off in WaitForRun**

In `internal/evalworkflow/service.go`, when `GetEvaluation` returns an
`evalapi.HTTPError` with status `429`:

- Sleep for `HTTPError.RetryAfter` when positive.
- Otherwise sleep for exponential backoff capped by `MaxBackoff`.
- Reset backoff after a successful poll.
- Preserve timeout behavior.

Use jitter only if it can be tested deterministically by injecting a clock or
random source. Otherwise use deterministic capped exponential backoff.

- [ ] **Step 7: Run focused Go tests**

Run:

```bash
cd go/eval-mcp-service
go test ./internal/evalapi ./internal/evalworkflow ./internal/config
```

Expected: pass.

- [ ] **Step 8: Commit**

Run:

```bash
git add go/eval-mcp-service/internal/evalapi/client.go go/eval-mcp-service/internal/evalworkflow/service.go go/eval-mcp-service/internal/config/config.go go/eval-mcp-service/internal/evalapi/client_test.go go/eval-mcp-service/internal/evalworkflow/service_test.go go/eval-mcp-service/internal/config/config_test.go
git commit -m "feat: back off eval MCP polling on rate limits"
```

## Task 10: Add Env Wiring And README Notes

**Files:**
- Modify: `docker-compose.yml`
- Modify: `go/eval-mcp-service/README.md`
- Modify: Python chat/eval Kubernetes manifests if present.

- [ ] **Step 1: Locate Python chat/eval manifests narrowly**

Run:

```bash
rg -n "chat-service|eval-service|services/chat|services/eval|JWT_SECRET|CHAT_RATE_LIMIT|EVAL_RATE_LIMIT|RAG_INTERNAL_EVAL_TOKEN" k8s services go/k8s docker-compose.yml -S
```

Expected: exact files to update are identified. Do not broadly scan docs.

- [ ] **Step 2: Add compose env defaults**

In `docker-compose.yml`, add non-secret defaults for chat and eval services.
Use defaults matching the spec. Do not put `RAG_INTERNAL_EVAL_TOKEN` literal
values in compose unless it already comes from `${RAG_INTERNAL_EVAL_TOKEN}`.

- [ ] **Step 3: Add Kubernetes config carefully**

If Python chat/eval manifests exist:

- Put non-secret knobs in ConfigMaps or deployment env.
- Put `RAG_INTERNAL_EVAL_TOKEN` behind the same secret mechanism used for
  `JWT_SECRET`.
- Do not modify unrelated Go `ai-service` config.

- [ ] **Step 4: Update MCP README**

In `go/eval-mcp-service/README.md`, document:

```markdown
- `EVAL_MCP_MAX_BACKOFF`: maximum delay after repeated eval API `429`
  responses, defaults to `30s`.
- The client respects `Retry-After` from eval API responses before applying
  capped backoff.
```

- [ ] **Step 5: Commit**

Run:

```bash
git add docker-compose.yml go/eval-mcp-service/README.md
git add <python-chat-eval-k8s-files-if-present>
git commit -m "chore: document RAG eval rate limit configuration"
```

## Task 11: Run Verification

**Files:**
- No planned source edits unless verification fails.

- [ ] **Step 1: Run focused Python tests**

Run:

```bash
PYTHONPATH=services/shared:services/chat pytest services/shared/tests/test_auth.py services/shared/tests/test_rate_limits.py services/shared/tests/test_llm_admission.py services/chat/tests/test_config.py services/chat/tests/test_main.py services/chat/tests/test_chain.py -q
PYTHONPATH=services/shared:services/eval pytest services/eval/tests/test_config.py services/eval/tests/test_main.py services/eval/tests/test_rag_client.py services/eval/tests/test_evaluator.py -q
```

Expected: pass.

- [ ] **Step 2: Run focused Go tests**

Run:

```bash
cd go/eval-mcp-service
go test ./...
```

Expected: pass.

- [ ] **Step 3: Run preflights**

Run from repo root:

```bash
make preflight-python
make preflight-go
make preflight-security
```

Expected: pass. If blocked by local tooling, record the blocker and leave the
remaining check to CI.

- [ ] **Step 4: Inspect diff**

Run:

```bash
git diff --stat qa...HEAD
git diff qa...HEAD -- services/shared services/chat services/eval go/eval-mcp-service docker-compose.yml
```

Expected: diff is limited to the files in this plan plus any discovered Python
chat/eval manifest files.

- [ ] **Step 5: Final commit if fixes were needed**

Run:

```bash
git status --short
git add <changed-files>
git commit -m "test: verify RAG eval rate limit hardening"
```

Expected: commit only if verification required code/test fixes after the prior
task commits.

## Task 12: Push And Open PR

**Files:**
- No source edits.

- [ ] **Step 1: Push feature branch**

Run:

```bash
git push -u origin feature/rag-eval-operator-rate-limits
```

Expected: branch pushed.

- [ ] **Step 2: Open PR to qa**

Run:

```bash
gh pr create --base qa --head feature/rag-eval-operator-rate-limits --title "Add operator-aware RAG eval rate limits" --body "Adds configurable operator/user/internal rate limits for RAG eval and chat, Ollama admission control for Python RAG workloads, and MCP polling backoff for eval API 429 responses."
```

Expected: PR created. Do not watch CI unless Kyle asks.

## Self-Review

- Spec coverage: current enforcement points, protected traffic, identity tiers,
  quotas, Ollama admission, MCP 429 handling, tests, and config knobs all map to
  tasks above.
- Placeholder scan: no task depends on undefined future work; discovered
  Kubernetes files are intentionally located by a narrow command because the
  exact Python manifests were not part of the initial inspected scope.
- Type consistency: `AuthContext`, `FixedWindowRateLimiter`,
  `AsyncAdmissionLimiter`, `AdmissionRejected`, and `HTTPError.RetryAfter` are
  introduced before later tasks rely on them.
