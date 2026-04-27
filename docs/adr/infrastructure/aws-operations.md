# AWS Operations Runbook

## Overview

The AWS deployment is demo-oriented — spin up for interviews, tear down after to minimize cost. The current Minikube setup on the Windows PC remains the primary deployment.

## Prerequisites

- AWS CLI configured with credentials (`aws configure`)
- Terraform CLI installed (`brew install terraform`)
- kubectl installed
- GitHub secrets configured (see below)

## Spin Up (~15-20 min)

```bash
# From the repo root
./scripts/aws-up.sh
```

This runs:
1. **Bootstrap** — creates S3 state bucket + DynamoDB lock table (first time only)
2. **Terraform apply** — provisions VPC, EKS, RDS, ElastiCache, Amazon MQ, ECR, ALB controller
3. **kubectl config** — connects kubectl to the EKS cluster
4. **deploy.sh aws** — deploys all services using Kustomize AWS overlays

After completion, update DNS:
1. In Cloudflare, set `api.kylebradshaw.dev` as CNAME → ALB hostname (printed by the script)
2. Pause the Cloudflare Tunnel to the Windows PC
3. Wait ~2 min for DNS propagation

## Tear Down (~5 min)

```bash
./scripts/aws-down.sh
```

After completion, restore DNS:
1. In Cloudflare, restore `api.kylebradshaw.dev` → Cloudflare Tunnel
2. Unpause the Cloudflare Tunnel

## CI/CD Deploy (Alternative)

Instead of the scripts, you can deploy via GitHub Actions:
1. Go to Actions → "AWS Deploy" → Run workflow → Select "deploy"
2. This builds all images, pushes to ECR, and deploys to EKS

## Cost Estimates

### When Running (~per day)

| Resource | Cost |
|----------|------|
| EKS control plane | $3.30 |
| 2x t3.medium nodes | $2.00 |
| RDS db.t3.micro | $0.50 |
| ElastiCache cache.t3.micro | $0.50 |
| Amazon MQ mq.t3.micro | $0.80 |
| NAT Gateway | $1.10 |
| ALB | $0.80 |
| **Total** | **~$5-9/day** |

### When Torn Down

| Resource | Cost |
|----------|------|
| S3 state bucket | ~$0.01/month |
| ECR images | ~$0.10/month |
| MongoDB Atlas (free tier) | $0 |
| **Total** | **~$0.11/month** |

## GitHub Secrets Required

| Secret | Description |
|--------|-------------|
| `AWS_OIDC_ROLE_ARN` | IAM OIDC role ARN for GitHub Actions |
| `LLM_API_KEY` | Groq API key (or OpenAI/Anthropic) |
| `EMBEDDING_API_KEY` | OpenAI API key for embeddings |
| `AWS_DB_PASSWORD` | RDS PostgreSQL password |
| `AWS_MQ_PASSWORD` | Amazon MQ RabbitMQ password |
| `JWT_SECRET` | JWT signing secret (already exists) |

## Architecture

```
                    ┌─────────────┐
                    │  Cloudflare │
                    │    DNS      │
                    └──────┬──────┘
                           │
                    ┌──────▼──────┐
                    │     ALB     │
                    │  (AWS LB    │
                    │  Controller)│
                    └──────┬──────┘
                           │
              ┌────────────┼────────────┐
              │            │            │
     ┌────────▼──┐  ┌──────▼───┐  ┌────▼──────┐
     │ai-services│  │java-tasks│  │go-ecommerce│
     │ namespace │  │namespace │  │ namespace  │
     ├───────────┤  ├──────────┤  ├───────────┤
     │ingestion  │  │gateway   │  │auth       │
     │chat       │  │task      │  │ecommerce  │
     │debug      │  │activity  │  │ai-agent   │
     │qdrant     │  │notify    │  └─────┬─────┘
     └───────────┘  └────┬─────┘        │
                         │              │
              ┌──────────┼──────────────┤
              │          │              │
     ┌────────▼──┐ ┌─────▼────┐ ┌──────▼──────┐
     │   RDS     │ │ElastiCache│ │  Amazon MQ  │
     │PostgreSQL │ │  Redis    │ │  RabbitMQ   │
     └───────────┘ └──────────┘ └─────────────┘
```

## Troubleshooting

### Terraform state lock

If a previous `terraform apply` was interrupted:
```bash
cd terraform
terraform force-unlock <LOCK_ID>
```

### ALB not provisioning

Check the AWS Load Balancer Controller logs:
```bash
kubectl logs -n kube-system -l app.kubernetes.io/name=aws-load-balancer-controller
```

### Services not starting

Check pod events:
```bash
kubectl describe pod -n <namespace> <pod-name>
kubectl logs -n <namespace> <pod-name>
```

### Database connectivity

Verify the RDS endpoint is reachable from EKS nodes:
```bash
kubectl run pg-test --rm -it --image=postgres:17-alpine -- \
  psql "postgresql://postgres:<password>@<rds-endpoint>:5432/taskdb"
```
