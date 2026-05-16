# Eval MCP Auth Integration Design

## Summary

Wire `go/eval-mcp-service` into the existing `go/auth-service` JWT flow so the
local MCP workflow can call the authenticated Python eval API without manually
copying bearer tokens. The MCP service will authenticate with the seeded smoke
user, cache token state outside experiment metadata, and attach access tokens to
requests sent to `services/eval`.

The Python eval API remains the backend for datasets, evaluation runs, scores,
and experiment evidence. Auth-service remains the issuer of JWTs accepted by
Python services through the shared JWT secret.

## Goals

- Let the eval MCP service authenticate through `go/auth-service`.
- Use the existing seeded smoke user instead of adding a new application user in
  this change.
- Keep browser auth behavior intact while adding a machine-friendly token
  response for service clients.
- Store reusable auth state locally with restrictive permissions.
- Retry a failed eval API call once after token refresh or re-login.
- Keep credentials and tokens out of the eval MCP experiment SQLite database.
- Make production documentation at `/ai/eval` backed by repeatable authenticated
  evaluation workflows.

## Non-Goals

- Do not create a new auth service or bypass auth-service JWT issuance.
- Do not add a new production Kubernetes deployment for the MCP service.
- Do not store smoke-user credentials in ConfigMaps, source code, or local eval
  experiment records.
- Do not change the Python eval service's authorization model beyond relying on
  bearer JWTs it already accepts.
- Do not add role-based authorization in this slice.

## Current State

`go/eval-mcp-service` already accepts `EVAL_API_TOKEN` and sends it as
`Authorization: Bearer <token>` through `internal/evalapi.Client`.

`services/eval` requires authentication on dataset and evaluation endpoints via
`shared.auth.create_auth_dependency(settings.jwt_secret)`. Bearer headers take
precedence over cookies.

`go/auth-service` has `POST /auth/login` and `POST /auth/refresh` and already
generates access and refresh tokens internally. Its handlers set auth cookies
and return user profile fields in JSON. The seeded smoke user is
`smoke@kylebradshaw.dev`; the password is managed outside the repo as
`SMOKE_GO_PASSWORD`.

## Architecture

Add an auth client inside `go/eval-mcp-service`:

- `internal/authclient`: typed HTTP client for auth login and refresh.
- `internal/tokenstore`: small local token cache abstraction.
- `internal/evalapi`: request path gains an auth provider or retry hook instead
  of accepting only a static token string.

At runtime, the MCP process obtains a valid access token before making eval API
requests. It uses cached token state when available, refreshes before expiry,
and falls back to login when refresh fails or no cache exists.

The eval API client remains responsible for eval endpoints. The auth client is
responsible only for auth-service endpoints. The workflow layer should not
manually construct auth headers.

## Auth-Service Contract

Extend auth request and response behavior without breaking browsers.

Request shape:

```json
{
  "email": "smoke@kylebradshaw.dev",
  "password": "<secret>",
  "includeTokens": true
}
```

`includeTokens` defaults to `false`. When false, login and refresh continue to
return the current browser-safe profile response and set cookies. When true, the
JSON response includes:

```json
{
  "userId": "...",
  "email": "smoke@kylebradshaw.dev",
  "name": "Smoke Test",
  "avatarUrl": "",
  "accessToken": "...",
  "refreshToken": "...",
  "expiresInSeconds": 900
}
```

`refreshToken` is included for `POST /auth/login` and for JSON-body
`POST /auth/refresh` requests that opt in to token output. The handler still
sets cookies in both modes so existing browser and smoke-test flows remain
compatible.

The token TTL value should come from the existing auth-service access token TTL
configuration rather than being guessed by the MCP service.

## Eval MCP Configuration

Add configuration keys:

- `AUTH_SERVICE_URL`: auth-service base URL. Local default:
  `http://localhost:8091/auth`; production API default can be documented as
  `https://api.kylebradshaw.dev/go-auth/auth`.
- `EVAL_MCP_AUTH_EMAIL`: auth email, expected to be
  `smoke@kylebradshaw.dev` for this workflow.
- `EVAL_MCP_AUTH_PASSWORD`: auth password, supplied from a secret such as
  `SMOKE_GO_PASSWORD`.
- `EVAL_MCP_TOKEN_CACHE_PATH`: token cache path, default
  `data/eval-mcp-auth.json`.
- `EVAL_MCP_TOKEN_REFRESH_SKEW`: refresh-ahead duration, default `60s`.
- `EVAL_API_TOKEN`: remains supported as a manual override for local debugging.

If `EVAL_API_TOKEN` is set, the MCP service uses it directly and does not call
auth-service. If it is not set, all auth-service configuration must be present
and valid.

## Token Cache

The token cache stores:

- access token
- access token expiry time
- refresh token
- auth email
- auth-service URL
- write timestamp

The file must be created with owner-only permissions (`0600`) and parent
directories should be created with non-world-writable permissions. The cache is
not part of the SQLite experiment database and must not be displayed in MCP tool
responses.

The cache should be ignored by Git through the existing data directory ignore
patterns or a targeted ignore entry if needed.

## Data Flow

1. MCP starts and loads config.
2. First eval API operation requests an access token from the auth provider.
3. Auth provider checks the token cache.
4. If the cached access token is still valid beyond the refresh skew, it is
   returned.
5. If the access token is near expiry and a refresh token exists, the provider
   calls `POST /auth/refresh` with `includeTokens: true`.
6. If refresh fails, the provider calls `POST /auth/login` with
   `includeTokens: true`.
7. The eval API client sends `Authorization: Bearer <accessToken>`.
8. If an eval API request returns `401`, the client invalidates cached access
   state, refreshes or logs in once, and retries the original request once.

Repeated `401` responses after retry should return a clear error that points to
auth-service credentials, JWT secret mismatch, or expired/invalid token state.

## Secret Handling

Local MCP registration should pass secrets through environment variables, not
command-line arguments that expose values in shell history or process listings.

For CI or production-like automation, credentials must come from GitHub Actions
secrets or Kubernetes SealedSecret-managed Secret data. `AUTH_SERVICE_URL` and
other non-secret routing values can live in config. The smoke password and any
persisted tokens must not be placed in ConfigMaps.

No live Kubernetes mutation is required to implement this local MCP workflow. If
a future task adds Kubernetes secret material, that task must use the
`ops-as-code` workflow and committed sealed-secret manifests.

## Error Handling

- Missing auth config produces a startup/config error unless `EVAL_API_TOKEN` is
  provided.
- Login `401` reports invalid smoke-user credentials without logging the
  password.
- Refresh `401` clears refresh state and attempts one login.
- Eval API `401` triggers one token recovery retry.
- Auth-service `429` or `5xx` errors are returned with status code and a short
  response excerpt.
- Token cache read failures are non-fatal unless the file exists but has unsafe
  permissions; unsafe permissions should fail closed with a corrective message.

## Testing

Go eval MCP tests:

- config defaults and validation for auth-service settings
- static `EVAL_API_TOKEN` override bypasses auth-service
- auth client login parses token responses
- auth client refresh parses token responses
- token cache writes `0600` files and rejects unsafe existing files
- auth provider refreshes before expiry
- auth provider falls back to login after refresh failure
- eval API client retries once after `401` and reuses the recovered token
- eval API client does not retry non-auth failures

Go auth-service tests:

- login without `includeTokens` preserves current JSON shape and cookie behavior
- login with `includeTokens` includes access token, refresh token, and expiry
- refresh with `includeTokens` includes rotated token data
- token fields are not emitted by default

Python eval tests:

- Existing bearer-token validation coverage remains sufficient unless
  implementation changes eval auth wiring.

Relevant verification before implementation commit:

```bash
make preflight-go
```

If Python eval auth wiring changes, also run:

```bash
make preflight-python
```

## Operational Notes

The production-facing `/ai/eval` page can document experiments run through this
authenticated MCP path because each run is still created in the Python eval API.
The MCP service is a local orchestration surface, not a separate source of truth.

The seeded smoke user is acceptable for this slice because the workflow already
depends on that test user. A later hardening pass can introduce a dedicated
service account or role claim if evaluation workflows need narrower permission
boundaries.
