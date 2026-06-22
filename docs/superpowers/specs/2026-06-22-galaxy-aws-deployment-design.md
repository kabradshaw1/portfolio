# Galaxy AWS Deployment — Frontend Documentation Design

**Date:** 2026-06-22
**Status:** Approved (design)
**Owner:** Kyle Bradshaw
**Scope:** Frontend / docs only (`gen_ai_engineer` repo). No backend, k8s, or Terraform changes.

## Goal

Update the portfolio's `/galaxy` route to document the **GalaxyVoyagers production
AWS migration** (homelab k3s → EKS), and lightly refresh the `/aws` route so the
two pages read as a deliberate, non-redundant **pair** rather than two
overlapping AWS write-ups. Surface that GalaxyVoyagers shares this portfolio's
observability approach via a link to the Observability section.

This documents AWS and Kubernetes expertise: a production-grade, autoscaling,
AWS-native Terraform migration on `/galaxy`, contrasted with the existing
ephemeral, cost-first EKS demo on `/aws`.

## Context

- **`/galaxy`** (`frontend/src/app/galaxy/page.tsx`) currently documents
  GalaxyVoyagers as a **self-hosted Kubernetes homelab behind Cloudflare
  Tunnel**. GalaxyVoyagers lives in a separate repo (`~/repos/story`).
- **`/aws`** (`frontend/src/app/aws/page.tsx`) documents **this portfolio repo's
  own services** (`ai-services`, `java-tasks`, `go-ecommerce`) with an
  **ephemeral spin-up/tear-down EKS demo**. It is factually accurate against the
  real `terraform/` in this repo (DynamoDB lock, `t3.medium` x86 nodes,
  `db/cache/mq.t3.micro`, Minikube on Debian 13 + Cloudflare Tunnel).
- **GalaxyVoyagers AWS migration** (`~/repos/story/docs/superpowers/specs/2026-06-22-aws-terraform-migration-design.md`)
  is an **approved, in-progress** migration. Phase 1 (foundation) is being
  implemented; apps still serve from the homelab. The target architecture:
  EKS + Karpenter (spot) over a Graviton/ARM64 baseline node group, Aurora
  PostgreSQL Serverless v2, Amazon DocumentDB, ElastiCache Serverless (Valkey),
  Amazon MQ, IRSA, AWS Secrets Manager + External Secrets Operator, AWS Load
  Balancer Controller (ALB) + ACM + external-dns (Route 53), ghcr.io retained,
  Next.js frontend stays on Vercel. Migration is phased (1 Foundation, 2 Data,
  3 App, 4 Cutover); the homelab is untouched until cutover.

### The non-redundant pairing

The overlap (both pages describe "EKS + ALB + managed datastores") is resolved
by making the two pages a deliberate contrast, each cross-linking the other:

| | `/aws` | `/galaxy` |
|---|---|---|
| App | This portfolio's own services | GalaxyVoyagers (separate app) |
| Posture | Ephemeral, cost-first; spin up for a demo, tear down | Production, always-on, autoscaling |
| Patterns | DynamoDB state lock, fixed `t3.medium` x86, t-class managed services | Karpenter spot + Graviton ARM, Aurora Serverless v2, IRSA, External Secrets, native S3 state locking |
| Story | "I can run anything on AWS cheaply" | "I can design and execute a production AWS migration" |

## Honesty constraint

The migration is **in progress** — apps still serve from the homelab. The
`/galaxy` page must frame AWS as the **target**, not a live deployment:

- A status pill: `Migrating to AWS · Phase 1 of 4`.
- One sentence: *"Currently served from a self-hosted k3s homelab; actively
  migrating to a production AWS deployment."*
- A phase-status strip marking Phase 1 as in progress, Phases 2–4 as upcoming.
- The "Open GalaxyVoyagers.com" button continues to point at the live
  (homelab-served) site.
- No present-tense claim that application workloads run on AWS.

## Design

### Page: `/galaxy` rewrite (`frontend/src/app/galaxy/page.tsx`)

Section order, top → bottom:

1. **Hero** — keep the "Deployed project" tag, title, the two intro paragraphs
   (lightly trimmed), and the live "Open GalaxyVoyagers.com" button. Add the
   status pill (`Migrating to AWS · Phase 1 of 4`) and the one-sentence homelab
   → AWS framing.

2. **Technology Stack** — keep app/runtime chips (Next.js, React, TypeScript,
   Apollo Client, Go, gqlgen, GraphQL subscriptions, gRPC + Protobuf,
   PostgreSQL, MongoDB, Redis, RabbitMQ, OpenAI, image generation, Docker,
   GitHub Actions). Replace deployment chips with the AWS target: **AWS, EKS,
   Karpenter, Graviton (ARM64), Terraform, Aurora Serverless v2, DocumentDB,
   ElastiCache (Valkey), Amazon MQ, ALB, Route 53, ACM, IRSA, External Secrets,
   ghcr.io.** Remove "Cloudflare Tunnel" and the bare "Kubernetes" chip
   (superseded by EKS).

3. **Architecture** — replace the homelab Mermaid diagram with a single
   **AWS-target** diagram:
   - Vercel frontend → Route 53 / ACM → ALB (AWS Load Balancer Controller)
   - ALB → EKS: GraphQL `gateway` + gRPC services (`story`, `chat`, `image`,
     `storygen`, `authv2`, `stripe`)
   - EKS compute: Graviton baseline managed node group + Karpenter spot
   - Datastores: Aurora PostgreSQL Serverless v2, Amazon DocumentDB, ElastiCache
     Valkey, Amazon MQ
   - IRSA → S3 images bucket; Secrets Manager + External Secrets Operator;
     images pulled from ghcr.io
   - Path routing preserved: `/graphql` → gateway:4000, `/webhook` → stripe:4003

4. **Production AWS Migration** *(new centerpiece)*:
   - **Why this shape** — managed EKS preserves the existing kustomize/gRPC
     topology; Karpenter on spot over a small Graviton on-demand baseline for
     burst-ready, cost-conscious scaling; ~20% ARM cost savings.
   - **"Self-hosted → AWS managed" table**:
     | Today (homelab) | AWS managed target |
     |---|---|
     | PostgreSQL (`story`, `auth`) | Aurora PostgreSQL Serverless v2 (one cluster, two DBs) |
     | MongoDB (chat) | Amazon DocumentDB |
     | Redis | ElastiCache Serverless (Valkey) |
     | RabbitMQ | Amazon MQ for RabbitMQ |
     | Static AWS keys in pods | IRSA (scoped IAM roles) |
     | sealed-secrets | AWS Secrets Manager + External Secrets Operator |
     | nginx ingress | ALB (LB Controller) + ACM + external-dns (Route 53) |
   - **Phase-status strip** — Phase 1 Foundation *(in progress)* · Phase 2 Data ·
     Phase 3 App · Phase 4 Cutover, with one line each.
   - **Cross-link** to `/aws`: *"For a leaner, ephemeral take on the same AWS
     tools, see the portfolio's spin-up/tear-down deployment."*

5. **Observability callout** — short bordered box linking to `/observability`.
   Wording kept to what is actually deployed (see Verify-items): *"GalaxyVoyagers
   exposes the same Prometheus-based metrics approach documented in the
   Observability section."* Upgrade wording toward full parity only if confirmed
   true at implementation time.

6. **Why GraphQL Was The Right Boundary** — keep as-is.

7. **Engineering Focus** — update the closing paragraph to add the production
   AWS migration and Terraform IaC as highlighted competencies.

### Page: `/aws` refresh (`frontend/src/app/aws/page.tsx`)

Light-touch positioning refresh. Keep all existing content (both diagrams,
comparison table, one-command deploy, cost tables). Changes:

1. **Intro** — add a sentence clarifying this page documents **this portfolio's
   own services** and an intentionally **ephemeral, cost-first** approach (spin
   up for a demo, tear down after).
2. **Contrast cross-link** — add a short bordered callout: *"This is the lean,
   ephemeral approach. For a production, always-on AWS migration — Karpenter spot
   autoscaling, Graviton, Aurora Serverless v2, IRSA, External Secrets — see the
   GalaxyVoyagers project →"* linking to `/galaxy`.
3. **Optional framing line** — position the page as "one of two AWS approaches
   in this portfolio," included only if it reads cleanly.

Deliberately **not changed**: DynamoDB-lock / `t3.medium` / t-class details
(accurate, and the intended contrast with galaxy's advanced patterns), the
diagrams, the cost model. No Terraform or k8s changes.

### Cross-cutting

- **Cross-links (both directions):** `/galaxy` → `/aws` and `/galaxy` →
  `/observability`; `/aws` → `/galaxy`. Use the existing `next/link` `Link`
  pattern.
- **Components & reuse:** no new heavy components. Reuse `MermaidDiagram`,
  `Link`, and existing Tailwind table/card/pill patterns. The phase strip and
  datastore-mapping table are plain JSX in the page file, matching the existing
  `stack` / `graphDomains` array style.
- **Nav:** `/galaxy` stays off the top nav (it's a homepage project card today).
  No `SiteHeader` change.

## Verification

- Update `frontend/e2e/mocked/galaxy-portfolio.spec.ts` to match the new
  headings/content.
- Run frontend CI gates before committing (`tsc` + eslint/lint).
- Confirm Mermaid diagrams render.

## Verify-items (resolve during implementation)

- **Observability parity** — inspect `~/repos/story/k8s/story/observability` and
  `~/repos/story/docs/observability` to confirm exactly what is deployed
  (Prometheus scrape annotations + OTel, vs. full Prometheus/Loki/Jaeger/Grafana).
  Keep the `/galaxy` callout wording to what is true; upgrade only if confirmed.
- **Service list** — confirm the gRPC service set and ports against the current
  `~/repos/story/k8s/story` manifests before finalizing the architecture diagram.

## Out of scope

- Any change to `~/repos/story` (the GalaxyVoyagers repo / migration itself).
- Backend, Kubernetes, or Terraform changes in this repo.
- Adding `/galaxy` to the top navigation.
- Deep rewrite of `/aws` content beyond positioning and cross-linking.
