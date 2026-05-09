# Building a Go AI Service That Streams Tool Use to Next.js

## Feed Post

I built a Go AI service that streams an agent loop into a Next.js frontend.

The frontend does not wait for a black-box answer. It receives typed
Server-Sent Events:

- tool_call
- tool_result
- tool_error
- final
- error

That lets the UI show what the agent is doing while it works across Go
microservices, an Ollama model, and a Python RAG pipeline.

The lesson: streaming AI UX is also backend architecture. The event contract is
the product.

#Golang #NextJS #AIEngineering #TypeScript #Microservices

## Article

The AI portion of my portfolio has a simple user experience: ask a shopping or
product knowledge question and receive a streamed answer.

The backend is less simple.

The Go AI service wraps a tool-calling agent loop around an ecommerce backend
and a Python RAG pipeline. The frontend is Next.js and TypeScript. The model is
Ollama running Qwen 2.5 14B. The RAG path crosses from Go to Python chat
services and Qdrant.

The interesting design problem was not just "call an LLM." It was how to make
the agent loop understandable to the user and debuggable to the engineer.

## Why I Used Server-Sent Events

The frontend talks to the AI service through an SSE stream.

Each response is not a single blob of text. It is a sequence of typed events:

- `tool_call`
- `tool_result`
- `tool_error`
- `final`
- `error`

The Next.js client parses the `event:` and `data:` lines, converts JSON payloads
into TypeScript union types, and yields them in order through an async
generator.

That contract is small, but it matters.

It lets the frontend render the agent's work as it happens. If the model calls
`search_products`, the user can see the tool call. If the tool returns display
data, the UI can render the result. If the tool fails, that failure is visible
without collapsing the entire interaction into a vague "something went wrong."

For this use case, SSE was a good fit because the stream is server-to-client.
The browser sends a prompt, then receives ordered events until the final answer.
I did not need full duplex WebSockets.

## The Agent Loop

The Go service runs a bounded ReAct-style loop:

1. Send conversation history and tool schemas to the model.
2. Inspect whether the model requested tool calls.
3. Dispatch tools against ecommerce APIs or the RAG pipeline.
4. Append tool results to the conversation.
5. Repeat until the model returns a final answer.

The loop is bounded by both step count and wall-clock timeout. In this project,
that means a maximum of 8 steps and a 90-second request window.

Those limits are important. Agent loops need a stop condition that is enforced
by the service, not merely requested in the prompt.

Tool errors become conversation context for the model instead of immediate hard
failures. That means the model can often explain a partial failure or choose a
different path. But the final SSE stream still exposes that a tool error
happened.

## Go as the Tool Gateway

The Go AI service acts as the tool gateway.

Some tools call ecommerce services directly: product search, cart operations,
and order lookups. Other tools cross into the Python RAG system: document
search, document Q&A, and collection listing.

That bridge let me keep the agent orchestration in Go while using Python where
the RAG ecosystem is strongest.

The boundary needed production concerns:

- circuit breakers around downstream HTTP calls
- request deadlines
- structured logs
- OpenTelemetry trace propagation
- typed display payloads for the frontend

The result is a stack where a single user question can move through Next.js,
the Go AI service, Ollama, Go ecommerce APIs, Python chat services, and Qdrant,
while still preserving enough context to debug it.

## The Frontend Contract Matters

On the Next.js side, the most important decision was to model the stream as
typed events instead of ad hoc strings.

The event union makes UI handling explicit:

- tool calls can render as progress
- tool results can render as structured evidence
- final answers can append to the assistant message
- errors can show a specific recovery state

This matters because AI interfaces often hide too much. If the user only sees a
spinner for 20 seconds, they cannot tell whether the system is searching
products, querying documents, retrying a slow service, or stuck.

Streaming tool activity gives the user a clearer mental model. It also gives
the engineer better evidence when something breaks.

## What I Learned

The biggest lesson was that AI UX and distributed systems design are connected.

The frontend event contract forced clarity in the backend. The backend agent
loop forced clarity in observability. The observability requirements forced
better boundaries around tools and downstream clients.

For hiring managers, this is the part I want the project to show: AI features
are not just prompt work. In a production-style application, they involve API
contracts, streaming protocols, failure handling, typed frontend state,
cross-service tracing, and operational limits.

The model may generate the answer, but the system around it determines whether
the feature is usable, debuggable, and reliable.
