# Observability MCP Private API Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move normal Observability MCP access from local ports/SSH to an authenticated Grafana API host while preserving public anonymous dashboard viewing.

**Architecture:** Add a separate `observability-api.kylebradshaw.dev` route to the existing Grafana service and protect it at the edge. Extend the local stdio MCP with a Grafana datasource client mode for Prometheus and Loki; keep direct backend clients as fallback and leave Jaeger proxy support as a follow-up verification item.

**Tech Stack:** Go, modelcontextprotocol Go SDK, Grafana datasource API, Kubernetes ingress, Cloudflare Access or equivalent edge policy, Codex MCP config.

---

## File Structure

- Modify `k8s/monitoring/ingress.yml`: add `observability-api.kylebradshaw.dev` host routing to Grafana.
- Create `docs/ops/observability-api-cloudflare-access.md`: manual Cloudflare Access/service-token setup record. Cloudflare policy cannot be safely inferred from repo state, so document exact required settings.
- Modify `go/observability-mcp-service/internal/config/config.go`: add Grafana mode config and validation.
- Modify `go/observability-mcp-service/internal/config/config_test.go`: cover direct mode, Grafana mode, and invalid mixed token config.
- Create `go/observability-mcp-service/internal/observability/grafana.go`: Grafana client using fixed datasource UIDs for Prometheus instant queries and Loki log queries.
- Create `go/observability-mcp-service/internal/observability/grafana_test.go`: request/response tests for Grafana client behavior.
- Modify `go/observability-mcp-service/cmd/observability-mcp/main.go`: choose direct clients or Grafana-backed clients based on config.
- Modify `go/observability-mcp-service/cmd/observability-mcp/main_test.go`: verify Grafana mode constructs the app without direct backend URLs.
- Modify `go/observability-mcp-service/README.md`: document private API mode, local env file, and SSH fallback.
- Modify `/Users/kylebradshaw/.codex/config.toml`: source `~/.codex/env/observability-mcp.env` for the observability MCP registration after the local env file exists.

## Task 1: Add Observability API Host To Monitoring Ingress

**Files:**
- Modify: `k8s/monitoring/ingress.yml`

- [ ] **Step 1: Inspect current ingress**

Run:

```bash
sed -n '1,120p' k8s/monitoring/ingress.yml
```

Expected: shows `grafana.kylebradshaw.dev` and `jaeger.kylebradshaw.dev`.

- [ ] **Step 2: Add the API hostname**

Update `k8s/monitoring/ingress.yml` so the `rules` list includes this second Grafana route:

```yaml
    - host: observability-api.kylebradshaw.dev
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: grafana
                port:
                  number: 3000
```

Keep the existing `grafana.kylebradshaw.dev` and `jaeger.kylebradshaw.dev` rules unchanged.

- [ ] **Step 3: Validate rendered monitoring manifests**

Run:

```bash
kubectl kustomize k8s/monitoring >/tmp/monitoring-rendered.yml
rg -n "observability-api.kylebradshaw.dev|grafana.kylebradshaw.dev|jaeger.kylebradshaw.dev" /tmp/monitoring-rendered.yml
```

Expected: all three hostnames are present.

- [ ] **Step 4: Commit ingress change**

Run:

```bash
git add k8s/monitoring/ingress.yml
git commit -m "infra: add observability api ingress host"
```

Expected: commit succeeds. Do not apply to the shared cluster until the edge access policy is ready.

## Task 2: Document Cloudflare Access Service Token Setup

**Files:**
- Create: `docs/ops/observability-api-cloudflare-access.md`

- [ ] **Step 1: Create the setup document**

Create `docs/ops/observability-api-cloudflare-access.md` with:

````markdown
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
````

- [ ] **Step 2: Commit the ops document**

Run:

```bash
git add docs/ops/observability-api-cloudflare-access.md
git commit -m "docs: document observability api access setup"
```

Expected: commit succeeds.

## Task 3: Add Grafana Mode Config

**Files:**
- Modify: `go/observability-mcp-service/internal/config/config.go`
- Modify: `go/observability-mcp-service/internal/config/config_test.go`

- [ ] **Step 1: Write failing config tests**

Add tests to `go/observability-mcp-service/internal/config/config_test.go`:

```go
func TestFromEnvGrafanaMode(t *testing.T) {
	t.Setenv("OBS_GRAFANA_URL", "https://observability-api.kylebradshaw.dev")
	t.Setenv("OBS_GRAFANA_TOKEN", "grafana-token")
	t.Setenv("OBS_GRAFANA_ACCESS_CLIENT_ID", "cf-id")
	t.Setenv("OBS_GRAFANA_ACCESS_CLIENT_SECRET", "cf-secret")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error = %v", err)
	}
	if !cfg.UseGrafanaGateway() {
		t.Fatal("expected Grafana gateway mode")
	}
	if cfg.GrafanaURL != "https://observability-api.kylebradshaw.dev" {
		t.Fatalf("GrafanaURL = %q", cfg.GrafanaURL)
	}
	if cfg.GrafanaToken != "grafana-token" {
		t.Fatal("GrafanaToken not loaded")
	}
	if cfg.GrafanaAccessClientID != "cf-id" || cfg.GrafanaAccessClientSecret != "cf-secret" {
		t.Fatalf("Cloudflare token config not loaded: %+v", cfg)
	}
}

func TestFromEnvRejectsPartialGrafanaAccessToken(t *testing.T) {
	t.Setenv("OBS_GRAFANA_URL", "https://observability-api.kylebradshaw.dev")
	t.Setenv("OBS_GRAFANA_ACCESS_CLIENT_ID", "cf-id")
	t.Setenv("OBS_GRAFANA_ACCESS_CLIENT_SECRET", "")

	if _, err := FromEnv(); err == nil {
		t.Fatal("expected partial Cloudflare access token error")
	}
}
```

- [ ] **Step 2: Run config tests and verify failure**

Run:

```bash
cd go/observability-mcp-service && go test ./internal/config
```

Expected: FAIL because `UseGrafanaGateway` and Grafana config fields do not exist.

- [ ] **Step 3: Implement config fields**

In `Config`, add:

```go
	GrafanaURL                string
	GrafanaToken              string
	GrafanaAccessClientID     string
	GrafanaAccessClientSecret string
```

In `FromEnv`, set:

```go
		GrafanaURL:                getenv("OBS_GRAFANA_URL", ""),
		GrafanaToken:              os.Getenv("OBS_GRAFANA_TOKEN"),
		GrafanaAccessClientID:     os.Getenv("OBS_GRAFANA_ACCESS_CLIENT_ID"),
		GrafanaAccessClientSecret: os.Getenv("OBS_GRAFANA_ACCESS_CLIENT_SECRET"),
```

Add validation:

```go
	if (cfg.GrafanaAccessClientID == "") != (cfg.GrafanaAccessClientSecret == "") {
		return Config{}, fmt.Errorf("OBS_GRAFANA_ACCESS_CLIENT_ID and OBS_GRAFANA_ACCESS_CLIENT_SECRET must be set together")
	}
```

Add method:

```go
func (c Config) UseGrafanaGateway() bool {
	return c.GrafanaURL != ""
}
```

- [ ] **Step 4: Run config tests and verify pass**

Run:

```bash
cd go/observability-mcp-service && go test ./internal/config
```

Expected: PASS.

- [ ] **Step 5: Commit config change**

Run:

```bash
git add go/observability-mcp-service/internal/config/config.go go/observability-mcp-service/internal/config/config_test.go
git commit -m "feat: add observability grafana gateway config"
```

Expected: commit succeeds.

## Task 4: Add Grafana Datasource Client

**Files:**
- Create: `go/observability-mcp-service/internal/observability/grafana.go`
- Create: `go/observability-mcp-service/internal/observability/grafana_test.go`

- [ ] **Step 1: Write failing Grafana client tests**

Create `go/observability-mcp-service/internal/observability/grafana_test.go` with tests that cover:

```go
package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGrafanaPrometheusQueryUsesDatasourceProxy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer grafana-token" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := r.Header.Get("CF-Access-Client-Id"); got != "cf-id" {
			t.Fatalf("CF access id = %q", got)
		}
		if r.URL.Path != "/api/datasources/proxy/uid/PBFA97CFB590B2093/api/v1/query" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("query") != "up" {
			t.Fatalf("query = %s", r.URL.Query().Get("query"))
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{"job":"prometheus"},"value":[1710000000,"1"]}]}}`))
	}))
	defer server.Close()

	client := NewGrafana(GrafanaConfig{
		BaseURL:            server.URL,
		Token:              "grafana-token",
		AccessClientID:     "cf-id",
		AccessClientSecret: "cf-secret",
	}, server.Client())

	got, err := client.Query(context.Background(), "up")
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(got) != 1 || got[0].Value != 1 {
		t.Fatalf("samples = %+v", got)
	}
}

func TestGrafanaLokiQueryUsesDatasourceProxy(t *testing.T) {
	start := time.Unix(1710000000, 0)
	end := start.Add(time.Minute)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/datasources/proxy/uid/loki/loki/api/v1/query_range" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("limit") != "3" {
			t.Fatalf("limit = %s", r.URL.Query().Get("limit"))
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"result":[{"stream":{"service":"go-order-service"},"values":[["1710000000000000000","{\"level\":\"ERROR\",\"msg\":\"boom\"}"]]}]}}`))
	}))
	defer server.Close()

	client := NewGrafana(GrafanaConfig{BaseURL: server.URL}, server.Client())
	got, truncated, err := client.QueryLogs(context.Background(), LogQuery{
		Service: "go-order-service",
		Pattern: "boom",
		Start:   start,
		End:     end,
		Limit:   2,
	})
	if err != nil {
		t.Fatalf("QueryLogs() error = %v", err)
	}
	if truncated {
		t.Fatal("did not expect truncated logs")
	}
	if len(got) != 1 || got[0].Labels["service"] != "go-order-service" {
		t.Fatalf("logs = %+v", got)
	}
}
```

- [ ] **Step 2: Run observability tests and verify failure**

Run:

```bash
cd go/observability-mcp-service && go test ./internal/observability
```

Expected: FAIL because `NewGrafana` and `GrafanaConfig` do not exist.

- [ ] **Step 3: Implement Grafana client**

Create `go/observability-mcp-service/internal/observability/grafana.go` with:

```go
package observability

import (
	"context"
	"net/http"
	"strings"
	"time"
)

const (
	grafanaPrometheusUID = "PBFA97CFB590B2093"
	grafanaLokiUID       = "loki"
)

type GrafanaConfig struct {
	BaseURL            string
	Token              string
	AccessClientID     string
	AccessClientSecret string
}

type GrafanaClient struct {
	prometheus *PrometheusClient
	loki       *LokiClient
	headers    map[string]string
}

func NewGrafana(cfg GrafanaConfig, httpClient *http.Client) *GrafanaClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	headers := map[string]string{}
	if cfg.Token != "" {
		headers["Authorization"] = "Bearer " + cfg.Token
	}
	if cfg.AccessClientID != "" {
		headers["CF-Access-Client-Id"] = cfg.AccessClientID
		headers["CF-Access-Client-Secret"] = cfg.AccessClientSecret
	}
	wrapped := &headerRoundTripper{base: httpClient.Transport, headers: headers}
	client := *httpClient
	client.Transport = wrapped
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	return &GrafanaClient{
		prometheus: NewPrometheus(baseURL+"/api/datasources/proxy/uid/"+grafanaPrometheusUID, &client),
		loki:       NewLoki(baseURL+"/api/datasources/proxy/uid/"+grafanaLokiUID, &client),
		headers:    headers,
	}
}

func (c *GrafanaClient) Query(ctx context.Context, query string) ([]MetricSample, error) {
	return c.prometheus.Query(ctx, query)
}

func (c *GrafanaClient) QueryRange(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]MetricSample, error) {
	return c.prometheus.QueryRange(ctx, query, start, end, step)
}

func (c *GrafanaClient) QueryLogs(ctx context.Context, q LogQuery) ([]LogLine, bool, error) {
	return c.loki.QueryLogs(ctx, q)
}

type headerRoundTripper struct {
	base    http.RoundTripper
	headers map[string]string
}

func (t *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	cloned := req.Clone(req.Context())
	for key, value := range t.headers {
		cloned.Header.Set(key, value)
	}
	return base.RoundTrip(cloned)
}
```

- [ ] **Step 4: Run observability tests and verify pass**

Run:

```bash
cd go/observability-mcp-service && go test ./internal/observability
```

Expected: PASS.

- [ ] **Step 5: Commit Grafana client**

Run:

```bash
git add go/observability-mcp-service/internal/observability/grafana.go go/observability-mcp-service/internal/observability/grafana_test.go
git commit -m "feat: add grafana observability client"
```

Expected: commit succeeds.

## Task 5: Wire Grafana Mode Into MCP Startup

**Files:**
- Modify: `go/observability-mcp-service/cmd/observability-mcp/main.go`
- Modify: `go/observability-mcp-service/cmd/observability-mcp/main_test.go`

- [ ] **Step 1: Write startup test**

In `go/observability-mcp-service/cmd/observability-mcp/main_test.go`, add:

```go
func TestRunUsesGrafanaGatewayMode(t *testing.T) {
	t.Setenv("OBS_GRAFANA_URL", "https://observability-api.kylebradshaw.dev")
	t.Setenv("OBS_GRAFANA_TOKEN", "token")
	var got *app
	err := run(context.Background(), log.New(io.Discard, "", 0), func(_ context.Context, application *app) error {
		got = application
		return nil
	})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if got == nil || got.service == nil {
		t.Fatal("expected app service")
	}
	if !got.cfg.UseGrafanaGateway() {
		t.Fatal("expected Grafana gateway mode")
	}
}
```

- [ ] **Step 2: Run command tests and verify failure or missing behavior**

Run:

```bash
cd go/observability-mcp-service && go test ./cmd/observability-mcp
```

Expected: FAIL until imports and Grafana mode wiring exist.

- [ ] **Step 3: Wire client selection**

In `run`, replace direct client construction with:

```go
	var prom workflows.Prometheus
	var loki workflows.Loki
	jaeger := observability.NewJaeger(cfg.JaegerURL, httpClient, cfg.MaxTraceSpans)
	if cfg.UseGrafanaGateway() {
		grafana := observability.NewGrafana(observability.GrafanaConfig{
			BaseURL:            cfg.GrafanaURL,
			Token:              cfg.GrafanaToken,
			AccessClientID:     cfg.GrafanaAccessClientID,
			AccessClientSecret: cfg.GrafanaAccessClientSecret,
		}, httpClient)
		prom = grafana
		loki = grafana
		logger.Printf("observability MCP server running on stdio grafana=%s jaeger=%s", cfg.GrafanaURL, cfg.JaegerURL)
	} else {
		prom = observability.NewPrometheus(cfg.PrometheusURL, httpClient)
		loki = observability.NewLoki(cfg.LokiURL, httpClient)
		logger.Printf("observability MCP server running on stdio prometheus=%s loki=%s jaeger=%s", cfg.PrometheusURL, cfg.LokiURL, cfg.JaegerURL)
	}
	service := workflows.NewService(prom, loki, jaeger, cfg.MaxLogLines)
```

Remove the old duplicate `prom`, `loki`, and `service` construction.

- [ ] **Step 4: Run command tests**

Run:

```bash
cd go/observability-mcp-service && go test ./cmd/observability-mcp
```

Expected: PASS.

- [ ] **Step 5: Run all MCP tests**

Run:

```bash
cd go/observability-mcp-service && go test ./...
```

Expected: PASS.

- [ ] **Step 6: Commit startup wiring**

Run:

```bash
git add go/observability-mcp-service/cmd/observability-mcp/main.go go/observability-mcp-service/cmd/observability-mcp/main_test.go
git commit -m "feat: run observability mcp through grafana gateway"
```

Expected: commit succeeds.

## Task 6: Update MCP Documentation And Local Registration

**Files:**
- Modify: `go/observability-mcp-service/README.md`
- Modify: `/Users/kylebradshaw/.codex/config.toml`
- Create locally, uncommitted: `~/.codex/env/observability-mcp.env`

- [ ] **Step 1: Update README**

Add a "Grafana Gateway Mode" section to `go/observability-mcp-service/README.md`:

````markdown
## Grafana Gateway Mode

For normal development-machine usage, prefer Grafana gateway mode instead of
direct local Prometheus/Loki/Jaeger ports.

Create an uncommitted local env file:

```bash
mkdir -p ~/.codex/env
$EDITOR ~/.codex/env/observability-mcp.env
```

Expected shape:

```bash
export OBS_GRAFANA_URL="https://observability-api.kylebradshaw.dev"
export OBS_GRAFANA_ACCESS_CLIENT_ID="<cloudflare-access-client-id>"
export OBS_GRAFANA_ACCESS_CLIENT_SECRET="<cloudflare-access-client-secret>"
```

Optional, if Grafana itself requires a service account token in addition to
Cloudflare Access:

```bash
export OBS_GRAFANA_TOKEN="<grafana-service-account-token>"
```

The MCP remains read-only. SSH-based diagnostics remain available through the
`debug-observability` skill for break-glass cases and when the observability
gateway is unavailable.
````

- [ ] **Step 2: Update Codex MCP registration**

Edit `/Users/kylebradshaw/.codex/config.toml`:

```toml
[mcp_servers.observability]
command = "zsh"
args = ["-lc", "source ~/.codex/env/observability-mcp.env && cd /Users/kylebradshaw/repos/gen_ai_engineer/go/observability-mcp-service && exec go run ./cmd/observability-mcp"]
```

- [ ] **Step 3: Create local env file**

Run:

```bash
mkdir -p ~/.codex/env
touch ~/.codex/env/observability-mcp.env
chmod 600 ~/.codex/env/observability-mcp.env
```

Then add real Cloudflare token values manually after Task 2 is completed in Cloudflare.

- [ ] **Step 4: Verify Codex sees registration**

Run:

```bash
codex mcp get observability
```

Expected: command shows `observability` enabled and the `source ~/.codex/env/observability-mcp.env` command.

- [ ] **Step 5: Commit README only**

Run:

```bash
git add go/observability-mcp-service/README.md
git commit -m "docs: document observability mcp grafana gateway mode"
```

Expected: commit succeeds. Do not commit `~/.codex/env/observability-mcp.env`.

## Task 7: Final Verification And Rollout Gate

**Files:**
- No new files.

- [ ] **Step 1: Run Go tests**

Run:

```bash
cd go/observability-mcp-service && go test ./...
```

Expected: PASS.

- [ ] **Step 2: Run Go preflight**

Run:

```bash
make preflight-go
```

Expected: PASS. If blocked by missing local tools, record the exact missing tool and leave remaining verification to CI.

- [ ] **Step 3: Verify public dashboard remains public after deploy**

After the committed ingress and Cloudflare changes are applied through the approved deployment path, run:

```bash
curl -I https://grafana.kylebradshaw.dev
```

Expected: HTTP 200 or a Grafana response that does not require Cloudflare Access.

- [ ] **Step 4: Verify API host denies anonymous access**

Run:

```bash
curl -I https://observability-api.kylebradshaw.dev/api/health
```

Expected: Cloudflare Access denial or redirect, not Grafana health JSON.

- [ ] **Step 5: Verify API host accepts service token**

Run:

```bash
source ~/.codex/env/observability-mcp.env
curl -fsS \
  -H "CF-Access-Client-Id: ${OBS_GRAFANA_ACCESS_CLIENT_ID}" \
  -H "CF-Access-Client-Secret: ${OBS_GRAFANA_ACCESS_CLIENT_SECRET}" \
  "${OBS_GRAFANA_URL}/api/health"
```

Expected: Grafana health JSON.

- [ ] **Step 6: Verify MCP command startup**

Run:

```bash
source ~/.codex/env/observability-mcp.env
cd go/observability-mcp-service
timeout 5s go run ./cmd/observability-mcp
```

Expected: process starts and exits due to `timeout`; stderr logs include `observability MCP server running on stdio grafana=https://observability-api.kylebradshaw.dev`.

- [ ] **Step 7: Commit any remaining tracked changes**

Run:

```bash
git status --short
```

Expected: no unexpected tracked changes. Commit only intentional repo changes.

## Self-Review Notes

- Spec coverage: public Grafana is preserved, private API host is added, Prometheus/Loki stay internal, SSH is kept as fallback, and MCP direct backend mode is retained.
- Jaeger is intentionally not required for first rollout because Grafana trace proxy behavior needs validation.
- Secrets are local-only and not committed.
- Shared-environment mutation is gated behind committed manifests/docs and the approved deployment path.
