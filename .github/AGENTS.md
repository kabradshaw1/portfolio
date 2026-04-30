# GitHub Actions And CI/CD

The unified workflow is `.github/workflows/ci.yml`.

## Trigger Matrix

- PR to `qa` - quality checks
- Push to `qa` - quality checks, image builds, QA deploy, QA smoke tests
- Push to `main` - quality checks, image builds, production deploy, production smoke tests

Quality checks include Python lint/tests, frontend type/build checks, Java
checkstyle/tests, Go lint/tests, security scans, Kubernetes validation, and
CORS guardrails.

## CI Log Handling

Identify the failed job first:

```bash
gh pr view <pr> --json statusCheckRollup
```

Then inspect only the failed job with a targeted filter:

```bash
gh run view <run-id> --job <job-id> --log \
  | rg -n "##\\[error\\]|FAIL|failed|Error|panic|Exception|required|unhealthy"
```

Only fetch broader logs after filtered output identifies the failing step or
proves insufficient. Prefer redirecting full logs to `/tmp` and searching
locally instead of streaming large logs into the conversation.

## Compose Smoke Realism

The `compose-smoke` job runs the Python AI stack through `docker-compose.yml`
with mocked Ollama. Python service configuration changes must update both
`docker-compose.yml` and the matching Kubernetes manifests under
`k8s/ai-services/`.

## Tailscale Auth Key

The free-plan Tailscale auth key expires every 90 days. Regenerate it in the
Tailscale admin console and update the `TAILSCALE_AUTHKEY` GitHub repo secret.
