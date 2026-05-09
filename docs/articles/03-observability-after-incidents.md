# Observability Got Real After the System Failed

## Feed Post

I had Prometheus, Loki, Jaeger, and Grafana deployed.

Then the system failed in ways those tools did not explain.

Four incidents changed how I thought about observability:

- a gRPC mTLS handshake hung silently
- a webhook 500 disappeared inside middleware
- an AI agent loop had almost no useful logs
- Postgres connections could not be attributed to services

The lesson: observability is not "tools installed." It is whether the next
failure can be diagnosed without guessing.

#Observability #Golang #Microservices #Kubernetes #Grafana

## Article

I had the observability stack deployed before I had observability.

Prometheus was scraping metrics. Loki was collecting logs. Jaeger was receiving
traces. Grafana had dashboards. On paper, the system looked observable.

Then real incidents exposed the gaps.

That was one of the better learning experiences in this project. The tools were
necessary, but they were not sufficient. Observability became meaningful only
after failures forced me to ask: can I explain what happened without guessing?

## Incident 1: The Silent gRPC Handshake

The checkout saga called the payment-service through gRPC with mTLS.

When the handshake broke, the user saw a generic order failure. The saga blocked
for 30 seconds. The logs did not make the failed dependency obvious. The metric
layer did not show which gRPC target was failing. The only way to understand the
issue was to inspect pods, test TLS manually, and compare recent deploys.

The fix was to instrument outbound gRPC calls.

I added a shared client interceptor that records request counts and durations by
target, method, and status code. I also added structured error logs on non-OK
results and enforced context deadlines around calls.

The lesson was direct: internal network calls are production dependencies. They
need the same visibility as public HTTP endpoints.

## Incident 2: The Webhook Error That Was Not Logged

A Stripe webhook failed with a server error, but Loki had no useful error log.

The problem was middleware behavior. The application converted structured
errors into JSON responses, but did not log every 5xx before responding. The
client saw a failure. The server technically handled it. The observability layer
missed it.

That is a dangerous failure mode because it creates false confidence. The system
is failing, but the logs imply nothing is wrong.

The fix was simple and important: every 5xx application error now emits a
structured log with code, message, status, and request ID before the response is
written.

The lesson: if an error reaches a user or external system, it should be visible
to the operator.

## Incident 3: The Black-Box AI Agent Loop

The Go AI service runs an agent loop. A user asks a question, the service calls
an LLM, the LLM may request tools, the service dispatches tools against
ecommerce APIs or the Python RAG pipeline, and the result streams back to the
frontend.

That can be three to eight LLM turns and multiple tool calls inside one request.

The first version had far too little structured logging. When something failed,
I could see the request start and end, but not the useful middle: which tool was
called, which provider responded, which guardrail fired, or which downstream
call failed.

I added structured logging across the HTTP handler, agent loop, LLM clients,
cache, guardrails, and tools. Logs use context-aware `slog` calls so the
OpenTelemetry trace ID is injected into every record. LLM clients emit spans,
which makes provider comparison visible in Jaeger.

The lesson: complex control flow needs internal landmarks. A trace that only
wraps the outside of an agent loop is not enough.

## Incident 4: Postgres Connections Looked Anonymous

During a Postgres incident, connection metrics showed pressure, but not
ownership.

Several services used the same database role. In `pg_stat_activity`, every
connection looked like the same application. That made it harder to identify
which service was leaking connections or driving load.

The fix was to include `application_name=<service-name>` in each service's
database URL and surface "Connections by Service" in Grafana.

That changed the question from "why is Postgres busy?" to "which service is
responsible?"

## What the System Looks Like Now

The stack now works as a connected investigation path:

1. A Grafana alert fires from a symptom-based metric.
2. The dashboard points to the affected namespace, service, or dependency.
3. Loki logs can be filtered by structured fields and trace ID.
4. Grafana derived fields link logs to Jaeger traces.
5. The trace shows the request path across HTTP, gRPC, Kafka, and downstream
   services.

That path matters more than any individual tool.

## What I Learned

Observability is not finished when the dashboards look good.

It is finished when the next failure has an obvious investigation path. That
requires instrumentation at the boundaries that actually fail:

- outbound gRPC calls
- async message consumers
- middleware error handling
- agent-loop internals
- database connections
- deploy annotations
- slow query visibility

The best observability work in this project came after incidents, not before
them. The incidents turned generic monitoring into specific questions the system
needed to answer.

That is the bar I now use: can this system explain its own failures?
