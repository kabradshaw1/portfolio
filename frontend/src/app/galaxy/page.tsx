import { ExternalLink } from "lucide-react";
import Link from "next/link";

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
  "Prompt engineering",
  "Docker",
  "GitHub Actions",
  "AWS",
  "EKS",
  "Karpenter",
  "Graviton (ARM64)",
  "Terraform",
  "Aurora Serverless v2",
  "DocumentDB",
  "ElastiCache (Valkey)",
  "Amazon MQ",
  "ALB",
  "Route 53",
  "ACM",
  "IRSA",
  "External Secrets",
  "ghcr.io",
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

const datastoreMigration = [
  {
    from: "PostgreSQL (story, auth)",
    to: "Aurora PostgreSQL Serverless v2",
    note: "One cluster, two databases; autoscales from a low ACU floor.",
  },
  {
    from: "MongoDB (chat)",
    to: "Amazon DocumentDB",
    note: "CRUD-only chat usage; TLS required on the connection.",
  },
  {
    from: "Redis",
    to: "ElastiCache Serverless (Valkey)",
    note: "Pay-per-use, scales to a low idle floor.",
  },
  {
    from: "RabbitMQ",
    to: "Amazon MQ for RabbitMQ",
    note: "Single-instance broker for cost; cluster deployment for HA later.",
  },
  {
    from: "Static AWS keys in pods",
    to: "IRSA (scoped IAM roles)",
    note: "S3 image access without long-lived credentials.",
  },
  {
    from: "sealed-secrets",
    to: "Secrets Manager + External Secrets Operator",
    note: "Synced into native Kubernetes Secrets via IRSA.",
  },
  {
    from: "nginx ingress",
    to: "ALB + ACM + external-dns",
    note: "Route 53 record for api.galaxyvoyagers.com.",
  },
];

const phases = [
  {
    name: "Phase 1 — Foundation",
    status: "In progress",
    desc: "Remote state, VPC, EKS + Graviton baseline node group, Karpenter, and core cluster add-ons (LB Controller, external-dns, External Secrets Operator, metrics-server).",
  },
  {
    name: "Phase 2 — Data layer",
    status: "Upcoming",
    desc: "Aurora, DocumentDB, ElastiCache, and Amazon MQ; Secrets Manager entries synced into the cluster by External Secrets.",
  },
  {
    name: "Phase 3 — App layer",
    status: "Upcoming",
    desc: "IRSA roles, an EKS kustomize overlay pointing services at the managed datastores, and the ALB Ingress with ACM + external-dns.",
  },
  {
    name: "Phase 4 — Cutover",
    status: "Upcoming",
    desc: "Data migration, flip the api.galaxyvoyagers.com Route 53 record to the ALB, verify, then decommission the homelab stack.",
  },
];

const awsTargetDiagram = `flowchart TD
  VERCEL[Next.js on Vercel<br/>galaxyvoyagers.com]
  R53[Route 53 + ACM<br/>api.galaxyvoyagers.com]
  ALB[AWS ALB<br/>Load Balancer Controller]
  VERCEL -->|GraphQL API| R53
  R53 --> ALB

  subgraph EKS["EKS — Graviton baseline + Karpenter spot"]
    GW[gateway<br/>gqlgen :4000]
    STORY[story<br/>gRPC :50051]
    CHAT[chat<br/>gRPC :50052]
    STRIPE[stripe<br/>gRPC :50053 · webhook :4003]
    STORYGEN[storygen<br/>gRPC :50054]
    IMAGE[image<br/>gRPC :50055]
    AUTH[authv2<br/>gRPC :50056]
    ESO[External Secrets Operator]
  end

  ALB -->|/graphql| GW
  ALB -->|/webhook| STRIPE
  GW -->|gRPC| STORY
  GW -->|gRPC| CHAT
  GW -->|gRPC| STRIPE
  GW -->|gRPC| STORYGEN
  GW -->|gRPC| IMAGE
  GW -->|gRPC| AUTH

  subgraph Managed["AWS Managed Data"]
    AURORA[(Aurora PostgreSQL<br/>Serverless v2)]
    DOCDB[(Amazon DocumentDB)]
    EC[(ElastiCache<br/>Valkey)]
    MQ{{Amazon MQ<br/>RabbitMQ}}
  end

  STORY --> AURORA
  AUTH --> AURORA
  CHAT --> DOCDB
  STORY --> EC
  AUTH --> EC
  IMAGE --> EC
  GW --> MQ
  IMAGE --> MQ

  SM[Secrets Manager] --> ESO
  ESO -. syncs secrets .-> GW
  IMAGE -->|IRSA| S3[(S3 images bucket)]
  GHCR[ghcr.io] -. pulls images .-> EKS
  STORYGEN --> OPENAI[OpenAI]
  IMAGE --> OPENAI`;

export default function GalaxyPage() {
  return (
    <div className="mx-auto max-w-3xl px-6 py-12">
      <section>
        <p className="text-sm font-medium text-primary">Deployed project</p>
        <h1 className="mt-3 text-3xl font-bold">GalaxyVoyagers.com</h1>
        <div className="mt-4 inline-flex items-center gap-2 rounded-full border border-foreground/15 bg-primary/5 px-3 py-1 text-xs font-medium text-primary">
          Migrating to AWS · Phase 1 of 4
        </div>
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
        <p className="mt-4 text-sm text-muted-foreground leading-relaxed">
          The site is served today from a self-hosted k3s homelab and is
          actively migrating to a production AWS deployment — the target
          architecture is documented below.
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
          Image Generation: Layered Style Composition
        </h2>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          Image generation is slow and expensive, so it is decoupled from the
          request path. The image service accepts the job over gRPC and enqueues
          it on RabbitMQ. A consumer composes the final prompt, calls the image
          model, uploads the PNG to object storage, records metadata in
          PostgreSQL, and notifies the gateway with a signed URL through an HTTP
          callback. Failed jobs are retried up to three times before moving to a
          dead-letter queue.
        </p>
        <div className="mt-6">
          <MermaidDiagram
            chart={`flowchart LR
  GW[Go GraphQL Gateway]
  IMG[image-service<br/>ProcessImage gRPC]
  MQ{{RabbitMQ<br/>image queue}}
  W[image-consumer<br/>generation worker]
  COMP[Style composer<br/>YAML templates]
  AI[OpenAI gpt-image-1.5<br/>1024x1024]
  OBJ[(Object storage<br/>signed URLs)]
  PG[(PostgreSQL<br/>GenImageJob / GenFile)]
  GW -->|gRPC| IMG
  IMG -->|enqueue job| MQ
  MQ -->|consume| W
  W --> COMP
  COMP -->|composed prompt| AI
  AI -->|PNG bytes| W
  W -->|upload| OBJ
  W -->|metadata| PG
  W -->|HTTP callback + signed URL| GW
  MQ -. retry x3 then DLQ .-> MQ`}
          />
        </div>
        <p className="mt-6 text-muted-foreground leading-relaxed">
          The prompt is assembled by a style composer from a three-part template:
          a global art-direction base, a per-entity-type overlay that reframes
          the composition, and the user&apos;s subject text.
        </p>
        <div className="mt-4">
          <MermaidDiagram
            chart={`flowchart LR
  BASE["base<br/>global art direction"]
  OVL["overlay<br/>per entity type"]
  SUBJ["Subject: user_prompt"]
  OUT["{base}. {overlay}. Subject: {user_prompt}"]
  BASE --> OUT
  OVL --> OUT
  SUBJ --> OUT`}
          />
        </div>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          The entire visual identity lives in a YAML configmap loaded at startup,
          so the art direction can be tuned without a code change:
        </p>
        <pre className="mt-4 overflow-x-auto rounded-lg border border-border bg-muted/50 p-4 text-sm">
{`# services/image/style/templates.yaml
version: 1
format: "{base}. {overlay}. Subject: {user_prompt}"
base: |-
  Anime sci-fi superhero aesthetic. Cel-shaded with crisp ink lines and
  vibrant saturated palette. Dramatic rim lighting against deep space or
  neon-lit backdrops. High detail, dynamic composition, painterly shading.
entities:
  characters:
    overlay: "Hero portrait, three-quarter angle, expressive pose, costume detail emphasized."
  ships:
    overlay: "Exterior hero shot, 3/4 angle, sense of scale, engines glowing, motion-blurred star field."
  scenes:
    overlay: "Wide cinematic establishing shot, atmospheric perspective, characters small in frame."`}
        </pre>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          A request to illustrate a character therefore composes to a single
          prompt — the base style, the character overlay, then{" "}
          <code>Subject: &lt;the user&apos;s description&gt;</code> — giving every
          generated image a consistent, art-directed look across the platform.
        </p>
      </section>

      <section className="mt-12">
        <h2 className="text-2xl font-semibold">Prompt Engineering Takeaways</h2>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          Across both generation paths, the same prompt-engineering principles
          recur: ground the model in real domain data, keep that data relevant
          and cheap to fetch, compose prompts in layers, and match the execution
          model to the cost of the work.
        </p>
        <div className="mt-5 grid grid-cols-1 gap-3 sm:grid-cols-2">
          {promptPrinciples.map((item) => (
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
        <h2 className="text-2xl font-semibold">Production AWS Migration</h2>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          The migration moves GalaxyVoyagers from a self-hosted homelab onto a
          managed, autoscaling AWS deployment without rewriting application
          services. Managed EKS runs the existing manifests; Karpenter
          provisions spot capacity over a small Graviton (ARM64) on-demand
          baseline, consolidating and scaling to zero extra nodes when idle.
          ARM instances run the Go services at roughly 20% lower cost than x86.
        </p>
        <div className="mt-6">
          <MermaidDiagram chart={awsTargetDiagram} />
        </div>

        <h3 className="mt-8 text-lg font-medium">Self-hosted → AWS managed</h3>
        <div className="mt-4 overflow-x-auto">
          <table className="w-full text-sm text-muted-foreground">
            <thead>
              <tr className="border-b text-left">
                <th className="pb-2 pr-4 font-medium text-foreground">
                  Today (homelab)
                </th>
                <th className="pb-2 pr-4 font-medium text-foreground">
                  AWS managed target
                </th>
                <th className="pb-2 font-medium text-foreground">Notes</th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {datastoreMigration.map((row) => (
                <tr key={row.from}>
                  <td className="py-2 pr-4">{row.from}</td>
                  <td className="py-2 pr-4 text-foreground">{row.to}</td>
                  <td className="py-2">{row.note}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        <h3 className="mt-8 text-lg font-medium">Migration phases</h3>
        <div className="mt-4 space-y-3">
          {phases.map((phase) => (
            <div
              key={phase.name}
              className="rounded-lg border border-foreground/10 p-4"
            >
              <div className="flex items-center justify-between gap-3">
                <h4 className="text-sm font-semibold text-foreground">
                  {phase.name}
                </h4>
                <span
                  className={
                    phase.status === "In progress"
                      ? "rounded-full bg-primary/10 px-2.5 py-0.5 text-xs font-medium text-primary"
                      : "rounded-full bg-foreground/5 px-2.5 py-0.5 text-xs font-medium text-muted-foreground"
                  }
                >
                  {phase.status}
                </span>
              </div>
              <p className="mt-2 text-xs text-muted-foreground leading-relaxed">
                {phase.desc}
              </p>
            </div>
          ))}
        </div>

        <p className="mt-6 text-sm text-muted-foreground leading-relaxed">
          For a leaner, ephemeral take on the same AWS tools — spin up the
          portfolio&apos;s own services for a demo and tear them down after —
          see the{" "}
          <Link href="/aws" className="text-primary hover:underline">
            portfolio AWS deployment
          </Link>
          .
        </p>
      </section>

      <section className="mt-12">
        <div className="rounded-xl border border-foreground/10 bg-card p-6">
          <h2 className="text-lg font-semibold">Observability</h2>
          <p className="mt-3 text-sm text-muted-foreground leading-relaxed">
            GalaxyVoyagers ships the same Prometheus / Loki / Grafana
            observability stack used across this portfolio — Prometheus scrape
            annotations on pods, Loki log queries, and Grafana dashboards for the
            story services. The approach is documented in detail in the{" "}
            <Link
              href="/observability"
              className="text-primary hover:underline"
            >
              Observability section
            </Link>
            .
          </p>
        </div>
      </section>

      <section className="mt-12">
        <h2 className="text-2xl font-semibold">Engineering Focus</h2>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          The project highlights production-oriented backend design and
          infrastructure: a typed GraphQL boundary, protobuf service contracts,
          separate persistence models for relational worldbuilding data and
          document-style discussion data, async job handling for expensive
          generation work, and a Terraform-defined migration onto autoscaling
          AWS managed services (EKS with Karpenter and Graviton, Aurora
          Serverless v2, DocumentDB, IRSA, and External Secrets).
        </p>
      </section>
    </div>
  );
}
