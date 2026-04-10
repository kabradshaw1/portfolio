# AWS Infrastructure Page Design

## Overview

A new portfolio page at `/aws` showcasing the two deployment architectures for the portfolio site — the current Minikube-on-home-server production setup and the AWS-ready alternative with EKS and managed services. Includes a new homepage card and nav link.

## Homepage Changes

### New Card

Added as the fourth card in the homepage grid, after "Full Stack Java Developer":

- **Title:** "Infrastructure & Deployment"
- **Description:** "Production Kubernetes on a home server, AWS-ready with Terraform and EKS"
- **Content:** "Two deployment architectures for the same services — a cost-effective Minikube cluster with Cloudflare Tunnel serving production today, and a one-command AWS deployment with EKS, RDS, ElastiCache, and Amazon MQ."
- **Links to:** `/aws`

### Nav Link

Add "AWS" to `SiteHeader` nav, between "Go" and the Grafana link. Uses the same `navLinkClass` pattern with `/aws` prefix.

## Page: `/aws`

File: `frontend/src/app/aws/page.tsx` — server component, no layout file needed.

Uses the same `max-w-3xl` centered container and section patterns as `/ai`, `/go`, `/java`. Uses `MermaidDiagram` component for architecture diagrams.

### Section 1: Header + Intro

- `h1`: "Infrastructure & Deployment"
- Introductory paragraph explaining the two-deployment story: the portfolio runs on a home server today (zero hosting cost, Cloudflare Tunnel for security), but is also deployable to AWS with a single command. Frames this as pragmatism plus cloud readiness.

### Section 2: Current Production Architecture

- `h2`: "Current Production"
- Paragraph covering: Vercel serves the Next.js frontend. API traffic hits Cloudflare's edge, routes through a Cloudflare Tunnel to a Windows PC running a Minikube cluster. Three namespaces isolate services. Ollama runs on bare metal for RTX 3090 GPU access. No open ports, no public IP, no port forwarding.
- **Mermaid flowchart diagram** showing the request path:
  - Internet → Vercel (frontend) / Cloudflare Edge (API)
  - Cloudflare Tunnel → Windows PC → Minikube Ingress
  - Three namespace boxes: ai-services (ingestion, chat, debug, Qdrant), java-tasks (gateway, task, activity, notification, Postgres, MongoDB, Redis, RabbitMQ), go-ecommerce (auth, ecommerce, ai-agent)
  - Ollama on host (RTX 3090) connected to ai-services

### Section 3: AWS Deployment

- `h2`: "AWS Deployment"
- Paragraph covering: The same services deployed on EKS, with self-managed infrastructure replaced by AWS managed services. Terraform provisions the VPC, EKS cluster, RDS, ElastiCache, Amazon MQ, ECR, and ALB controller. Kustomize overlays swap service configs without changing the application code.
- **Mermaid flowchart diagram** showing:
  - Cloudflare DNS → ALB (AWS Load Balancer Controller)
  - ALB → EKS cluster → three namespace boxes
  - Managed services below: RDS (PostgreSQL), ElastiCache (Redis), Amazon MQ (RabbitMQ)
  - Groq API for LLM inference (replaces local Ollama)

### Section 4: Service Comparison Table

- `h2`: "What Changes Between Environments"
- HTML table with three columns: Concern, Home Server, AWS

| Concern | Home Server | AWS |
|---------|-------------|-----|
| Kubernetes | Minikube (Docker driver) | EKS (managed) |
| Ingress | NGINX Ingress Controller | AWS ALB (LB Controller) |
| PostgreSQL | Self-hosted in K8s | RDS |
| Redis | Self-hosted in K8s | ElastiCache |
| RabbitMQ | Self-hosted in K8s | Amazon MQ |
| MongoDB | Self-hosted in K8s | MongoDB Atlas (free tier) |
| LLM inference | Ollama (local RTX 3090) | Groq API |
| Embeddings | Ollama nomic-embed-text | OpenAI API |
| DNS / TLS | Cloudflare Tunnel | Cloudflare DNS + ACM |
| CI/CD deploy | SSH → kubectl (Tailscale) | GitHub OIDC → EKS |

### Section 5: Spin-Up Workflow

- `h2`: "One-Command Deployment"
- Paragraph: `./scripts/aws-up.sh` brings the full AWS stack up in ~15-20 minutes.
- Numbered list of what the script does:
  1. Bootstrap S3 state bucket + DynamoDB lock table (first run only)
  2. Terraform apply — provisions VPC, EKS, RDS, ElastiCache, Amazon MQ, ECR, ALB controller
  3. Configures kubectl for the new EKS cluster
  4. Deploys all services using Kustomize AWS overlays
  5. Prints ALB hostname for DNS configuration
- Brief mention of tear-down: `./scripts/aws-down.sh` destroys everything except S3 state and ECR images (~5 minutes).

### Section 6: Cost Breakdown

- `h2`: "Cost"
- Short paragraph: designed for spin-up/tear-down to minimize cost. Only runs during demos.

**Running (~$5-9/day):**

| Resource | Cost/day |
|----------|----------|
| EKS control plane | $3.30 |
| 2x t3.medium nodes | $2.00 |
| RDS db.t3.micro | $0.50 |
| ElastiCache cache.t3.micro | $0.50 |
| Amazon MQ mq.t3.micro | $0.80 |
| NAT Gateway | $1.10 |
| ALB | $0.80 |

**Torn down (~$0.11/month):**

| Resource | Cost/month |
|----------|------------|
| S3 state bucket | ~$0.01 |
| ECR images | ~$0.10 |
| MongoDB Atlas (free tier) | $0 |

- Closing sentence: this cost profile is why the home server remains the primary deployment.

## Files Changed

| File | Change |
|------|--------|
| `frontend/src/app/page.tsx` | Add fourth card linking to `/aws` |
| `frontend/src/components/SiteHeader.tsx` | Add "AWS" nav link |
| `frontend/src/app/aws/page.tsx` | New page (server component) |

## No Separate Tech Stack Section

Technologies surface naturally through diagrams, comparison table, and workflow steps. An explicit bullet list would duplicate them.

## Out of Scope

- No interactive demos (this is an explanatory page)
- No Terraform code display or syntax highlighting
- No changes to the actual AWS infrastructure or scripts
