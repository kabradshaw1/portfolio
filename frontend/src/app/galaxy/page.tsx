import { ExternalLink } from "lucide-react";

import { MermaidDiagram } from "@/components/MermaidDiagram";

const stack = [
  "Next.js App Router",
  "React",
  "TypeScript",
  "Apollo Client",
  "Go",
  "gqlgen",
  "GraphQL subscriptions",
  "gRPC + Protobuf",
  "PostgreSQL",
  "MongoDB",
  "Redis",
  "RabbitMQ",
  "OpenAI",
  "Image generation",
  "Docker",
  "GitHub Actions",
  "Kubernetes",
  "Cloudflare Tunnel",
];

const graphDomains = [
  {
    title: "Connected worldbuilding data",
    desc: "Stories connect to scenes, characters, locations, organizations, conflicts, roles, ships, generated images, and discussion content.",
  },
  {
    title: "One declarative UI query",
    desc: "The frontend can ask for the nested shape a screen needs instead of fetching a story, then scenes, then related entities, then media through chained browser calls.",
  },
  {
    title: "Gateway-owned composition",
    desc: "The Go GraphQL gateway resolves fields across backend services and datastores while keeping the browser API explicit and stable.",
  },
  {
    title: "Subscriptions fit live generation",
    desc: "GraphQL subscriptions support streaming story suggestions and async creation flows without adding a second frontend API model.",
  },
];

const promptPrinciples = [
  {
    title: "Context injection over raw passthrough",
    desc: "Generation never sends only the user's text to the model. The service assembles a prompt from related worldbuilding entities so output stays consistent with the established universe.",
  },
  {
    title: "Relevance-scoped context",
    desc: "Injected entities are fetched cache-first (Redis, 10-minute TTL) with a Postgres fallback, and scene characters are filtered to only the organizations involved in that scene — focused prompts, not context dumps.",
  },
  {
    title: "Layered, compositional prompts",
    desc: "Images compose a global art-direction base, a per-entity-type overlay, and the user's subject. Text layers a task instruction, the writer's seed text, and a structured context block.",
  },
  {
    title: "Execution model matched to cost",
    desc: "Cheap, latency-sensitive text streams back token-by-token over gRPC. Expensive image jobs are queued through RabbitMQ with retries and a dead-letter queue.",
  },
];

export default function GalaxyPage() {
  return (
    <div className="mx-auto max-w-3xl px-6 py-12">
      <section>
        <p className="text-sm font-medium text-primary">Deployed project</p>
        <h1 className="mt-3 text-3xl font-bold">GalaxyVoyagers.com</h1>
        <p className="mt-6 text-muted-foreground leading-relaxed">
          GalaxyVoyagers is a collaborative sci-fi worldbuilding platform for
          building stories, scenes, characters, organizations, locations, ships,
          conflicts, and supporting media. It is a separate production
          deployment that demonstrates how I design a full-stack application
          around a connected domain instead of treating each screen as an
          isolated CRUD form.
        </p>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          The live site uses a Next.js frontend backed by a Go GraphQL gateway.
          Behind that gateway, Go services communicate over gRPC, store domain
          data in PostgreSQL and MongoDB, use Redis for shared runtime state,
          and send async work through RabbitMQ for AI-assisted story and image
          generation flows.
        </p>
        <a
          href="https://galaxyvoyagers.com"
          target="_blank"
          rel="noopener noreferrer"
          className="mt-6 inline-flex items-center gap-2 rounded-lg bg-primary px-5 py-3 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
        >
          Open GalaxyVoyagers.com
          <ExternalLink className="size-4" aria-hidden="true" />
        </a>
      </section>

      <section className="mt-12">
        <h2 className="text-2xl font-semibold">Technology Stack</h2>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          The project is intentionally polyglot at the system boundary but
          conservative inside each service: TypeScript and Apollo Client in the
          browser, Go and gqlgen at the API gateway, protobuf-defined gRPC
          contracts between services, and proven datastores selected for the
          access pattern they serve.
        </p>
        <div className="mt-4 flex flex-wrap gap-2">
          {stack.map((tech) => (
            <span
              key={tech}
              className="rounded-full bg-primary/10 px-3 py-1 text-xs font-medium text-primary"
            >
              {tech}
            </span>
          ))}
        </div>
      </section>

      <section className="mt-12">
        <h2 className="text-2xl font-semibold">Architecture</h2>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          The browser talks to one GraphQL entry point for queries, mutations,
          and subscriptions. The gateway owns backend composition: it calls the
          story, chat, auth, image, story-generation, and Stripe services over
          gRPC, while async generation work moves through RabbitMQ and streams
          results back to the UI.
        </p>
        <div className="mt-6">
          <MermaidDiagram
            chart={`flowchart LR
  U[Browser]
  NEXT[Next.js App Router<br/>Apollo Client]
  GW[Go GraphQL Gateway<br/>gqlgen :4000]
  STORY[story-service<br/>gRPC :50051]
  CHAT[chat-service<br/>gRPC :50052]
  STRIPE[stripe-service<br/>gRPC :50053]
  STORYGEN[storygen-service<br/>gRPC :50054]
  IMAGE[image-service<br/>gRPC :50055]
  AUTH[authv2-service<br/>gRPC :50056]
  PG[(PostgreSQL<br/>world + auth data)]
  MONGO[(MongoDB<br/>chat/discussion data)]
  REDIS[(Redis<br/>shared runtime state)]
  MQ{{RabbitMQ<br/>async jobs}}
  AI[OpenAI<br/>story + image models]
  K8S[Kubernetes homelab<br/>Cloudflare Tunnel]
  U --> NEXT
  NEXT -->|GraphQL queries + mutations| GW
  NEXT -->|GraphQL subscriptions| GW
  K8S -. serves .-> NEXT
  K8S -. routes API .-> GW
  GW -->|gRPC| STORY
  GW -->|gRPC| CHAT
  GW -->|gRPC| AUTH
  GW -->|gRPC| IMAGE
  GW -->|gRPC| STORYGEN
  GW -->|gRPC| STRIPE
  STORY --> PG
  AUTH --> PG
  STRIPE --> PG
  CHAT --> MONGO
  STORY --> REDIS
  AUTH --> REDIS
  IMAGE --> REDIS
  GW --> MQ
  IMAGE --> MQ
  STORYGEN --> AI
  IMAGE --> AI`}
          />
        </div>
      </section>

      <section className="mt-12">
        <h2 className="text-2xl font-semibold">How the Generative AI Works</h2>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          GalaxyVoyagers has two generative capabilities with deliberately
          opposite execution models, and both are driven by prompt engineering
          rather than by passing raw user text to a model. Text generation runs
          synchronously and streams results token-by-token, so the writer sees
          prose appear immediately. Image generation is expensive, so it is
          decoupled onto an asynchronous queue with retries. In both cases the
          interesting work happens before the model call: the service assembles
          a structured prompt from the surrounding worldbuilding data.
        </p>
      </section>

      <section className="mt-12">
        <h2 className="text-2xl font-semibold">
          Text Generation: Context-Injected Prompting
        </h2>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          When a writer asks for a scene description, the request reaches the
          storygen service over gRPC. Before calling the model, storygen pulls
          the entities related to that scene — characters, locations, conflicts,
          organizations, and ship casualties — from a Redis cache, falling back
          to PostgreSQL on a miss. It folds them into a single context-injected
          prompt, then streams the model&apos;s output back token-by-token over a
          gRPC server stream.
        </p>
        <div className="mt-6">
          <MermaidDiagram
            chart={`flowchart LR
  U[Browser]
  GW[Go GraphQL Gateway]
  SG[storygen-service<br/>gRPC :50054]
  CACHE[(Redis cache<br/>10-min TTL)]
  PG[(PostgreSQL<br/>world data)]
  AI[OpenAI /responses<br/>streaming SSE]
  U -->|request scene text| GW
  GW -->|gRPC StreamScene| SG
  SG -->|fetch related entities| CACHE
  CACHE -. cache miss .-> PG
  PG -. hydrate .-> CACHE
  SG -->|context-injected prompt| AI
  AI -->|output_text.delta chunks| SG
  SG -->|gRPC stream| GW
  GW -->|tokens stream to UI| U`}
          />
        </div>
        <p className="mt-6 text-muted-foreground leading-relaxed">
          The prompt itself is built in layers. A fixed task instruction comes
          first, then the writer&apos;s seed text, then a context block populated
          with the related entities:
        </p>
        <div className="mt-4">
          <MermaidDiagram
            chart={`flowchart TB
  A["1 · Task instruction<br/>I need a description of this scene as an essay"]
  B["2 · Writer's seed text"]
  C["3 · For context: injected entities"]
  C1[Characters — filtered to in-scene organizations]
  C2[Locations · Conflicts · Organizations · Casualties]
  OUT[Assembled prompt → OpenAI /responses]
  A --> OUT
  B --> OUT
  C --> OUT
  C1 --> C
  C2 --> C`}
          />
        </div>
        <pre className="mt-4 overflow-x-auto rounded-lg border border-border bg-muted/50 p-4 text-sm">
{`// storygen assembles the scene prompt by injecting related entities (Go)
prompt := "I need a description of this scene in the form of an essay.\\n"
prompt += req.Text                          // the writer's seed text
prompt += "\\nFor context:\\n"
prompt += "The following characters are involved in this scene: " + characters
prompt += "The scene takes place at the following locations: "    + locations
prompt += "This scene is associated with the following conflicts: " + conflicts
prompt += "The following organizations are involved in this scene: " + orgs

// characters are filtered to ONLY the organizations involved in this scene,
// and every entity is fetched cache-first (Redis, 10-min TTL) -> Postgres.`}
        </pre>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          The result is a prompt grounded in the specific corner of the universe
          the writer is editing, which keeps generated prose consistent with
          established characters and places instead of inventing contradictions.
        </p>
      </section>

      <section className="mt-12">
        <h2 className="text-2xl font-semibold">
          Why GraphQL Was The Right Boundary
        </h2>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          GalaxyVoyagers is not a flat resource catalog. A useful screen often
          needs a nested view: a story, its ordered scenes, the characters and
          locations in each scene, related organizations and conflicts,
          generated images, and discussion context. GraphQL fits that shape
          because the UI can request the exact graph it needs in one operation.
        </p>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          With a REST-only browser API, that same screen would tend to become a
          chain of dependent requests: fetch the story, fetch scenes, fetch the
          entities attached to each scene, fetch media, then fetch comments. The
          GraphQL gateway moves that composition into the backend, where it can
          resolve nested fields through service calls and datastore access
          without forcing the browser to coordinate every step.
        </p>
        <div className="mt-5 grid grid-cols-1 gap-3 sm:grid-cols-2">
          {graphDomains.map((item) => (
            <div
              key={item.title}
              className="rounded-lg border border-foreground/10 p-4"
            >
              <h3 className="text-sm font-semibold">{item.title}</h3>
              <p className="mt-2 text-xs text-muted-foreground leading-relaxed">
                {item.desc}
              </p>
            </div>
          ))}
        </div>
      </section>

      <section className="mt-12">
        <h2 className="text-2xl font-semibold">Engineering Focus</h2>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          The project highlights production-oriented backend design: a typed
          GraphQL boundary, protobuf service contracts, separate persistence
          models for relational worldbuilding data and document-style discussion
          data, async job handling for expensive generation work, and deployment
          through containerized services on Kubernetes.
        </p>
      </section>
    </div>
  );
}
