# Security Audit & Comprehensive Hardening

**Date:** 2026-04-15
**Status:** Approved
**Approach:** Stack-by-stack sequential (Go -> Java -> Python -> Frontend -> Infrastructure)

## Summary

Comprehensive security audit identified ~40+ findings across all stacks. This spec covers fixes for every finding, organized by stack in dependency order. The goal is production-quality security posture across the entire portfolio.

## Decisions Made

- **Token storage:** Migrate from localStorage to httpOnly cookies (both Java and Go auth flows)
- **Secrets:** Remove from git, use templates + `.gitignore` (no external secret manager)
- **Java authorization:** Propagate JWT to downstream services (matching Go pattern), remove `X-User-Id` header trust
- **Prompt injection:** Structural defenses (delimiters + role separation), no output validation or guardrail model

---

## Section 1: Go Services Hardening

### 1.1 JWT Algorithm Validation (CRITICAL)
- Add HMAC signing method check in `go/ecommerce-service/internal/middleware/auth.go`
- Match the pattern in `go/auth-service/internal/service/auth.go:74`:
  ```go
  if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
      return nil, apperror.Unauthorized("INVALID_TOKEN", "unexpected signing method")
  }
  ```

### 1.2 Rate Limiting on Auth & Ecommerce (HIGH)
- Extend Redis-based rate limiter (from `go/ai-service/internal/guardrails/`) to auth-service and ecommerce-service
- Auth: `/login` and `/register` — 10 req/min per IP
- Ecommerce: all endpoints — 60 req/min per IP
- Same circuit-breaker-wrapped, fail-open pattern as AI service

### 1.3 Password Validation (MEDIUM)
- Increase minimum from 8 to 12 characters
- Add complexity requirements: uppercase, lowercase, digit, special character
- Custom validator or regex binding on `RegisterRequest`

### 1.4 CORS Verification (HIGH)
- Verify all three Go services only set CORS headers for whitelisted origins
- Ensure the `else` branch sets no CORS headers (currently correct — verify and add clarifying comment)

### 1.5 Token Revocation (MEDIUM)
- Add Redis-based token denylist in auth-service
- On logout, store token JTI (or hash) in Redis with TTL matching remaining token lifetime
- Auth middleware checks denylist before accepting a token
- Add `POST /auth/logout` endpoint

### 1.6 Secret Templates (CRITICAL)
- Replace `go/k8s/secrets/go-secrets.yml` with `go-secrets.yml.template` containing placeholder values
- Add `go-secrets.yml` to `.gitignore`
- Remove hardcoded JWT secret from `go/docker-compose.yml`, use `.env` reference

### 1.7 Security Headers (LOW)
- Add middleware to all three Go services:
  - `X-Content-Type-Options: nosniff`
  - `X-Frame-Options: DENY`

### 1.8 Error Disclosure (LOW)
- `auth-service/internal/google/client.go`: return generic error to caller, log full details with `slog.Error`

---

## Section 2: Java Services Hardening

### 2.1 JWT Propagation (CRITICAL)
- Gateway forwards original `Authorization: Bearer` header to downstream services
- Remove `X-User-Id` header fallback in `GraphQlInterceptor.java` (lines 22-28)
- Add JWT validation to task-service, activity-service, and notification-service
- Each service validates the token independently using shared `JWT_SECRET`
- Extract userId from validated token, not from header

### 2.2 Authorization Checks (CRITICAL)
- Task service: Add ownership/membership checks to `ProjectService.getProject()`, all `TaskService` methods, and `AnalyticsController` endpoints
- Activity service: Add Spring Security config, validate userId from JWT context before returning activities or accepting comments
- Notification service: Extract userId from JWT instead of trusting `X-User-Id` header

### 2.3 GraphQL Hardening (CRITICAL + HIGH)
- Disable GraphiQL in production: change `GRAPHIQL_ENABLED` default to `"false"` in K8s configmap
- Add `MaxQueryDepthInstrumentation` (max depth: 10)
- Add `MaxQueryComplexityInstrumentation` (max complexity: 100)

### 2.4 Input Validation (MEDIUM)
- Add `@Size` constraints to all DTOs: `CreateProjectRequest`, `CreateTaskRequest`, `CreateCommentRequest`
- Add `@Pattern` UUID validation on `@PathVariable` parameters in activity-service

### 2.5 Pod Security Contexts (HIGH)
- Add `securityContext` to all Java K8s deployments matching Go pattern:
  ```yaml
  securityContext:
    runAsNonRoot: true
    readOnlyRootFilesystem: true
    allowPrivilegeEscalation: false
    capabilities:
      drop: ["ALL"]
  ```

### 2.6 OAuth CSRF Protection (MEDIUM)
- Add `state` parameter validation to Google OAuth callback in `AuthController.java`

### 2.7 Error Handler Tightening (MEDIUM)
- `IllegalArgumentException` handler returns generic message instead of `ex.getMessage()`

### 2.8 Hardcoded Secret Fallbacks (CRITICAL)
- Remove default fallback values from `application.yml` for `JWT_SECRET`
- App should fail to start if `JWT_SECRET` not provided

---

## Section 3: Python Services Hardening

### 3.1 Authentication Middleware (CRITICAL)
- Add shared FastAPI dependency that validates Bearer JWT tokens
- Reuse same JWT secret and HMAC validation pattern from Go auth-service
- Extract userId from token, make available via `request.state.user_id`
- Protect all mutating and query endpoints across ingestion, chat, and debug services

### 3.2 Prompt Injection Defenses (HIGH)
- Chat service (`prompt.py`): Wrap user input in `<user_question>` tags, context in `<context>` tags
- Use Ollama system/user message role separation instead of single-string interpolation
- Debug service (`prompts.py`): Same delimiter approach for bug descriptions and error output
- Add system prompt instruction to ignore instructions embedded in user input or context

### 3.3 Rate Limiting (HIGH)
- Add `slowapi` to all three services
- Ingestion: 5 req/min on `/ingest`, 30 req/min on reads
- Chat: 20 req/min on `/chat`
- Debug: 5 req/min on `/index`, 10 req/min on `/debug`

### 3.4 Debug Service Path Restriction (HIGH)
- Add allowlist of indexable paths via `ALLOWED_PROJECT_PATHS` environment variable
- Validate `project_path` falls within an allowed directory
- Default to deny-all if not configured

### 3.5 File Upload Validation (MEDIUM)
- Add PDF magic byte validation (`%PDF-` header check) in addition to filename extension
- Replace deprecated `pypdf2` with `pypdf`

### 3.6 Input Sanitization (MEDIUM)
- Add max message count validation in debug agent loop
- Validate `document_id` and `collection_name` formats before Qdrant operations

### 3.7 Logging Sanitization (MEDIUM)
- Truncate user content in log messages
- Don't log full retrieved chunks or user prompts at INFO level

---

## Section 4: Frontend — httpOnly Cookie Migration

### 4.1 Go Auth Flow
- Backend (`go/auth-service`): Set `access_token` and `refresh_token` as `httpOnly`, `Secure`, `SameSite=Lax`, `Path=/` cookies on login/register/refresh
- Add `POST /auth/logout` that clears both cookies
- Frontend (`GoAuthProvider.tsx`, `go-auth.ts`): Remove localStorage token logic, use `credentials: "include"` on all requests

### 4.2 Java Auth Flow
- Backend (`java/task-service`): Same cookie-setting pattern on `/auth/login`, `/auth/register`, `/auth/google`, `/auth/refresh`
- Add logout endpoint that clears cookies
- Frontend (`AuthProvider.tsx`, `auth.ts`): Same localStorage removal, switch to `credentials: "include"`

### 4.3 Apollo Client Update
- Add `credentials: "include"` on all GraphQL requests
- Remove auth link that attaches `Authorization` header from localStorage

### 4.4 CORS Updates
- All backend services: `Access-Control-Allow-Credentials: true` with explicit origin whitelist
- Cookie `Domain` works across dev (`localhost`) and prod (`kylebradshaw.dev` / `api.kylebradshaw.dev`)

### 4.5 Cookie Configuration by Environment
- Dev: `Secure=false`, `SameSite=Lax`, no `Domain` attribute (defaults to exact host, which is correct for `localhost` — setting `Domain=localhost` is non-standard and inconsistent across browsers)
- Prod: `Secure=true`, `SameSite=Lax`, `Domain=.kylebradshaw.dev` (allows `api.kylebradshaw.dev` to set cookies readable by `kylebradshaw.dev`)
- Driven by environment variables

---

## Section 5: Infrastructure Hardening

### 5.1 NGINX Security Headers (MEDIUM)
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `X-XSS-Protection: 1; mode=block`
- `Referrer-Policy: strict-origin-when-cross-origin`

### 5.2 NGINX Rate Limiting (HIGH)
- Add `limit_req_zone` for expensive endpoints (ingestion, chat, debug)
- Generous limits — demonstrate the pattern, not lock down a portfolio demo

### 5.3 K8s NetworkPolicies (MEDIUM)
- Default-deny ingress in each namespace
- Allow ingress from ingress controller and necessary inter-service traffic
- One NetworkPolicy per namespace: `ai-services`, `java-tasks`, `go-ecommerce`

### 5.4 Secret Cleanup (CRITICAL)
- Add to `.gitignore`: `go/k8s/secrets/go-secrets.yml`, `.env.local`, `go/.env`
- Create template files with placeholder values
- Remove actual secret files from git tracking (not from disk)

### 5.5 Docker Compose Cleanup (LOW)
- Bind exposed dev ports to `127.0.0.1` only
- Move hardcoded secrets to `.env` file references

### 5.6 CORS Wildcard Headers (MEDIUM)
- Replace `allow_headers=["*"]` in Python services with explicit list: `["Content-Type", "Authorization"]`

### 5.7 Grafana Anonymous Access (LOW)
- Leave enabled with comment noting it's dev-only (Docker Compose is local dev environment)

---

## Out of Scope

- External secret management (Sealed Secrets, AWS Secrets Manager)
- LLM guardrail model for prompt injection
- Output validation on LLM responses
- Secret rotation (Google OAuth creds in git history)
- mTLS between K8s services
- OWASP Dependency-Check for Java (existing CI scanning is sufficient)
- Redis encryption at rest
