# QA Workflow Guide

Step-by-step reference for reviewing and shipping changes through the QA pipeline.

## Normal Flow: Agent Delivers a Feature

1. **Agent notifies you:** "PR to `qa` is ready, CI passed. Review at [PR link]"
2. **Review the PR diff** on GitHub — check the code changes make sense
3. **Merge the PR** on GitHub — this triggers the QA build + deploy pipeline
4. **Wait for deploy** — watch the Actions tab or wait for the pipeline to finish (~5-10 min)
5. **Inspect QA** at [qa.kylebradshaw.dev](https://qa.kylebradshaw.dev) — click through the affected pages, verify the change works as expected
6. **Ship to production** — tell Claude "ship it" and Claude will:
   - Merge `qa` into `main`
   - Push `main`
   - Watch the production CI/deploy pipeline
   - Debug any minor failures (lint, config)
   - Clean up the worktree and delete the feature branch
   - Report back when production is live

## Tweaking QA

If something needs adjustment after inspecting QA:

1. `git checkout qa && git pull`
2. For **frontend changes:** run `npm run dev` in `frontend/` with QA backend env vars:
   ```bash
   NEXT_PUBLIC_API_URL=https://qa-api.kylebradshaw.dev \
   NEXT_PUBLIC_GATEWAY_URL=https://qa-api.kylebradshaw.dev \
   NEXT_PUBLIC_INGESTION_API_URL=https://qa-api.kylebradshaw.dev/ingestion \
   NEXT_PUBLIC_CHAT_API_URL=https://qa-api.kylebradshaw.dev/chat \
   NEXT_PUBLIC_GO_AUTH_URL=https://qa-api.kylebradshaw.dev/go-auth \
   NEXT_PUBLIC_GO_ECOMMERCE_URL=https://qa-api.kylebradshaw.dev/go-api \
   NEXT_PUBLIC_AI_SERVICE_URL=https://qa-api.kylebradshaw.dev/ai-api \
   npm run dev
   ```
3. Tell Claude what to fix
4. **Ask Claude to push** — it will:
   - Commit the fix
   - Push to `qa`
   - Watch CI and debug minor failures (lint, formatting, config issues)
   - Stop and ask you before changing anything that affects app behavior
5. Wait for QA to redeploy, inspect again
6. When satisfied, tell Claude to ship it

## Environments

| Environment | Backend URL | Frontend URL |
|-------------|-------------|--------------|
| Production | api.kylebradshaw.dev | kylebradshaw.dev |
| QA | qa-api.kylebradshaw.dev | qa.kylebradshaw.dev |
| Local dev | localhost:8000 (via SSH tunnel) | localhost:3000 |

## What Claude Can Fix Autonomously

When watching CI on the `qa` branch:
- **Go ahead:** lint errors, formatting issues, type errors, import ordering, config typos
- **Stop and ask:** logic changes, API contract changes, data flow changes, new dependencies
