import { MermaidDiagram } from "@/components/MermaidDiagram";

export function AiAssistantTab() {
  return (
    <div className="mt-8">
      <p className="mt-4 text-muted-foreground leading-relaxed">
        An LLM-powered shopping assistant that wraps a tool-calling agent
        loop around the ecommerce backend and a RAG knowledge base. Users
        ask natural language questions &mdash; the agent decides which tools
        to invoke, calls Go microservices or the Python RAG pipeline, and
        synthesizes a streamed response. Built in Go with Ollama (Qwen 2.5
        14B). The RAG bridge connects Go &rarr; Python chat service &rarr;
        Qdrant vector DB, with circuit breakers and OTel trace propagation
        across the stack boundary.
      </p>

      <h3 className="mt-10 text-xl font-semibold">Tool Catalog</h3>
      <p className="mt-4 text-muted-foreground leading-relaxed">
        The agent has access to twelve tools organized into four domains.
        Catalog tools are public; order, cart, and return tools require JWT
        authentication; knowledge base tools are public and hit the Python
        RAG pipeline via a circuit-breaker HTTP bridge with 30-second
        timeout. Checkout is deliberately excluded &mdash; the agent can
        advise but not transact.
      </p>
      <div className="mt-6">
        <MermaidDiagram
          chart={`flowchart LR
  AGENT((Agent<br/>Qwen 2.5 14B))
  subgraph Catalog ["Catalog (public)"]
    T1[search_products<br/>query + max_price]
    T2[get_product<br/>full details by ID]
    T3[check_inventory<br/>stock count]
  end
  subgraph Orders ["Orders (auth-scoped)"]
    T4[list_orders<br/>last 20 orders]
    T5[get_order<br/>single order detail]
    T6[summarize_orders<br/>LLM-generated summary]
  end
  subgraph CartReturns ["Cart & Returns (auth-scoped)"]
    T7[view_cart<br/>items + total]
    T8[add_to_cart<br/>product + quantity]
    T9[initiate_return<br/>order item + reason]
  end
  subgraph Knowledge ["Knowledge Base (public, RAG)"]
    T10[search_documents<br/>semantic search + sources]
    T11[ask_document<br/>natural-language Q&A]
    T12[list_collections<br/>vector store inventory]
  end
  AGENT --> Catalog
  AGENT --> Orders
  AGENT --> CartReturns
  AGENT --> Knowledge
  X[place_order<br/>deliberately excluded]:::disabled
  AGENT -.-x X
  classDef disabled stroke-dasharray: 5 5,opacity:0.5`}
        />
      </div>

      <h3 className="mt-10 text-xl font-semibold">Agent Loop</h3>
      <p className="mt-4 text-muted-foreground leading-relaxed">
        The agent runs a synchronous ReAct-style loop &mdash; call the LLM,
        dispatch any requested tools, feed results back into the
        conversation, and repeat until the LLM produces a final answer.
        Bounded by 8 steps and a 30-second wall-clock timeout. Tool errors
        become conversation context for the LLM to handle, not hard failures.
      </p>
      <div className="mt-6">
        <MermaidDiagram
          chart={`flowchart TD
  START([Receive user message])
  LLM[Call Ollama<br/>history + tool schemas]
  DECIDE{Tool calls<br/>in response?}
  DISPATCH[Dispatch tool to<br/>ecommerce API or RAG pipeline]
  APPEND[Append result to<br/>conversation history]
  GUARD{Max 8 steps<br/>or 90s?}
  REFUSAL{Refusal<br/>detected?}
  TAG[Tag outcome as refused]
  STREAM([Stream final answer<br/>via SSE])
  START --> LLM
  LLM --> DECIDE
  DECIDE -->|Yes| DISPATCH
  DISPATCH --> APPEND
  APPEND --> GUARD
  GUARD -->|No| LLM
  GUARD -->|Yes| STREAM
  DECIDE -->|No| REFUSAL
  REFUSAL -->|Yes| TAG
  TAG --> STREAM
  REFUSAL -->|No| STREAM`}
        />
      </div>

      <h3 className="mt-10 text-xl font-semibold">
        Request flow: Product search
      </h3>
      <p className="mt-4 text-muted-foreground leading-relaxed">
        A concrete example: the user asks &ldquo;find me a waterproof jacket
        under $150.&rdquo; The frontend streams Server-Sent Events from the
        AI service, which orchestrates between Ollama and the ecommerce API.
      </p>
      <div className="mt-6">
        <MermaidDiagram
          chart={`sequenceDiagram
  participant U as User
  participant FE as Frontend
  participant AI as AI Service
  participant OL as Ollama
  participant EC as Ecommerce API
  U->>FE: "find waterproof jackets under $150"
  FE->>AI: POST /chat (SSE stream, Bearer JWT)
  AI->>OL: Chat(messages, tool_schemas)
  OL-->>AI: tool_call: search_products
  AI-->>FE: SSE: tool_call {name, args}
  AI->>EC: GET /products?q=waterproof+jacket&max_price=15000
  EC-->>AI: [{name:"Storm Jacket", price:12999}]
  AI->>OL: Chat(messages + tool_result)
  OL-->>AI: final text
  AI-->>FE: SSE: final {text}
  FE-->>U: "I found 3 waterproof jackets under $150..."`}
        />
      </div>

      <h3 className="mt-10 text-xl font-semibold">
        Request flow: Product knowledge query
      </h3>
      <p className="mt-4 text-muted-foreground leading-relaxed">
        A cross-stack example: the user asks &ldquo;what&rsquo;s the warranty
        on the Storm Jacket?&rdquo; The Go AI service calls the Python RAG
        pipeline, which searches Qdrant for relevant document chunks and
        generates an answer with source citations.
      </p>
      <div className="mt-6">
        <MermaidDiagram
          chart={`sequenceDiagram
  participant U as User
  participant FE as Frontend
  participant AI as AI Service (Go)
  participant OL as Ollama
  participant PY as Python Chat Svc
  participant QD as Qdrant
  U->>FE: "what's the warranty on the Storm Jacket?"
  FE->>AI: POST /chat (SSE stream, Bearer JWT)
  AI->>OL: Chat(messages, tool_schemas)
  OL-->>AI: tool_call: ask_document
  AI-->>FE: SSE: tool_call {name, args}
  AI->>PY: POST /chat {question, collection}
  PY->>QD: vector search (embedded question)
  QD-->>PY: ranked chunks + scores
  PY->>OL: RAG prompt + retrieved context
  OL-->>PY: generated answer
  PY-->>AI: {answer, sources: [{file, page}]}
  AI->>OL: Chat(messages + tool_result)
  OL-->>AI: final text
  AI-->>FE: SSE: final {text}
  FE-->>U: answer with source citations`}
        />
      </div>
    </div>
  );
}
