# AWS Deployment & LLM Provider Abstraction

## Context

The portfolio currently runs on a Windows PC (Minikube + Ollama with RTX 3090) exposed via Cloudflare Tunnel. To demonstrate cloud deployment skills for job applications, we're adding AWS infrastructure managed by Terraform, deploying to EKS, and abstracting the LLM layer so the provider can be swapped via configuration. The AWS environment is demo-oriented — spun up for interviews and torn down after to minimize cost.

## Scope

Two workstreams:

1. **Terraform + EKS deployment** — provision AWS infrastructure and deploy all services to EKS with managed backing services
2. **LLM provider abstraction** — decouple from Ollama so any OpenAI-compatible or Anthropic API can be used

The current Minikube setup stays fully functional. Kustomize overlays handle the differences between environments.

---

## 1. Terraform Structure

### Bootstrap (`terraform/bootstrap/`)

Separate Terraform config with local state. Run once manually.

- **S3 bucket**: versioning enabled, AES-256 encryption, public access blocked
- **DynamoDB table**: `LockID` partition key for state locking
- State file committed to git (safe — only describes bucket/table metadata)

Files: `main.tf`, `variables.tf`, `outputs.tf`

### Main Config (`terraform/`)

Uses S3 backend created by bootstrap.

```
terraform/
├── bootstrap/
│   ├── main.tf
│   ├── variables.tf
│   └── outputs.tf
├── main.tf              # Provider, backend config
├── variables.tf         # Region, instance sizes, project name
├── outputs.tf           # Cluster endpoint, DB endpoints, ALB DNS
├── vpc.tf               # VPC, 2 AZs, public/private subnets, NAT gateway
├── eks.tf               # EKS cluster, managed node group, IAM
├── rds.tf               # PostgreSQL (shared by Java + Go)
├── elasticache.tf       # Redis
├── mq.tf                # Amazon MQ (RabbitMQ)
├── ecr.tf               # Container registries (one per service)
├── alb.tf               # AWS Load Balancer Controller
├── secrets.tf           # AWS Secrets Manager
├── terraform.tfvars.example  # Committed with safe defaults
└── .gitignore           # terraform.tfvars, *.tfstate (bootstrap state is exception)
```

---

## 2. AWS Resources

### Networking

- 1 VPC, 2 availability zones
- 2 public subnets (ALB, NAT gateway)
- 2 private subnets (EKS nodes, RDS, ElastiCache, Amazon MQ)
- 1 NAT gateway (single AZ — cost-conscious)
- Security groups: ALB accepts 80/443 from internet; nodes accept from ALB only; managed services accept from node SG only

### EKS Cluster

- Kubernetes 1.31
- Managed node group: 2x `t3.medium` (2 vCPU, 4GB each)
- Nodes in private subnets
- Public cluster endpoint (kubectl access from Mac)
- AWS Load Balancer Controller (replaces NGINX Ingress)
- Namespaces: `ai-services`, `java-tasks`, `go-ecommerce`, `monitoring`

### Managed Services

| Service | AWS Resource | Size | Notes |
|---------|-------------|------|-------|
| PostgreSQL | RDS | db.t3.micro, 20GB gp3 | Two databases: `taskdb`, `ecommercedb`. Free tier eligible. |
| Redis | ElastiCache | cache.t3.micro | Single node. Free tier eligible. |
| RabbitMQ | Amazon MQ | mq.t3.micro | Single-instance broker. |
| MongoDB | MongoDB Atlas | M0 (free tier) | 512MB, managed outside AWS via `mongodbatlas` Terraform provider. |
| Qdrant | In-cluster StatefulSet | Same as current | No managed option worth the cost. PVC for persistence. |

### Container Registry

- ECR: one repository per service
- CI pushes to both ECR (for AWS) and GHCR (for Minikube) so both deploy paths work

---

## 3. Kustomize Overlays

Restructure K8s manifests to support multiple environments:

```
k8s/
├── base/                          # Current manifests (shared across envs)
│   ├── ai-services/
│   │   ├── deployments/
│   │   ├── services/
│   │   ├── configmaps/
│   │   ├── hpa/
│   │   ├── ingress.yml
│   │   └── kustomization.yaml
│   └── monitoring/
│       └── ...
├── overlays/
│   ├── minikube/
│   │   ├── kustomization.yaml    # Patches: ExternalName ollama, nginx ingress annotations
│   │   └── patches/
│   └── aws/
│       ├── kustomization.yaml    # Patches: RDS endpoints, ALB annotations, remove DB deployments
│       └── patches/
java/k8s/
├── base/
└── overlays/
    ├── minikube/
    └── aws/                       # Remove postgres/redis/rabbitmq/mongo deployments, patch endpoints
go/k8s/
├── base/
└── overlays/
    ├── minikube/
    └── aws/
```

**AWS overlay patches:**
- Remove in-cluster database Deployments and Services (Postgres, Redis, RabbitMQ, MongoDB)
- Update ConfigMaps with managed service endpoints (RDS hostname, ElastiCache endpoint, etc.)
- Swap Ingress annotations from `nginx` to `alb` class with AWS-specific annotations
- Update image references from `ghcr.io` to ECR
- Add LLM provider env vars (`LLM_PROVIDER`, `LLM_API_KEY`, etc.)

**Minikube overlay patches:**
- ExternalName service for Ollama (`host.minikube.internal`)
- NGINX Ingress annotations
- GHCR image references

**deploy.sh update:**
- Takes environment argument: `./deploy.sh minikube` or `./deploy.sh aws`
- Uses `kubectl apply -k k8s/overlays/$ENV`

---

## 4. LLM Provider Abstraction

### Python Services

Create a provider abstraction as a shared `llm` package under `services/shared/llm/` imported by each service:

**Interface methods:**
- `embed(texts: list[str]) -> list[list[float]]` — for ingestion, chat, debug
- `generate(prompt, system, stream) -> AsyncIterator[str]` — for chat (streaming)
- `chat(messages, tools) -> dict` — for debug (tool calling)

**Implementations:**
- `OllamaProvider` — wraps current `httpx` calls to `/api/embed`, `/api/generate`, `/api/chat`
- `OpenAICompatibleProvider` — works with OpenAI, Groq, Together AI, OpenRouter (shared API format)
- `AnthropicProvider` — Claude via Anthropic SDK

**Configuration (env vars):**
- `LLM_PROVIDER`: `ollama` | `openai` | `anthropic`
- `LLM_BASE_URL`: provider endpoint (e.g., `https://api.groq.com/openai/v1`)
- `LLM_API_KEY`: API key
- `LLM_MODEL`: model name (e.g., `llama-3.1-70b-versatile`)
- `EMBEDDING_PROVIDER`: `ollama` | `openai`
- `EMBEDDING_BASE_URL`: endpoint
- `EMBEDDING_API_KEY`: API key
- `EMBEDDING_MODEL`: model name (e.g., `text-embedding-3-small`)

**Health checks:** Each provider implements its own connectivity check (replacing `/api/tags`).

### Go AI Service

Already has `Client` interface in `go/ai-service/internal/llm/client.go`. Add:
- `OpenAIClient` in `openai.go` — covers Groq, Together, OpenRouter
- `AnthropicClient` in `anthropic.go` — Claude
- Factory function in `factory.go` reads `LLM_PROVIDER` env var, returns the right client

### Default Provider for AWS Demo

- **Chat/completion:** Groq (Llama 3.1 70B) — free tier, fast, OpenAI-compatible
- **Embeddings:** OpenAI `text-embedding-3-small` — $0.02/1M tokens

### What Stays the Same

- Mock Ollama for CI — unchanged
- Local dev with Ollama — set `LLM_PROVIDER=ollama`
- Streaming, tool calling, response parsing — each implementation handles its own API format

---

## 5. CI/CD Changes

### New Workflow: `aws-deploy.yml`

- **Trigger:** `workflow_dispatch` (manual) — demo environment, not continuous
- **Steps:** build images → push to ECR → configure kubectl for EKS → `kubectl apply -k` overlays → run migration Jobs → smoke test
- **Auth:** IAM OIDC federation (no long-lived AWS keys in GitHub secrets)

### GitHub Secrets (new)

- `AWS_ACCOUNT_ID`, `AWS_REGION`
- IAM OIDC role ARN
- `LLM_API_KEY` (Groq/OpenAI)
- MongoDB Atlas connection string

### Existing Workflows

- `ci.yml`, `java-ci.yml`, `go-ci.yml` — unchanged (lint, test, security)
- Minikube deploy path — unchanged
- ECR push added alongside GHCR push in existing build steps

---

## 6. DNS & Tear-Down

### When AWS is up

- `api.kylebradshaw.dev` → CNAME to ALB hostname (set in Cloudflare)
- Cloudflare Tunnel to Windows PC paused

### When AWS is torn down

- `api.kylebradshaw.dev` → Cloudflare Tunnel to Windows PC (restore)
- DNS flip: manual in Cloudflare dashboard (or automate via Terraform Cloudflare provider)

### Tear-down workflow

```bash
cd terraform && terraform destroy    # ~5 min, removes everything
# Bootstrap S3 bucket stays (~$0.01/month)
# MongoDB Atlas free tier stays ($0)
# ECR images stay (~$0.10/month)
```

### Spin-up workflow

```bash
cd terraform && terraform apply      # ~15-20 min
# CI deploys services, or: kubectl apply -k k8s/overlays/aws
```

---

## 7. Cost Estimates

### When running (~per day)

| Resource | Cost |
|----------|------|
| EKS control plane | $3.30 |
| 2x t3.medium nodes | $2.00 |
| RDS db.t3.micro | $0.50 |
| ElastiCache cache.t3.micro | $0.50 |
| Amazon MQ mq.t3.micro | $0.80 |
| NAT Gateway | $1.10 |
| ALB | $0.80 |
| MongoDB Atlas M0 | Free |
| LLM API calls (Groq free tier) | Free |
| Embedding API calls | ~$0.01 |
| **Total** | **~$5-9/day** |

### When torn down

- S3 state bucket: ~$0.01/month
- ECR images: ~$0.10/month
- MongoDB Atlas: free
- **Effectively $0**

---

## 8. Files to Modify

### New files

- `terraform/bootstrap/main.tf`, `variables.tf`, `outputs.tf`
- `terraform/main.tf`, `variables.tf`, `outputs.tf`, `vpc.tf`, `eks.tf`, `rds.tf`, `elasticache.tf`, `mq.tf`, `ecr.tf`, `alb.tf`, `secrets.tf`
- `terraform/terraform.tfvars.example`
- Kustomize `kustomization.yaml` files in each base and overlay directory
- AWS overlay patch files
- LLM provider implementations (Python + Go)
- `.github/workflows/aws-deploy.yml`

### Modified files

- `k8s/deploy.sh` — add environment argument, use `kubectl apply -k`
- `services/ingestion/app/config.py` — add LLM provider env vars
- `services/ingestion/app/embedder.py` — use provider abstraction
- `services/chat/app/config.py` — add LLM provider env vars
- `services/chat/app/chain.py` — use provider abstraction
- `services/debug/app/config.py` — add LLM provider env vars
- `services/debug/app/agent.py` — use provider abstraction
- `services/debug/app/tools.py`, `indexer.py` — use embedding provider
- `go/ai-service/cmd/server/main.go` — factory function for LLM client
- K8s ConfigMaps — add LLM provider config
- CI workflows — add ECR push step

### Unchanged

- Java services (no LLM integration, connect to managed DBs via endpoint change only)
- Frontend (no changes, just points at `api.kylebradshaw.dev`)
- Mock Ollama (CI stays the same)

---

## 9. Verification

1. **Terraform:** `terraform plan` shows expected resources, `terraform apply` succeeds
2. **Kustomize:** `kubectl kustomize k8s/overlays/aws` renders correct manifests; `kubectl kustomize k8s/overlays/minikube` still matches current behavior
3. **LLM abstraction (local):** Set `LLM_PROVIDER=ollama`, verify all services work identically to current behavior
4. **LLM abstraction (API):** Set `LLM_PROVIDER=openai` with Groq credentials, verify chat streaming, debug tool calling, and embeddings work
5. **AWS deployment:** Services healthy, ingress routes work, managed DB connections succeed
6. **Tear-down:** `terraform destroy` cleans up everything, DNS flipped back, Minikube setup unaffected
7. **CI:** Existing workflows pass, new `aws-deploy.yml` deploys successfully
