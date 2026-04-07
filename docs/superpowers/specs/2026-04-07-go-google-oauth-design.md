# Go Auth Service — Google OAuth Sign-In

**Date:** 2026-04-07
**Status:** Approved
**Stack:** Go (auth-service), Next.js frontend

## Summary

Add "Sign in with Google" to the Go auth-service, mirroring the existing Java
task-service pattern: the frontend drives the OAuth redirect and receives an
authorization code; the backend exchanges the code for Google tokens, fetches
userinfo, upserts the user, and issues the service's own JWT access/refresh
pair.

This unblocks the `/go` sub-header redesign (dedicated login/register pages
with Google as a first-class option) and demonstrates polyglot OAuth
implementation across the portfolio's Java and Go stacks.

## Goals

- Google sign-in works on `/go/login` and `/go/register` in local dev and
  production.
- Backend code-exchange pattern matches Java's `/auth/google` endpoint so the
  two services share an identical flow shape.
- Existing email/password login path is untouched.
- Single Google OAuth client is reused across Java and Go stacks.

## Non-goals

- `/go` sub-header, cart icon, user dropdown (separate brainstorm).
- Removing the inline login form from `ecommerce/page.tsx` (handled by the
  header spec).
- Linking a Google identity to a pre-existing password account (account
  merging).
- Email verification flow.
- A "disconnect Google" UI.
- Google ID-token (GIS) flow as an alternative path.
- Any change to the Java task-service.

## Architecture

```
Browser                      Go auth-service             Google
  │                                 │                       │
  │ 1. click "Sign in w/ Google"    │                       │
  │────────────────────────────────────────────────────────▶│
  │ 2. redirect back w/ ?code=...   │                       │
  │◀───────────────────────────────────────────────────────  │
  │ 3. POST /auth/google            │                       │
  │    {code, redirectUri}          │                       │
  │────────────────────────────────▶│                       │
  │                                 │ 4. POST token exchange│
  │                                 │──────────────────────▶│
  │                                 │◀────────────────────  │
  │                                 │ 5. GET /userinfo      │
  │                                 │──────────────────────▶│
  │                                 │◀────────────────────  │
  │                                 │ 6. upsert user,       │
  │                                 │    issue JWT pair     │
  │ 7. {accessToken, refreshToken,  │                       │
  │     userId, email, name,        │                       │
  │     avatarUrl}                  │                       │
  │◀────────────────────────────────│                       │
```

The code exchange happens server-side because `client_secret` must not live in
the browser. The frontend only holds `client_id` and the redirect URI.

## Backend changes (`go/auth-service`)

### Migration: `002_google_oauth.sql`

```sql
ALTER TABLE users ALTER COLUMN password_hash DROP NOT NULL;
ALTER TABLE users ADD COLUMN avatar_url VARCHAR(500);
```

`password_hash` becomes nullable so Google-only users exist without a local
password. `avatar_url` matches the Java schema and feeds the upcoming header
dropdown.

### Config (env vars, read in `cmd/server/main.go`)

| Var | Default | Required |
|---|---|---|
| `GOOGLE_CLIENT_ID` | — | yes |
| `GOOGLE_CLIENT_SECRET` | — | yes |
| `GOOGLE_TOKEN_URL` | `https://oauth2.googleapis.com/token` | no |
| `GOOGLE_USERINFO_URL` | `https://www.googleapis.com/oauth2/v3/userinfo` | no |

Missing `GOOGLE_CLIENT_ID` or `GOOGLE_CLIENT_SECRET` is a startup fatal error,
consistent with how `JWT_SECRET` is handled today.

### New package: `internal/google`

```go
type Client struct { /* http.Client, token/userinfo URLs, client id/secret */ }

type UserInfo struct {
    Email   string
    Name    string
    Picture string
}

func (c *Client) ExchangeCode(ctx context.Context, code, redirectURI string) (*UserInfo, error)
```

`ExchangeCode` POSTs the code to the token endpoint, reads `access_token` from
the response, then GETs `/userinfo` with `Authorization: Bearer <token>`, and
returns the parsed `UserInfo`. Errors from either endpoint are wrapped so the
handler can distinguish "bad code" from "Google unreachable".

Isolated package so tests can drive it with `httptest.NewServer`.

### Model (`internal/model/user.go`, `internal/model/auth.go`)

- `User.AvatarURL *string` (nullable to match schema)
- New request: `GoogleLoginRequest { Code string; RedirectURI string }` with
  gin/validator tags
- `AuthResponse` gains `AvatarUrl string` (empty string if null in DB — the
  existing `UserID/Email/Name` fields stay as-is)

### Repository (`internal/repository/user.go`)

New method:

```go
func (r *UserRepository) UpsertGoogleUser(
    ctx context.Context, email, name, avatarURL string,
) (*model.User, error)
```

SQL:

```sql
INSERT INTO users (email, name, avatar_url, password_hash)
VALUES ($1, $2, $3, NULL)
ON CONFLICT (email) DO UPDATE
  SET name = EXCLUDED.name,
      avatar_url = EXCLUDED.avatar_url
RETURNING id, email, name, avatar_url, created_at
```

Leaves `password_hash` NULL on new Google users and does not touch it on
existing accounts (so a user who registered with a password and later signs in
with Google keeps their password).

### Service (`internal/service/auth.go`)

```go
func (s *AuthService) AuthenticateGoogleUser(
    ctx context.Context, email, name, avatarURL string,
) (*model.AuthResponse, error)
```

Calls `UpsertGoogleUser`, then issues tokens via the existing token-issuing
helper. Returned `AuthResponse` includes `avatarUrl`.

### Handler (`internal/handler/auth.go`)

```go
type GoogleClientInterface interface {
    ExchangeCode(ctx context.Context, code, redirectURI string) (*google.UserInfo, error)
}

// AuthHandler gains a googleClient field
func (h *AuthHandler) GoogleLogin(c *gin.Context)
```

Behavior:
1. Bind `GoogleLoginRequest`; on error → 400.
2. `googleClient.ExchangeCode(ctx, req.Code, req.RedirectURI)`; on error → 401
   with `"google authentication failed"`.
3. `svc.AuthenticateGoogleUser(ctx, info.Email, info.Name, info.Picture)`; on
   error → 500.
4. Return `AuthResponse` as JSON (same shape as email login).

Interface on the handler side enables handler tests without real HTTP.

### Routing (`cmd/server/main.go`)

```go
router.POST("/auth/google", authHandler.GoogleLogin)
```

No changes to other routes. Google client is constructed once in `main.go`
from env vars and passed into `NewAuthHandler`.

## Frontend changes (`frontend/`)

### `components/go/GoGoogleLoginButton.tsx`

New component. Mirrors `components/java/GoogleLoginButton.tsx`:

```ts
const redirectUri = `${window.location.origin}/go/login`;
const params = new URLSearchParams({
  client_id: GOOGLE_CLIENT_ID,   // shared env var, see below
  redirect_uri: redirectUri,
  response_type: "code",
  scope: "openid email profile",
  access_type: "offline",
  prompt: "consent",
});
window.location.href = `https://accounts.google.com/o/oauth2/v2/auth?${params}`;
```

### `app/go/login/page.tsx` (new)

Client component. Renders:
- Email/password form (reuses markup from the current inline form in
  `ecommerce/page.tsx`)
- `<GoGoogleLoginButton />`
- Error display

On mount (`useEffect`):
- Read `code` from `window.location.search`.
- If present: call `loginWithGoogle(code, ${window.location.origin}/go/login)`
  from `GoAuthProvider`, then `router.push("/go/ecommerce")`. Strip the code
  from the URL on success or failure so a refresh doesn't retry.

On email form submit:
- Call existing `login(email, password)`, then `router.push("/go/ecommerce")`.

### `app/go/register/page.tsx` (new)

Client component. Renders:
- Email/password/name form
- `<GoGoogleLoginButton />` (Google "register" and Google "login" are the same
  endpoint and same UX)
- Error display

On email form submit: existing `register(...)`, then push to `/go/ecommerce`.

### `components/go/GoAuthProvider.tsx`

- `GoAuthUser` gains `avatarUrl?: string`.
- `handleAuthResponse` reads `data.avatarUrl` if present and stores it in
  `localStorage` alongside name/email.
- New context method:

  ```ts
  loginWithGoogle: (code: string, redirectUri: string) => Promise<void>
  ```

  POSTs `{code, redirectUri}` to `${GO_AUTH_URL}/auth/google`, runs the same
  `handleAuthResponse` path as `login`, updates state.

### Env var

Reuse `NEXT_PUBLIC_GOOGLE_CLIENT_ID` across both Java and Go components. The
Go button imports it from `lib/go-auth.ts`, which re-exports it (or imports
directly from the shared `lib/auth.ts`, to be decided during implementation
based on existing structure).

### Not in this spec

- Header/dropdown changes
- Removing the inline login form from `ecommerce/page.tsx`

Both are owned by the subsequent `/go` sub-header spec.

## Deployment & config

### Local dev — `go/docker-compose.yml`

Extend the `auth-service` `environment` block:

```yaml
GOOGLE_CLIENT_ID: ${GOOGLE_CLIENT_ID}
GOOGLE_CLIENT_SECRET: ${GOOGLE_CLIENT_SECRET}
```

Create or extend `go/.env.example` with placeholder entries for both. Real
values live in `go/.env` (gitignored) and are copied from the existing Java
`.env` (same Google OAuth client).

### Kubernetes — `go/k8s/`

- Add `GOOGLE_CLIENT_ID` and `GOOGLE_CLIENT_SECRET` to the existing Go secret
  template (or create one if absent), following the pattern used for
  `JWT_SECRET` and `DATABASE_URL`.
- Wire the new keys into the `auth-service` Deployment via the existing
  `envFrom`/`env` mechanism. No new Secret resource unless there isn't one
  already.
- Secret *values* are the same as the Java stack's existing Google creds. The
  implementation plan will call out the kubectl apply step.

### Google Cloud Console (manual prerequisite)

On the existing OAuth 2.0 client (the one Java already uses), add two
authorized redirect URIs:

- `http://localhost:3000/go/login`
- `https://kylebradshaw.dev/go/login`

This is a click-through step in Google Cloud Console. The implementation plan
will list it as a checklist item before enabling the feature.

### CI/CD

No workflow changes. `go-ci.yml` does not need Google credentials — tests mock
the `google.Client` and the handler interface. Deploy paths already apply K8s
secrets separately from CI.

## Testing

### Backend

- `internal/google/client_test.go` — `httptest.NewServer` fakes Google.
  Cover:
  - Successful token + userinfo flow
  - Token endpoint returns 4xx
  - Userinfo endpoint returns 4xx
  - Malformed JSON from either endpoint
- `internal/service/auth_test.go` — extend existing file. Cover:
  - `AuthenticateGoogleUser` creates a new user with `password_hash` NULL and
    stores avatar
  - `AuthenticateGoogleUser` updates name and avatar on existing user, leaves
    `password_hash` untouched
  - Token issuance delegates to existing path (verify non-empty access/refresh
    in response)
- `internal/handler/auth_test.go` — extend existing file with a fake
  `GoogleClientInterface`. Cover:
  - Happy path returns `AuthResponse`
  - Bad request body → 400
  - Google client error → 401
  - Service error → 500

Repository-level tests for `UpsertGoogleUser` only if there are existing repo
tests to extend; otherwise service-level coverage is sufficient.

### Frontend

No new automated tests in this spec. The existing Playwright suite runs
against mocked endpoints on staging; the Google redirect dance can't be
meaningfully asserted without hitting real Google. Manual smoke test steps
(included in the implementation plan):

1. Click "Sign in with Google" on `/go/login`.
2. Complete Google consent.
3. Land on `/go/login?code=...`.
4. Verify automatic redirect to `/go/ecommerce`.
5. Verify `localStorage.go_user` contains `avatarUrl`.
6. Reload and verify the session persists.
7. Sign out and verify tokens cleared.

## Open questions

None. Prerequisites (redirect URI registration, secret copy) are captured in
the Deployment section and will surface as plan checklist items.

## Rollout

1. Merge migration + backend code together (migration is backward-compatible
   because `avatar_url` is nullable and `password_hash` drop-not-null doesn't
   affect existing rows).
2. Apply migration to dev Postgres, then K8s Postgres.
3. Update K8s secret with Google creds and roll `auth-service`.
4. Merge frontend changes.
5. Smoke test in production.
