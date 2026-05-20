# CI Quality Gate Design

## TL;DR

Add an explicit GitHub Actions `quality-gate` job so QA and production deploys
cannot start unless the existing deploy-relevant checks have completed
successfully. QA should use the same quality bar as production so failures show
up before code reaches `main`.

## Problem

The unified CI/CD workflow already has broad quality coverage: language linting
and tests, Java integration tests, Go migration validation, Kubernetes manifest
validation, compose smoke tests, mocked frontend E2E, and several security
checks.

The current dependency graph does not make all of those checks deploy blockers.
`build-images` waits for only a subset of jobs, and `deploy-qa` waits on
`build-images` plus mocked E2E. As a result, a push to `qa` can publish images
and start deployment while other deploy-relevant checks are still running or
have failed. That makes QA a weaker gate than intended and increases the chance
of production surprises.

## Goals

- Make QA a full rehearsal gate for production.
- Prevent image builds, QA deploys, and production deploys when any
  deploy-relevant quality job fails or is cancelled.
- Keep the deploy policy visible in one place.
- Preserve existing path-filter behavior inside jobs so unrelated changes do
  not run expensive steps unnecessarily.
- Keep this change focused on workflow orchestration only.

## Non-Goals

- Do not add missing Go services to the CI matrix in this spec.
- Do not refactor the large QA or production deploy shell blocks.
- Do not change Dependabot coverage.
- Do not add new scanners or test suites.
- Do not run full CI locally as part of implementation verification.

## Recommended Approach

Add a lightweight aggregate job named `quality-gate` in
`.github/workflows/ci.yml`. The job should use `if: always()` so it still runs
and reports a clear gate result when one of its dependencies fails, is
cancelled, or is intentionally skipped by event scope.

The gate should depend on every existing deploy-relevant check:

- `grafana-dashboard-sync`
- `python-lint`
- `python-tests`
- `java-lint`
- `java-unit-tests`
- `java-integration-tests`
- `go-lint`
- `go-tests`
- `buf-breaking`
- `go-migration-test`
- `frontend-checks`
- `e2e-mocked`
- `k8s-manifest-validation`
- `compose-smoke`
- `compose-smoke-go`
- `compose-smoke-java`
- `security-bandit`
- `security-pip-audit`
- `security-npm-audit`
- `security-gitleaks`
- `security-hadolint`
- `security-cors-check`

The job should not checkout the repository. It should inspect `needs.*.result`,
print a compact summary, and fail if any required dependency is `failure`,
`cancelled`, or an unexpected `skipped`.

`skipped` should be accepted only where it is intentional from workflow event
scope. Known cases are `buf-breaking`, which runs only for pull requests, and
`e2e-mocked`, which currently runs for pull requests and `qa` pushes but not
`main` pushes. Path-filtered jobs should still complete with a successful job
result even when their expensive internal steps are skipped, so they should not
require special handling.

If the implementation chooses to make mocked E2E a required production-push
gate too, it should first widen the existing `e2e-mocked` job condition to run
on `main` pushes. The first version does not need that broader behavior because
production is expected to receive code that already passed the full QA gate.

## Workflow Behavior

For pull requests to `qa`, `quality-gate` should validate the PR quality
surface. Image builds and deploys should remain disabled because `build-images`
already runs only on push.

For pushes to `qa`, `build-images` should depend on `quality-gate`. QA deploys
should continue to depend on image builds and mocked E2E, but because image
builds are behind the gate, QA deployment cannot start while security,
manifest, compose smoke, migration, or integration checks are failing or still
running.

For pushes to `main`, production deploys should also require the same gate.
The existing `changes` job can still drive selective restart behavior, but
production should not deploy unless the gate and image build both succeeded.

The current production deploy condition should be tightened. Instead of relying
on `always()` plus absence of `failure`, it should explicitly require:

- `github.ref == 'refs/heads/main'`
- `github.event_name == 'push'`
- `needs.quality-gate.result == 'success'`
- `needs.build-images.result == 'success'`

This avoids proceeding after a cancelled or unexpectedly skipped required gate.

## Error Handling

The gate should print a short table or line-per-job summary before deciding
whether to fail. The summary should include every dependency name and result so
the failing gate is immediately actionable from the log.

If the gate fails, the message should say that deploys are blocked because one
or more required checks did not succeed. It should not duplicate large logs
from underlying jobs.

## Testing And Verification

Implementation verification should be focused because this is an orchestration
change:

- Parse or validate the workflow YAML locally if a suitable tool is available.
- Inspect the final dependency graph in the workflow file:
  quality checks -> `quality-gate` -> `build-images` -> deploy -> smoke.
- Confirm `deploy-prod` cannot run unless both `quality-gate` and
  `build-images` succeeded.
- Do not run the full CI/CD pipeline locally.

The PR and subsequent branch pushes are the full-system verification:

- A PR to `qa` should show the `Quality Gate` check.
- A push to `qa` should show image builds waiting for `Quality Gate`.
- A production push should show production deploy waiting for both
  `Quality Gate` and image builds.

## Follow-Up Work

Separate specs or implementation tasks should cover these related but distinct
improvements:

- Add missing Go services to the CI lint/test matrix.
- Pin the CI `golangci-lint` version to match local preflight.
- Align local preflight coverage with CI for `rag-triage`.
- Expand Dependabot coverage for the remaining Go modules.
- Extract shared QA and production deploy shell logic into versioned scripts.
