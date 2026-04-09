import Link from "next/link";
import { MermaidDiagram } from "@/components/MermaidDiagram";

const architectureDiagram = `flowchart LR
  subgraph Ingestion["Document Ingestion"]
    direction LR
    A[PDF Upload] --> B[Parse\nPyMuPDF]
    B --> C[Chunk\nLangChain]
    C --> D[Embed\nnomic-embed-text]
    D --> E[(Qdrant)]
  end

  subgraph Query["Question Answering"]
    direction LR
    F[User Question] --> G[Embed\nnomic-embed-text]
    G --> H[Vector Search\nQdrant]
    H --> I[Build RAG Prompt]
    I --> J[Stream Response\nQwen 2.5 14B]
  end
`;

const debugArchitectureDiagram = `flowchart LR
  subgraph Index["Code Indexing"]
    direction LR
    A[Python Project] --> B[Walk Files]
    B --> C[Chunk\nLanguage.PYTHON]
    C --> D[Embed\nnomic-embed-text]
    D --> E[(Qdrant)]
  end

  subgraph Agent["Agent Loop"]
    direction LR
    F[Bug Description] --> G[Call LLM\nQwen 2.5 14B]
    G --> H{Tool Call?}
    H -->|Yes| I[Execute Tool]
    I --> G
    H -->|No| J[Stream Diagnosis]
  end
`;

export default function AISection() {
  return (
    <div className="min-h-screen bg-background text-foreground">
      <div className="mx-auto max-w-3xl px-6 py-12">
        {/* Header */}
        <h1 className="mt-8 text-3xl font-bold">AI / Gen AI Engineer</h1>

        {/* Bio */}
        <section className="mt-8">
          <p className="text-muted-foreground leading-relaxed">
            Building intelligent systems with retrieval-augmented generation and
            agentic architectures. This section demonstrates RAG pipelines,
            vector search, LLM orchestration, and tool-using agents — built with
            FastAPI, Qdrant, and Ollama, deployed on Kubernetes.
          </p>
          <p className="mt-4 text-sm text-muted-foreground leading-relaxed">
            Prometheus scrapes every AI service and streams metrics to a live{" "}
            <a
              href="https://api.kylebradshaw.dev/grafana/"
              target="_blank"
              rel="noopener noreferrer"
              className="underline hover:text-foreground transition-colors"
            >
              Grafana dashboard
            </a>
            .
          </p>
        </section>

        {/* Project Explanation */}
        <section className="mt-12">
          <h2 className="text-2xl font-semibold">Document Q&A Assistant</h2>
          <p className="mt-4 text-muted-foreground leading-relaxed">
            A full-stack Retrieval-Augmented Generation (RAG) application that
            lets users upload PDF documents and ask questions about their
            content. The system parses, chunks, and embeds documents into a
            vector database, then retrieves relevant context to generate
            accurate, grounded answers using a local LLM.
          </p>

          <h3 className="mt-6 text-lg font-medium">Tech Stack</h3>
          <ul className="mt-2 list-disc pl-6 text-muted-foreground space-y-1">
            <li>FastAPI microservices (ingestion + chat)</li>
            <li>Qdrant vector database</li>
            <li>Ollama with Mistral 7B (chat) and nomic-embed-text (embeddings)</li>
            <li>Next.js + TypeScript + shadcn/ui frontend</li>
            <li>Minikube Kubernetes deployment (production), Docker Compose (local dev)</li>
            <li>CI/CD with GitHub Actions, security scanning, E2E tests</li>
          </ul>
        </section>

        {/* Architecture Diagram */}
        <section className="mt-12">
          <h2 className="text-2xl font-semibold">How It Works</h2>
          <div className="mt-6 rounded-xl border border-foreground/10 bg-card p-6">
            <MermaidDiagram chart={architectureDiagram} />
          </div>
        </section>

        {/* Demo Link */}
        <section className="mt-12">
          <Link
            href="/ai/rag"
            className="inline-flex items-center gap-2 rounded-lg bg-primary px-6 py-3 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors"
          >
            Try the Demo &rarr;
          </Link>
        </section>

        {/* Debug Assistant Section */}
        <section className="mt-16">
          <h2 className="text-2xl font-semibold">Debug Assistant</h2>
          <p className="mt-4 text-muted-foreground leading-relaxed">
            An agentic debugging tool that indexes a Python codebase into a
            vector store and uses a ReAct-style agent loop to search the code,
            retrieve relevant context, and stream a grounded diagnosis of the
            described bug.
          </p>

          <h3 className="mt-6 text-lg font-medium">Tech Stack</h3>
          <ul className="mt-2 list-disc pl-6 text-muted-foreground space-y-1">
            <li>FastAPI debug service (index + agent endpoints)</li>
            <li>Qdrant vector database (per-project collections)</li>
            <li>Ollama with Qwen 2.5 14B (agent reasoning) and nomic-embed-text (embeddings)</li>
            <li>LangChain Python-aware text splitter</li>
            <li>SSE streaming for real-time agent event output</li>
            <li>Minikube Kubernetes deployment (production)</li>
          </ul>

          <h3 className="mt-6 text-lg font-medium">What It Demonstrates</h3>
          <ul className="mt-2 list-disc pl-6 text-muted-foreground space-y-1">
            <li>Agentic tool-use loop (ReAct pattern) with a local LLM</li>
            <li>Language-aware code chunking for higher-quality retrieval</li>
            <li>Named SSE events for streaming structured agent state</li>
            <li>Per-request Qdrant collections for isolated debug sessions</li>
          </ul>
        </section>

        {/* Debug Architecture Diagram */}
        <section className="mt-12">
          <h2 className="text-2xl font-semibold">How It Works</h2>
          <div className="mt-6 rounded-xl border border-foreground/10 bg-card p-6">
            <MermaidDiagram chart={debugArchitectureDiagram} />
          </div>
        </section>

        {/* Debug Demo Link */}
        <section className="mt-12">
          <Link
            href="/ai/debug"
            className="inline-flex items-center gap-2 rounded-lg bg-primary px-6 py-3 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors"
          >
            Try the Debug Demo &rarr;
          </Link>
        </section>
      </div>
    </div>
  );
}
