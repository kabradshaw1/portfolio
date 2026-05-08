# CI/CD Lessons from a Polyglot Microservices Portfolio

## Feed Post

My CI/CD pipeline started as "run everything."

That was simple, but it eventually turned every small change into too much
waiting.

The portfolio now runs quality gates across Go, Java, Python, Next.js,
Kubernetes, Docker, security checks, and smoke tests, while skipping work that
does not apply to the change.

The biggest lesson: pipeline optimization is not just speed. It is correctness.
Bad change detection can silently skip the image you needed to rebuild.

#CICD #DevOps #Golang #NextJS #Microservices

## Article

This portfolio is intentionally polyglot.

It has Go ecommerce and AI services, Java Spring services, Python FastAPI AI
services, a Next.js frontend, Kubernetes manifests, Docker images, Grafana
dashboards, and smoke tests.

That made CI/CD one of the most important parts of the project. A portfolio that
claims production-grade engineering should not depend on manual confidence.
Every meaningful change needs quality gates.

## Why I Unified the Workflow

I started with separate GitHub Actions workflows for each stack.

That helped early on because each language had different checks. Python needed
Ruff, pytest, Bandit, and pip-audit. Java needed Checkstyle and tests. Go needed
golangci-lint, race-enabled tests, and migration checks. The frontend needed
ESLint, TypeScript, Next.js build, and npm audit.

Over time, separate workflows became harder to reason about.

As a solo developer, I rarely stopped between stages. The separate workflow
files created more maintenance overhead than decision value. I consolidated the
pipeline into one workflow that handles quality checks, image builds, QA
deploys, production deploys, and smoke tests.

That made the system easier for me to debug and easier for coding agents to
operate against. One workflow file. One place to inspect when a deploy fails.
One shared model of how code moves from PR to QA to production.

## The Quality Gates

The pipeline runs checks across the whole project:

- Python linting, formatting, tests, SAST, and dependency audit
- Java style checks, unit tests, and integration tests
- Go linting, race tests, and migration pipeline tests
- Frontend ESLint, TypeScript checks, Next.js build, and audit
- security checks for secrets, Dockerfiles, and CORS rules
- Kubernetes manifest validation
- Grafana dashboard sync
- compose smoke tests
- deployment smoke tests

The goal is not just to prove that code compiles. The goal is to catch the kinds
of mistakes that happen in a real service system: bad manifests, broken routing,
missing env vars, unsafe CORS, stale images, failed migrations, and frontend
build issues.

## When "Run Everything" Became Too Slow

The first version ran too much work on every change.

Adding a RAG evaluation service exposed the problem. Its dependency tree pulled
in 200+ transitive packages. Cold installs made CI slow enough that the feedback
loop was no longer acceptable.

The first optimization was virtualenv caching. Instead of reinstalling every
Python dependency on every run, the workflow caches the `.venv` directory keyed
on dependency file hashes. The eval test setup dropped from roughly 20 minutes
to roughly 20 seconds on cache hit. pip-audit dropped from roughly 20 minutes to
single-digit seconds.

The second optimization was conditional image builds. A one-line change to one
service should not rebuild every Go, Java, and Python image. Each image build
declares the paths that affect it, and unchanged images are skipped.

The third optimization was changing compose smoke tests to pull prebuilt images
instead of building from source. Those smoke tests exist to verify configuration
and service wiring, not to re-test source compilation.

## The Change Detection Bug

The most important CI/CD lesson came from a change detection bug.

The first path filter compared against `HEAD~1`. That missed rebuilds when a
fix landed in a multi-commit push because the diff only saw the last commit.

I widened it to `HEAD~5`. That fixed the missed rebuild, but created the
opposite problem. Once Go code appeared in the last five commits, docs-only and
frontend-only pushes could still trigger full Go test, lint, and image-build
matrices.

The final fix was to use the compare range GitHub already provides:

- `github.event.before` for pushes
- `github.event.pull_request.base.sha` for pull requests
- a fallback only for first pushes and manual dispatches

That shifted the pipeline from heuristic detection to event-accurate detection.

The lesson was bigger than speed: bad change detection can be a correctness bug.
It can either waste compute or skip the exact rebuild needed to deploy a fix.

## Kubernetes Jobs Added Another Trap

QA deploys also hit Kubernetes Job immutability.

The Go overlay includes migration Jobs. Once a Job exists, Kubernetes will not
patch its pod template. A normal `kubectl apply` failed when the image tag
changed.

The fix was to handle Jobs separately: apply the rest of the overlay, delete old
migration Jobs, create the new Jobs, and wait for completion.

That turned QA deploys from failing into a repeatable deployment step.

## What I Learned

CI/CD work is backend engineering.

The pipeline is part of the system's reliability story. It decides whether tests
run, whether images rebuild, whether migrations execute, whether environments
stay consistent, and whether a production deploy has evidence behind it.

The best version of the pipeline was not the fastest one at any cost. It was the
one that kept the correctness guarantees while removing wasted work.

That is the standard I want this project to show: production engineering is not
only service code. It is also the machinery that proves the service code can be
shipped safely.
