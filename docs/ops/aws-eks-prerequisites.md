# AWS / EKS deploy prerequisites

`.github/workflows/aws-deploy.yml` (`workflow_dispatch`) stands the portfolio up
on the Terraform-provisioned EKS cluster. Before the first real run, the items
below must exist. The workflow degrades gracefully when the managed-service
prerequisites are missing — it deploys what it can and skips/reports the rest —
but it cannot bring the full app up until they are provided.

## 1. Terraform

Apply `terraform/` first. It provisions EKS, ECR, RDS PostgreSQL
(`gen-ai-portfolio-postgres`), ElastiCache Redis (`gen-ai-portfolio-redis`), and
the Amazon MQ RabbitMQ broker (`gen-ai-portfolio-rabbitmq`). The deploy workflow
resolves these endpoints at runtime via the AWS CLI (it does **not** read
Terraform state), so the resource identifiers above must match.

## 2. GitHub Actions secrets

| Secret | Used for | Status |
| --- | --- | --- |
| `AWS_OIDC_ROLE_ARN` | OIDC role the workflow assumes | required |
| `AWS_DB_PASSWORD` | RDS **master** password (= Terraform `var.db_password`); used only by the RDS bootstrap Job | required |
| `AWS_TASK_DB_PASSWORD` | `taskuser` DB password — feeds every service DSN and the RDS bootstrap. **Must be URL-safe** (see note). | **TODO** |
| `JWT_SECRET` | Go + Java JWT signing | required |
| `LLM_API_KEY`, `EMBEDDING_API_KEY` | ai-services | required |
| `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET_PROD` | go payment-service `stripe-secrets` | present |
| `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET` | go-auth OAuth (falls back to `placeholder` when unset — auth still starts, login disabled) | optional |
| `AWS_MONGODB_ATLAS_URI` | java activity-service Mongo (Atlas) | **TODO** (follow-up) |
| `AWS_MQ_USERNAME`, `AWS_MQ_PASSWORD` | Amazon MQ creds (= Terraform `var.mq_username` / `var.mq_password`); build the RabbitMQ URLs | **TODO** (follow-up) |

> **URL-safe `AWS_TASK_DB_PASSWORD`.** The DSN-style Go services embed the
> password directly in a `postgres://taskuser:<pw>@...` URL via the
> `$(TASK_DB_PASSWORD)` dependent-env. Use only unreserved URL characters
> (`A–Z a–z 0–9 - . _ ~`); `@ : / ? # & %` would corrupt the DSN. The same
> value must match the `taskuser` role the RDS bootstrap Job creates.

## 3. TLS to RDS

RDS PostgreSQL 17's default parameter group forces SSL (`rds.force_ssl=1`). The
Go services connect with `sslmode=verify-full&sslrootcert=/etc/rds/global-bundle.pem`.
The deploy workflow downloads the RDS global CA bundle and mounts it (the
`rds-ca-bundle` ConfigMap) into every DB-touching Deployment and migration Job.

## What comes up now vs. what's blocked

**Healthy after a run (RDS + ElastiCache only):**

- `go-ecommerce`: `go-auth-service`, `go-product-service`, `go-order-projector`,
  `go-analytics-service` (Postgres → RDS, Redis → ElastiCache, Kafka in-cluster).
  All six migrations run against RDS.
- `ai-services`: `chat`, `debug`, `ingestion`, `qdrant` (RAG core).
- `monitoring`: all.

**Blocked until the follow-up (Amazon MQ + MongoDB Atlas):**

- `go-ecommerce`: `go-order-service`, `go-cart-service`, `go-payment-service`
  (RabbitMQ), `go-ai-service` (depends on order-service).
- `ai-services`: `eval`, `eval-worker` (RabbitMQ).
- `java-tasks`: **entire namespace deploy is skipped** — every Java service needs
  RabbitMQ, and activity-service needs Mongo.

## Follow-up: RabbitMQ (Amazon MQ) + MongoDB (Atlas)

Not just a config swap — Amazon MQ for RabbitMQ is **TLS-only on port 5671**
(`amqps://`), unlike the in-cluster `guest:guest` plaintext broker:

- **Go** services take a full `RABBITMQ_URL`, so an `amqps://` URL works once the
  managed-services overlay repoints it and `cart-service-mq` is created from CI.
- **Java** services configure RabbitMQ via discrete `RABBITMQ_HOST/USER/PASSWORD`
  with no SSL key — Spring AMQP needs `spring.rabbitmq.ssl.enabled=true` (port
  5671), which is an **application-config change**, not just a k8s patch. The same
  applies to the JDBC URL for RDS `verify-full`.
- **Mongo → Atlas** needs `AWS_MONGODB_ATLAS_URI` and an Atlas cluster reachable
  from the EKS VPC (peering / PrivateLink or IP allowlist).

When the broker, Atlas cluster, and the three secrets above exist, the workflow
auto-detects Amazon MQ and deploys `java-tasks` + `eval-rabbitmq` on the next run.
