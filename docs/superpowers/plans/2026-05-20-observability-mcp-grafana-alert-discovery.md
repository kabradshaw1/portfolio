# Observability MCP Grafana Alert Discovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add read-only Grafana active alert and alert rule metadata to the `get_system_health` Observability MCP evidence bundle.

**Architecture:** Introduce a focused Grafana alerting client that reuses existing Grafana auth headers but calls alerting APIs directly. Extend workflow evidence with a compact alert summary, wire it only into `GetSystemHealth`, and degrade Grafana alerting failures to partial evidence.

**Tech Stack:** Go, `net/http`, `encoding/json`, Grafana HTTP API, MCP Go SDK, existing Observability MCP test fakes.

---

## Execution Notes

Implementation changes touch Go application code, so do not work directly on `qa`. Before Task 1, create or select a feature worktree under `.codex/worktrees/<branch-name>/` using `superpowers:using-git-worktrees`, then run all searches, edits, tests, commits, and pushes from that worktree.

Official Grafana references checked while writing this plan:

- Grafana documents legacy HTTP API authentication with service account tokens:
  <https://grafana.com/docs/grafana/latest/developer-resources/api-reference/http-api/api-legacy/>
- Grafana documents `GET /api/v1/provisioning/alert-rules` for provisioned alert rules, while noting `/api` routes are deprecated starting in Grafana 13 and remain available for now.
  <https://grafana.com/docs/grafana/latest/developer-resources/api-reference/http-api/api-legacy/alerting_provisioning/>
- The repo already uses `GET /api/alertmanager/grafana/api/v2/alerts` in `scripts/ops/2026-05-09-reload-grafana-alerting.sh` for active-alert verification.

## File Structure

- Modify `go/observability-mcp-service/internal/observability/types.go`
  - Add compact `AlertInstance`, `AlertRule`, and `AlertSummary` data types.
- Create `go/observability-mcp-service/internal/observability/grafana_alerting.go`
  - Add read-only alerting API methods and response normalization.
- Modify `go/observability-mcp-service/internal/observability/grafana.go`
  - Store shared Grafana API base URL/client and expose alerting methods.
- Modify `go/observability-mcp-service/internal/observability/grafana_test.go`
  - Add request path, auth header, parsing, and bounded-output tests.
- Modify `go/observability-mcp-service/internal/workflows/types.go`
  - Add `Alerts observability.AlertSummary` to `EvidenceBundle`.
- Modify `go/observability-mcp-service/internal/workflows/service.go`
  - Add optional `GrafanaAlerting` dependency and call it from `GetSystemHealth`.
- Modify `go/observability-mcp-service/internal/workflows/service_test.go`
  - Add alerting success, firing finding, failure, and skipped tests.
- Modify `go/observability-mcp-service/cmd/observability-mcp/main.go`
  - Pass Grafana alerting dependency only when Grafana gateway mode is configured.
- Modify `go/observability-mcp-service/README.md`
  - Document read-only alert metadata and required read permissions.

### Task 1: Add Alerting Types

**Files:**
- Modify: `go/observability-mcp-service/internal/observability/types.go`

- [ ] **Step 1: Write the type definitions**

Add these types after `TraceSummary`:

```go
type AlertInstance struct {
	Name         string            `json:"name,omitempty"`
	State        string            `json:"state,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
	Annotations  map[string]string `json:"annotations,omitempty"`
	StartsAt     time.Time         `json:"starts_at,omitempty"`
	EndsAt       time.Time         `json:"ends_at,omitempty"`
	GeneratorURL string            `json:"generator_url,omitempty"`
	DashboardURL string            `json:"dashboard_url,omitempty"`
	RuleUID      string            `json:"rule_uid,omitempty"`
}

type AlertRule struct {
	UID        string            `json:"uid,omitempty"`
	Title      string            `json:"title,omitempty"`
	FolderUID  string            `json:"folder_uid,omitempty"`
	Namespace  string            `json:"namespace,omitempty"`
	RuleGroup  string            `json:"rule_group,omitempty"`
	Condition  string            `json:"condition,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
	Provenance string            `json:"provenance,omitempty"`
}

type AlertSummary struct {
	ActiveAlerts []AlertInstance `json:"active_alerts,omitempty"`
	Rules        []AlertRule     `json:"rules,omitempty"`
	Truncated    bool            `json:"truncated,omitempty"`
}
```

- [ ] **Step 2: Run package tests**

Run:

```bash
cd go/observability-mcp-service
go test ./internal/observability
```

Expected: PASS. This is a compile-only guard for the new exported types.

- [ ] **Step 3: Commit**

```bash
git add go/observability-mcp-service/internal/observability/types.go
git commit -m "feat: add alerting evidence types"
```

### Task 2: Add Grafana Alerting Client Tests

**Files:**
- Modify: `go/observability-mcp-service/internal/observability/grafana_test.go`
- Create later: `go/observability-mcp-service/internal/observability/grafana_alerting.go`

- [ ] **Step 1: Write failing tests for active alerts and rule metadata**

Append these tests to `grafana_test.go`:

```go
func TestGrafanaActiveAlertsUsesAlertmanagerAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer grafana-token" {
			t.Fatalf("Authorization = %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("CF-Access-Client-Id") != "cf-id" {
			t.Fatalf("CF-Access-Client-Id = %s", r.Header.Get("CF-Access-Client-Id"))
		}
		if r.Header.Get("CF-Access-Client-Secret") != "cf-secret" {
			t.Fatalf("CF-Access-Client-Secret = %s", r.Header.Get("CF-Access-Client-Secret"))
		}
		if r.URL.Path != "/api/alertmanager/grafana/api/v2/alerts" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[
			{
				"labels":{"alertname":"HighErrorRate","grafana_rule_uid":"rule-123","service":"go-order-service"},
				"annotations":{"summary":"Order service errors","__dashboardUid__":"orders"},
				"startsAt":"2026-05-20T12:00:00Z",
				"endsAt":"0001-01-01T00:00:00Z",
				"generatorURL":"https://grafana.example/alerting/grafana/rule-123/view",
				"status":{"state":"active"}
			}
		]`))
	}))
	defer server.Close()

	client := NewGrafana(GrafanaConfig{
		BaseURL:            server.URL,
		Token:              "grafana-token",
		AccessClientID:     "cf-id",
		AccessClientSecret: "cf-secret",
	}, server.Client())

	got, err := client.ActiveAlerts(context.Background())
	if err != nil {
		t.Fatalf("ActiveAlerts() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("alerts = %+v", got)
	}
	if got[0].Name != "HighErrorRate" || got[0].State != "active" || got[0].RuleUID != "rule-123" {
		t.Fatalf("alert = %+v", got[0])
	}
	if got[0].Annotations["summary"] != "Order service errors" {
		t.Fatalf("annotations = %+v", got[0].Annotations)
	}
}

func TestGrafanaAlertRulesUsesProvisioningAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/provisioning/alert-rules" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[
			{
				"uid":"rule-123",
				"title":"HighErrorRate",
				"folderUID":"go-services",
				"ruleGroup":"slo",
				"condition":"C",
				"labels":{"service":"go-order-service"},
				"provenance":"file"
			}
		]`))
	}))
	defer server.Close()

	client := NewGrafana(GrafanaConfig{BaseURL: server.URL}, server.Client())
	got, err := client.AlertRules(context.Background())
	if err != nil {
		t.Fatalf("AlertRules() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("rules = %+v", got)
	}
	if got[0].UID != "rule-123" || got[0].Title != "HighErrorRate" || got[0].FolderUID != "go-services" {
		t.Fatalf("rule = %+v", got[0])
	}
}

func TestGrafanaAlertingBoundsLabelsAndAnnotations(t *testing.T) {
	labels := map[string]string{
		"alertname": "Noisy",
		"keep1":     "1",
		"keep2":     "2",
		"keep3":     "3",
		"keep4":     "4",
		"keep5":     "5",
		"drop":      "6",
	}
	got := boundedMap(labels, 5)
	if len(got) != 5 {
		t.Fatalf("bounded labels length = %d", len(got))
	}
	if got["alertname"] != "Noisy" {
		t.Fatalf("bounded labels = %+v", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
cd go/observability-mcp-service
go test ./internal/observability
```

Expected: FAIL with undefined methods `ActiveAlerts`, `AlertRules`, or helper `boundedMap`.

### Task 3: Implement Grafana Alerting Client

**Files:**
- Modify: `go/observability-mcp-service/internal/observability/grafana.go`
- Create: `go/observability-mcp-service/internal/observability/grafana_alerting.go`

- [ ] **Step 1: Update Grafana client struct**

In `grafana.go`, change `GrafanaClient` and `NewGrafana` so the client keeps the base URL and shared HTTP client:

```go
type GrafanaClient struct {
	baseURL    string
	http       *http.Client
	prometheus *PrometheusClient
	loki       *LokiClient
}
```

Inside `NewGrafana`, return:

```go
return &GrafanaClient{
	baseURL:    baseURL,
	http:       &client,
	prometheus: NewPrometheus(baseURL+"/api/datasources/proxy/uid/"+prometheusUID, &client),
	loki:       NewLoki(baseURL+"/api/datasources/proxy/uid/"+lokiUID, &client),
}
```

- [ ] **Step 2: Add alerting implementation**

Create `grafana_alerting.go` with this content:

```go
package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"
)

const maxAlertMetadataEntries = 20

func (c *GrafanaClient) ActiveAlerts(ctx context.Context) ([]AlertInstance, error) {
	var raw []grafanaAlert
	if err := c.getJSON(ctx, "/api/alertmanager/grafana/api/v2/alerts", &raw); err != nil {
		return nil, err
	}
	alerts := make([]AlertInstance, 0, len(raw))
	for _, alert := range raw {
		alerts = append(alerts, alert.toAlertInstance())
	}
	return alerts, nil
}

func (c *GrafanaClient) AlertRules(ctx context.Context) ([]AlertRule, error) {
	var raw []grafanaAlertRule
	if err := c.getJSON(ctx, "/api/v1/provisioning/alert-rules", &raw); err != nil {
		return nil, err
	}
	rules := make([]AlertRule, 0, len(raw))
	for _, rule := range raw {
		rules = append(rules, rule.toAlertRule())
	}
	return rules, nil
}

func (c *GrafanaClient) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("grafana HTTP status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode grafana response: %w", err)
	}
	return nil
}

type grafanaAlert struct {
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	StartsAt     time.Time         `json:"startsAt"`
	EndsAt       time.Time         `json:"endsAt"`
	GeneratorURL string            `json:"generatorURL"`
	DashboardURL string            `json:"dashboardURL"`
	Status       struct {
		State string `json:"state"`
	} `json:"status"`
}

func (a grafanaAlert) toAlertInstance() AlertInstance {
	labels := boundedMap(a.Labels, maxAlertMetadataEntries)
	annotations := boundedMap(a.Annotations, maxAlertMetadataEntries)
	name := labels["alertname"]
	ruleUID := labels["grafana_rule_uid"]
	if ruleUID == "" {
		ruleUID = labels["rule_uid"]
	}
	return AlertInstance{
		Name:         name,
		State:        a.Status.State,
		Labels:       labels,
		Annotations:  annotations,
		StartsAt:     a.StartsAt,
		EndsAt:       a.EndsAt,
		GeneratorURL: a.GeneratorURL,
		DashboardURL: a.DashboardURL,
		RuleUID:      ruleUID,
	}
}

type grafanaAlertRule struct {
	UID        string            `json:"uid"`
	Title      string            `json:"title"`
	FolderUID  string            `json:"folderUID"`
	Namespace  string            `json:"namespace_uid"`
	RuleGroup  string            `json:"ruleGroup"`
	Condition  string            `json:"condition"`
	Labels     map[string]string `json:"labels"`
	Provenance string            `json:"provenance"`
}

func (r grafanaAlertRule) toAlertRule() AlertRule {
	namespace := r.Namespace
	if namespace == "" {
		namespace = r.FolderUID
	}
	return AlertRule{
		UID:        r.UID,
		Title:      r.Title,
		FolderUID:  r.FolderUID,
		Namespace:  namespace,
		RuleGroup:  r.RuleGroup,
		Condition:  r.Condition,
		Labels:     boundedMap(r.Labels, maxAlertMetadataEntries),
		Provenance: r.Provenance,
	}
}

func boundedMap(values map[string]string, limit int) map[string]string {
	if len(values) == 0 || limit <= 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > limit {
		keys = keys[:limit]
	}
	bounded := make(map[string]string, len(keys))
	for _, key := range keys {
		bounded[key] = values[key]
	}
	return bounded
}
```

- [ ] **Step 3: Run tests**

Run:

```bash
cd go/observability-mcp-service
go test ./internal/observability
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add go/observability-mcp-service/internal/observability/grafana.go go/observability-mcp-service/internal/observability/grafana_alerting.go go/observability-mcp-service/internal/observability/grafana_test.go
git commit -m "feat: query grafana alert metadata"
```

### Task 4: Wire Alerting Into System Health

**Files:**
- Modify: `go/observability-mcp-service/internal/workflows/types.go`
- Modify: `go/observability-mcp-service/internal/workflows/service.go`
- Modify: `go/observability-mcp-service/internal/workflows/service_test.go`
- Modify: `go/observability-mcp-service/cmd/observability-mcp/main.go`

- [ ] **Step 1: Write failing workflow tests**

Add this interface and fake to `service_test.go` near the other fakes:

```go
type fakeGrafanaAlerting struct {
	alerts []observability.AlertInstance
	rules  []observability.AlertRule
	err    error
}

func (f *fakeGrafanaAlerting) ActiveAlerts(_ context.Context) ([]observability.AlertInstance, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.alerts, nil
}

func (f *fakeGrafanaAlerting) AlertRules(_ context.Context) ([]observability.AlertRule, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.rules, nil
}
```

Append these tests:

```go
func TestGetSystemHealthIncludesGrafanaAlerting(t *testing.T) {
	alerting := &fakeGrafanaAlerting{
		alerts: []observability.AlertInstance{{
			Name:    "HighErrorRate",
			State:   "active",
			RuleUID: "rule-123",
			Labels:  map[string]string{"alertname": "HighErrorRate"},
		}},
		rules: []observability.AlertRule{{
			UID:   "rule-123",
			Title: "HighErrorRate",
		}},
	}
	service := NewService(&fakePrometheus{}, nil, nil, 10)
	service.SetGrafanaAlerting(alerting)

	got := service.GetSystemHealth(context.Background(), time.Minute)

	if len(got.Alerts.ActiveAlerts) != 1 {
		t.Fatalf("alerts = %+v", got.Alerts)
	}
	if len(got.Alerts.Rules) != 1 {
		t.Fatalf("rules = %+v", got.Alerts)
	}
	if got.Status != "warning" {
		t.Fatalf("status = %s", got.Status)
	}
	if !slices.Contains(sourceStatuses(got.Sources), "grafana_alerting:ok") {
		t.Fatalf("sources = %+v", got.Sources)
	}
}

func TestGetSystemHealthSkipsGrafanaAlertingWhenUnconfigured(t *testing.T) {
	service := NewService(&fakePrometheus{}, nil, nil, 10)
	got := service.GetSystemHealth(context.Background(), time.Minute)
	if len(got.Alerts.ActiveAlerts) != 0 || len(got.Alerts.Rules) != 0 {
		t.Fatalf("alerts = %+v", got.Alerts)
	}
	if !slices.Contains(sourceStatuses(got.Sources), "grafana_alerting:skipped") {
		t.Fatalf("sources = %+v", got.Sources)
	}
}

func TestGetSystemHealthGrafanaAlertingFailureIsPartial(t *testing.T) {
	service := NewService(&fakePrometheus{}, nil, nil, 10)
	service.SetGrafanaAlerting(&fakeGrafanaAlerting{err: errors.New("grafana down")})

	got := service.GetSystemHealth(context.Background(), time.Minute)

	if !got.Partial {
		t.Fatalf("bundle should be partial: %+v", got)
	}
	if len(got.Signals) == 0 {
		t.Fatalf("prometheus evidence should remain: %+v", got)
	}
	if !slices.Contains(sourceStatuses(got.Sources), "grafana_alerting:error") {
		t.Fatalf("sources = %+v", got.Sources)
	}
}

func sourceStatuses(sources []SourceStatus) []string {
	statuses := make([]string, 0, len(sources))
	for _, source := range sources {
		statuses = append(statuses, source.Name+":"+source.Status)
	}
	return statuses
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
cd go/observability-mcp-service
go test ./internal/workflows
```

Expected: FAIL with missing `EvidenceBundle.Alerts` and `SetGrafanaAlerting`.

- [ ] **Step 3: Add workflow interface, bundle field, and wiring**

In `workflows/types.go`, add this field to `EvidenceBundle` after `Traces`:

```go
Alerts observability.AlertSummary `json:"alerts,omitempty"`
```

In `workflows/service.go`, add this interface near the existing dependency interfaces:

```go
type GrafanaAlerting interface {
	ActiveAlerts(context.Context) ([]observability.AlertInstance, error)
	AlertRules(context.Context) ([]observability.AlertRule, error)
}
```

Add this field to `Service`:

```go
grafanaAlerting GrafanaAlerting
```

Add this setter:

```go
func (s *Service) SetGrafanaAlerting(alerting GrafanaAlerting) {
	s.grafanaAlerting = alerting
}
```

Update `GetSystemHealth`:

```go
func (s *Service) GetSystemHealth(ctx context.Context, window time.Duration) EvidenceBundle {
	b := s.bundle("get_system_health", window)
	s.addPrometheusSignals(ctx, &b, systemHealthQueries)
	s.addGrafanaAlerting(ctx, &b)
	s.finalize(&b)
	return b
}
```

Add this helper:

```go
func (s *Service) addGrafanaAlerting(ctx context.Context, b *EvidenceBundle) {
	if s.grafanaAlerting == nil {
		b.Sources = append(b.Sources, SourceStatus{Name: "grafana_alerting", Status: "skipped"})
		return
	}
	alerts, err := s.grafanaAlerting.ActiveAlerts(ctx)
	if err != nil {
		b.AddError("grafana_alerting", "active_alerts", err.Error())
		b.Sources = append(b.Sources, SourceStatus{Name: "grafana_alerting", Status: "error"})
		return
	}
	rules, err := s.grafanaAlerting.AlertRules(ctx)
	if err != nil {
		b.AddError("grafana_alerting", "alert_rules", err.Error())
		b.Sources = append(b.Sources, SourceStatus{Name: "grafana_alerting", Status: "error"})
		return
	}
	b.Alerts = observability.AlertSummary{ActiveAlerts: alerts, Rules: matchingRules(alerts, rules)}
	b.Sources = append(b.Sources, SourceStatus{Name: "grafana_alerting", Status: "ok"})
	for _, alert := range alerts {
		if alert.State == "active" || alert.State == "firing" {
			title := "Grafana alert is firing"
			if alert.Name != "" {
				title = "Grafana alert is firing: " + alert.Name
			}
			b.Findings = append(b.Findings, Finding{Severity: "warning", Title: title, Evidence: alert.RuleUID})
		}
	}
}

func matchingRules(alerts []observability.AlertInstance, rules []observability.AlertRule) []observability.AlertRule {
	if len(alerts) == 0 || len(rules) == 0 {
		return nil
	}
	wanted := make(map[string]struct{}, len(alerts))
	for _, alert := range alerts {
		if alert.RuleUID != "" {
			wanted[alert.RuleUID] = struct{}{}
		}
	}
	if len(wanted) == 0 {
		return nil
	}
	matched := make([]observability.AlertRule, 0, len(wanted))
	for _, rule := range rules {
		if _, ok := wanted[rule.UID]; ok {
			matched = append(matched, rule)
		}
	}
	return matched
}
```

- [ ] **Step 4: Wire Grafana dependency in main**

In `cmd/observability-mcp/main.go`, after creating `service := workflows.NewService(...)`, add:

```go
if cfg.UseGrafanaGateway() {
	if grafana, ok := prom.(*observability.GrafanaClient); ok {
		service.SetGrafanaAlerting(grafana)
	}
}
```

This works because Grafana gateway mode sets both `prom` and `loki` to the same `*observability.GrafanaClient`.

- [ ] **Step 5: Run workflow and command tests**

Run:

```bash
cd go/observability-mcp-service
go test ./internal/workflows ./cmd/observability-mcp
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add go/observability-mcp-service/internal/workflows/types.go go/observability-mcp-service/internal/workflows/service.go go/observability-mcp-service/internal/workflows/service_test.go go/observability-mcp-service/cmd/observability-mcp/main.go
git commit -m "feat: include grafana alerts in system health"
```

### Task 5: Update Documentation And Run Preflight

**Files:**
- Modify: `go/observability-mcp-service/README.md`

- [ ] **Step 1: Update README**

In the README configuration table, update the `OBS_GRAFANA_TOKEN` row to:

```markdown
| `OBS_GRAFANA_TOKEN` | unset | Optional Grafana service account token. When alert discovery is enabled, this token only needs read access to alerting metadata. |
```

Under `## Grafana Gateway Mode`, add:

```markdown
When `OBS_GRAFANA_URL` is configured, `get_system_health` also performs
read-only Grafana alert discovery. It queries active alert instances and
provisioned alert rule metadata, then returns compact metadata in the evidence
bundle. The MCP does not silence alerts, edit rules, restart workloads, or run
ops commands.
```

In `## Safety`, update the first paragraph to:

```markdown
V1 is read-only. It queries metrics, logs, traces, embedded runbook text, and
Grafana alert metadata. It does not call Kubernetes write APIs, roll out or
restart workloads, scale deployments, purge queues, silence alerts, mutate
databases, edit Grafana rules, or read secrets.
```

- [ ] **Step 2: Run targeted tests**

Run:

```bash
cd go/observability-mcp-service
go test ./...
```

Expected: PASS.

- [ ] **Step 3: Run required Go preflight**

Run from repo root:

```bash
make preflight-go
```

Expected: PASS. If this fails for unrelated existing issues or local toolchain limits, capture the exact failing command and output before deciding whether to fix or report the blocker.

- [ ] **Step 4: Commit**

```bash
git add go/observability-mcp-service/README.md
git commit -m "docs: describe grafana alert discovery"
```

### Task 6: Final Review And PR

**Files:**
- Review all modified files.

- [ ] **Step 1: Inspect final diff**

Run:

```bash
git diff qa...HEAD -- go/observability-mcp-service
```

Expected: Diff only covers Grafana alert discovery, system health wiring, tests, and README documentation.

- [ ] **Step 2: Verify no mutation path was introduced**

Run:

```bash
rg -n "Silence|silence|Pause|pause|Restart|restart|rollout|delete|POST|PUT|PATCH|DELETE" go/observability-mcp-service
```

Expected: No new write-capable Grafana/Kubernetes management method. Existing README safety text may contain non-goal words such as `restart` or `silence`.

- [ ] **Step 3: Push feature branch**

Run:

```bash
git status --short
git push -u origin HEAD
```

Expected: clean worktree before push; branch pushed successfully.

- [ ] **Step 4: Create PR to `qa`**

Run:

```bash
gh pr create --base qa --title "Add Grafana alert discovery to observability MCP" --body "## Summary
- add read-only Grafana active alert and alert rule discovery
- include alert metadata in get_system_health evidence bundles
- preserve partial evidence behavior when Grafana alerting is unavailable

## Tests
- go test ./... in go/observability-mcp-service
- make preflight-go"
```

Expected: PR created. Per branch workflow, do not watch CI unless Kyle asks.
