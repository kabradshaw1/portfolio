# CI/CD Pipeline Rebuild and QA Environment

- **Date:** 2026-04-13
- **Status:** Accepted
- **Supersedes:** [CI/CD Pipeline](cicd-pipeline.md)

## Context

The portfolio project had three separate GitHub Actions workflows (`ci.yml`, `java-ci.yml`, `go-ci.yml`) and a staging branch. For a solo developer, the staging branch added no value — it re-ran the same checks that already passed on the feature branch, with no code review or manual gate in between. The multiple workflow files also made it harder to understand the full CI/CD picture.

The project needed:
1. A pre-production QA environment for visual inspection before shipping
2. Automated agent workflow to reduce manual steps
3. A simpler, unified CI/CD pipeline

## Decision

### Unified Workflow

Consolidated all three workflow files into a single `ci.yml` with three triggers:
- **PR to `qa`:** runs all quality checks (lint, test, security, K8s validation)
- **Push to `qa`:** runs checks + builds all Docker images + deploys to QA namespaces + smoke tests
- **Push to `main`:** runs checks + builds images + deploys to production + smoke tests

The staging branch is retired. The `qa` branch replaces it.

### QA Environment

QA runs in the same Minikube cluster as production using separate namespaces (`ai-services-qa`, `java-tasks-qa`, `go-ecommerce-qa`). QA shares database instances with production but uses separate databases (`ecommercedb_qa`, `taskdb_qa`, `documents_qa` collection). This avoids duplicating infrastructure pods (PostgreSQL, MongoDB, Redis, RabbitMQ, Qdrant) saving ~1.3GB of memory on the single Minikube node.

QA is publicly accessible:
- Backend: `qa-api.kylebradshaw.dev` via Cloudflare Tunnel
- Frontend: `qa.kylebradshaw.dev` via Vercel branch domain

### Agent Workflow

Agents create feature branches, implement changes in git worktrees, push, and create PRs targeting `qa`. After CI passes, Kyle reviews the PR, merges it, inspects the QA deployment, and tells Claude to ship it to main. Claude handles the merge, push, CI watch, and worktree cleanup.

### Why Kyle Pushes Directly to Main

This is a solo developer project. By the time code reaches `main`, it has already passed all quality checks (on the PR), been deployed to QA, and been visually inspected. Branch protection requiring a PR approval would be a single person approving their own PR — ceremony with no value. Kyle pushes directly to main (via Claude when told to "ship it") after reviewing QA.

## Consequences

**Positive:**
- Single workflow file is easier to understand and maintain
- QA environment catches visual and integration issues before production
- Agent automation reduces manual steps from ~8 to ~2 (review PR, say "ship it")
- Shared database instances keep Minikube resource usage manageable

**Trade-offs:**
- All quality checks run on every trigger (no path-based skipping). Slower but simpler and catches cross-stack issues.
- QA and production share database instances. A runaway QA query could theoretically affect production, but this is a portfolio project with no real traffic.
- QA is publicly accessible, which is intentional — it demonstrates the CI/CD pipeline as part of the portfolio.
