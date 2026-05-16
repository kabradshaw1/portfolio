# Observability API Cloudflare Access Setup

## Purpose

`observability-api.kylebradshaw.dev` is the authenticated API entry point for
local Observability MCP access. `grafana.kylebradshaw.dev` remains public for
portfolio dashboard viewing.

## Required Cloudflare Configuration

Create a Cloudflare Access application for:

```text
observability-api.kylebradshaw.dev
```

Policy:

- Allow only Kyle's account and the Observability MCP service token.
- Deny unauthenticated requests.
- Leave `grafana.kylebradshaw.dev` outside this Access policy so public
  dashboards continue to load.

## Service Token

Create a service token named:

```text
observability-mcp-local
```

Store the token only on the development machine:

```text
~/.codex/env/observability-mcp.env
```

Expected local file shape:

```bash
export OBS_GRAFANA_URL="https://observability-api.kylebradshaw.dev"
export OBS_GRAFANA_ACCESS_CLIENT_ID="<cloudflare-access-client-id>"
export OBS_GRAFANA_ACCESS_CLIENT_SECRET="<cloudflare-access-client-secret>"
```

Do not commit token values.

## Verification

Without service-token headers:

```bash
curl -I https://observability-api.kylebradshaw.dev/api/health
```

Expected: Cloudflare Access denies or redirects the request.

With service-token headers:

```bash
source ~/.codex/env/observability-mcp.env
curl -fsS \
  -H "CF-Access-Client-Id: ${OBS_GRAFANA_ACCESS_CLIENT_ID}" \
  -H "CF-Access-Client-Secret: ${OBS_GRAFANA_ACCESS_CLIENT_SECRET}" \
  "${OBS_GRAFANA_URL}/api/health"
```

Expected: Grafana health JSON.
