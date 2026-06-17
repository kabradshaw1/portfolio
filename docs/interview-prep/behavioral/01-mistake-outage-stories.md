# Mistake / Production Outage Stories (CARL)

Question shape: "Tell me about a time you made a mistake or caused a
production outage. How did you handle it?"

Format: **C**ontext, **A**ction, **R**esult, **L**earning.
Target delivery: 60-90 seconds. Lead with the system, the impact, then
the fix. Save the systemic lesson for last so the interviewer remembers it.

Stories are ranked best-first. Pick one based on the role's signal:
distributed systems → Story 1; security/web depth → Story 2; ops/K8s
depth → Story 3; observability → Story 4.

---

## Story 1 — Saga refactor broke double-charge protection

**Best for:** distributed systems, backend, senior IC roles.
**Evidence:** CI run 24737144011, commit `145c685`, follow-ups
`afbad3ad`, `05e3534`, `69c308e` (2026-04-21). Smoke test
`full checkout lifecycle: cart → order → verify`.

### Context
I was migrating the order service in my Go ecommerce portfolio from a
synchronous "checkout endpoint clears the cart" call into a Kafka /
RabbitMQ saga so payment, inventory, and cart-clearing could each fail
and be retried independently. The saga shipped, smoke tests went green
on the first deploy, and I moved on.

### Action
The next push to main failed the smoke test on
`cart should be empty after saga completes (5s timeout)`. From a user's
perspective the bug was worse than the test described: they could
check out, see an order confirmation, go back to the cart, see their
items still there, and check out a *second* time — and because I
hadn't made the payment step idempotent, that second checkout would
create a second Stripe payment intent for the same order. I rolled
through three things in order. First, I added an idempotency key to
payment creation keyed on order ID so a retry would return the
existing intent instead of creating a new one. Second, I put a 30s
gRPC context deadline on the payment call so a hung handshake would
fail fast instead of leaving the saga in flight. Third — and this is
the one I almost missed — I changed the smoke test and the cart UI to
poll for up to 5s instead of asserting the cart was empty immediately,
because in a saga world "checkout returned 200" no longer means "cart
is empty."

### Result
Double-charge risk was closed before any real customer hit it (this is
a portfolio system, but the smoke test would have shipped the bug to
production if I'd ignored it). The saga step duration and error
metrics I added in the same window gave me per-step alerting so the
next saga regression pages instead of silently producing inconsistent
state.

### Learning
The mistake wasn't the saga — it was carrying a synchronous mental
model into an eventually-consistent system. "The checkout returned
200" stopped being a useful assertion the moment the cart-clear became
async, and I had to update *both* the UI and the test suite to admit
that. Now whenever I introduce an async boundary I ask two questions
up front: what's the idempotency key on every step that mutates
external state, and which existing assertions just silently became
wrong?

---

## Story 2 — Logged-in users were silently anonymous in production

**Best for:** web platform, security, full-stack roles.
**Evidence:** CI runs 24612756851, 24614834684, 24614428433,
24615018988 (2026-04-18). Commits `47f487e` (Go auth),
`cb9ba72` (Java task service), `797e4a9` (gateway forwarding).

### Context
I split my portfolio into a storefront at `kylebradshaw.dev` and an
API at `api.kylebradshaw.dev` so the deploys could be independent. I
deployed the split, the login form worked, the user landed on the
dashboard — and every authenticated API call came back 401. Smoke
tests had been hitting same-origin localhost so they hadn't caught
anything.

### Action
The misleading part was that login *succeeded* — the `Set-Cookie`
header was on the response, but the next `fetch()` to
`api.kylebradshaw.dev` didn't send it. I checked DevTools and found
the browser silently dropping the cookie on every cross-site request.
Root cause was three layers stacked: the Go auth service and the Java
task service both hardcoded `SameSite=Lax` with no `Domain` attribute,
and the Java gateway wasn't translating cookies back to a Bearer token
for downstream services even when cookies *were* sent. I made the
`SameSite` policy configurable through a `COOKIE_SAMESITE` env var
defaulting to `None` in cross-site environments, set
`Domain=.kylebradshaw.dev` and `Secure=true`, and added cookie-to-
Bearer forwarding at the gateway. I hit a second trap mid-fix: I
updated the ConfigMap with the new cookie settings and pods kept
serving the old policy because the image tag hadn't changed and
Kubernetes never re-pulled. That's why four runs failed back-to-back
that evening — two of them were "force image rebuild" commits.

### Result
Auth was back end-to-end. The Java gateway also got an exception
handler that exposes the actual error message in dev environments,
because the original generic 500 had made the cookie debugging much
slower than it needed to be.

### Learning
Two things stuck. First, "smoke tests pass and login works" doesn't
mean auth works — you have to assert *cross-origin* auth specifically
when the system is split across domains. Second, in Kubernetes a
config change without an image tag bump is a deploy that may or may
not actually take effect. I now bump the image tag on any change that
crosses the container boundary, and I treat "force rebuild" commits
in my history as a smell I want to design out, not a routine fix.

---

## Story 3 — Java services were silently OOM-killed on rotation

**Best for:** ops / SRE / Kubernetes-heavy roles.
**Evidence:** CI run 24644748706, commit `940e747` (2026-04-20).
Follow-up alerting commit `fd6c54d`.

### Context
The Java side of my portfolio runs a gateway, a task service, a
notification service, and an activity service on a self-hosted
Kubernetes cluster. Users on the task management page started seeing
intermittent 502s. Pods were cycling every few hours with `OOMKilled`,
but I didn't have restart-count alerts in place yet, so I only noticed
when smoke tests started flaking.

### Action
First instinct was a memory leak. I attached `jstack` and watched heap
through `kubectl top pod` and the heap was nowhere near its supposed
limit when the OOM kill happened. That was the clue: the JVM's
ergonomic max heap on a 512Mi cgroup was around 128Mi, but non-heap —
metaspace, native threads, direct buffers — was routinely pushing
container RSS over the cgroup limit. The Dockerfiles all ran
`java -jar app.jar` with no `-Xmx`, so the JVM was sizing itself
against numbers it inferred from the container, and the inference was
wrong. The fix was two lines: pin `-Xmx512m` in every Java Dockerfile
and raise the container memory limit to 768Mi to leave headroom for
non-heap. Then I added Kubernetes alert rules on restart count and
OOM events so the next regression in this shape would page me instead
of hiding behind a healthy `/actuator/health`.

### Result
Restart count dropped to zero in steady state and the 502s went away
within a rollout. The alerting change paid off two weeks later when
an unrelated service started restarting on probe failures and I got
paged within minutes instead of finding it through flaky tests.

### Learning
The headline lesson is the technical one — never ship a JVM container
without an explicit `-Xmx`, because the JVM's view of "the box" and
Kubernetes's view of "the box" don't match. But the second-order
lesson is what changed how I work: the bug wasn't really an OOM, it
was that a pod could die every few hours and nothing in my monitoring
would tell me. The fix isn't a heap flag — the fix is treating
restart count as a first-class SLI.

---

## Story 4 — A middleware was eating every 5xx error in silence

**Best for:** observability, platform, AI/infra roles.
**Evidence:** `frontend/src/app/observability/page.tsx:124-142` (the
incident write-up). Related infra: RabbitMQ `/qa` vhost isolation
(`098d58d`).

### Context
A Stripe checkout in my Go ecommerce service was failing for users in a specific shape: the payment confirmation page loaded, the cart was still full, and no order showed up in their orders list. Loki — my logging stack — had zero ERROR logs for the entire 24-hour window I was searching.

### Action
"Zero errors" was the lie I had to break first. I went and read my own middleware. The `apperror.ErrorHandler()` I'd written months earlier was catching every 5xx, converting it to a clean JSON response for the client, and returning *without logging anything* — the structured logger sat above it in the chain. I'd built a perfectly quiet error path. The actual underlying bug, once I added the `slog.Error` call and could see traffic, was worse than the middleware hiding it: QA and production were sharing a single RabbitMQ instance with identical queue names, and a QA smoke test publishing to `clear.cart` was being consumed by the live production cart-service. Real customer carts were being cleared by my QA runs. Fix went in three parts: the middleware now logs every 5xx with full context before responding, I migrated QA onto a dedicated `/qa` vhost so queue names can never collide across environments again, and I added a `saga-order-stalled` Grafana alert so any future stuck order would page within 30 minutes instead of waiting for a customer complaint.

### Result
The silent failure mode is gone — both the symptom (orders that died without trace) and the underlying cause (shared message broker state between environments). The alert has caught two unrelated saga regressions since.

### Learning
The mistake I keep coming back to here isn't the shared vhost — that was a setup shortcut I knew was wrong. The mistake was building a middleware that *looked* correct because it returned the right shape to the client, while making the system unobservable from the inside. Now whenever I write or review an error handler I ask one question first: if this code path fires at 3 a.m., what query in Loki would let me find out? If the answer is "none," the handler isn't done.

---

## Delivery tips

- Pick *one* story and own it. Don't string them together — the
  follow-up questions will be richer if you go deep.
- Be honest about the cause being yours. The interviewer is asking
  about a mistake, not a war story; the "I caused this" beat is what
  gives the answer credibility.
- End on the systemic change, not the immediate fix. "I bumped the
  memory limit" is a patch; "I made restart count a first-class SLI"
  is a thesis.
- If asked "what would you do differently next time?" you already have
  the answer — it's the Learning section, restated as a future habit.
