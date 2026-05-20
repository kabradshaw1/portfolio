# Observability MCP Management Actions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add opt-in, cataloged management actions to the Observability MCP so agents can run low-risk committed ops procedures autonomously without free-form mutation.

**Architecture:** Add a new `internal/management` package for catalog, policy, runner, service, and result types. Extend existing config and SQLite history so management action previews/executions are policy-gated, bounded, redacted, and optionally attached to incident timelines. Register four MCP tools that delegate to the management service instead of exposing shell or Kubernetes primitives.

**Tech Stack:** Go, `github.com/modelcontextprotocol/go-sdk/mcp`, `modernc.org/sqlite`, stdlib `os/exec`, existing `config`, `history`, and `mcpserver` packages.

---

## File Structure

- Create `go/observability-mcp-service/internal/management/types.go`: risk tiers, decisions, statuses, action metadata, request/result types.
- Create `go/observability-mcp-service/internal/management/catalog.go`: built-in catalog and validation.
- Create `go/observability-mcp-service/internal/management/policy.go`: config-driven allow/block/preview policy.
- Create `go/observability-mcp-service/internal/management/redact.go`: bounded output and redaction helpers.
- Create `go/observability-mcp-service/internal/management/runner.go`: fixed-path script runner.
- Create `go/observability-mcp-service/internal/management/service.go`: list, preview, execute, and history orchestration.
- Create tests beside each new file in `internal/management`.
- Modify `go/observability-mcp-service/internal/config/config.go` and `config_test.go`: management env config.
- Modify `go/observability-mcp-service/internal/history/types.go`, `sqlite.go`, and `sqlite_test.go`: management action event persistence and query.
- Modify `go/observability-mcp-service/internal/mcpserver/server.go` and `server_test.go`: register and handle management tools.
- Modify `go/observability-mcp-service/cmd/observability-mcp/main.go` and `main_test.go`: wire management service from repo root and config.
- Modify `go/observability-mcp-service/README.md`: document opt-in management model.

## Task 1: Management Config

**Files:**
- Modify: `go/observability-mcp-service/internal/config/config.go`
- Modify: `go/observability-mcp-service/internal/config/config_test.go`

- [ ] **Step 1: Write failing config tests**

Add these tests to `config_test.go`:

```go
func TestFromEnvManagementDefaults(t *testing.T) {
	clearEnv(t)
	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error = %v", err)
	}
	if cfg.ManagementActionsEnabled {
		t.Fatal("expected management actions disabled by default")
	}
	if cfg.ManagementAllowHighRisk {
		t.Fatal("expected high-risk management actions disabled by default")
	}
	if cfg.ManagementActionTimeout != 45*time.Minute {
		t.Fatalf("ManagementActionTimeout = %s, want 45m", cfg.ManagementActionTimeout)
	}
	if cfg.ManagementMaxOutputBytes != 32768 {
		t.Fatalf("ManagementMaxOutputBytes = %d, want 32768", cfg.ManagementMaxOutputBytes)
	}
}

func TestFromEnvManagementOverrides(t *testing.T) {
	clearEnv(t)
	t.Setenv("OBS_MANAGEMENT_ACTIONS_ENABLED", "true")
	t.Setenv("OBS_MANAGEMENT_ALLOW_HIGH_RISK", "true")
	t.Setenv("OBS_MANAGEMENT_ACTION_TIMEOUT", "45s")
	t.Setenv("OBS_MANAGEMENT_MAX_OUTPUT_BYTES", "4096")
	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error = %v", err)
	}
	if !cfg.ManagementActionsEnabled || !cfg.ManagementAllowHighRisk {
		t.Fatalf("management bool overrides not applied: %+v", cfg)
	}
	if cfg.ManagementActionTimeout != 45*time.Second {
		t.Fatalf("ManagementActionTimeout = %s", cfg.ManagementActionTimeout)
	}
	if cfg.ManagementMaxOutputBytes != 4096 {
		t.Fatalf("ManagementMaxOutputBytes = %d", cfg.ManagementMaxOutputBytes)
	}
}

func TestFromEnvRejectsInvalidManagementConfig(t *testing.T) {
	clearEnv(t)
	t.Setenv("OBS_MANAGEMENT_ACTION_TIMEOUT", "0s")
	if _, err := FromEnv(); err == nil {
		t.Fatal("expected non-positive management timeout error")
	}
	clearEnv(t)
	t.Setenv("OBS_MANAGEMENT_MAX_OUTPUT_BYTES", "0")
	if _, err := FromEnv(); err == nil {
		t.Fatal("expected non-positive management output cap error")
	}
}
```

Also add these lines to `clearEnv(t)`:

```go
t.Setenv("OBS_MANAGEMENT_ACTIONS_ENABLED", "")
t.Setenv("OBS_MANAGEMENT_ALLOW_HIGH_RISK", "")
t.Setenv("OBS_MANAGEMENT_ACTION_TIMEOUT", "")
t.Setenv("OBS_MANAGEMENT_MAX_OUTPUT_BYTES", "")
```

- [ ] **Step 2: Run config tests and verify failure**

Run:

```bash
cd go/observability-mcp-service && go test ./internal/config -run 'Management|Defaults|Overrides|Invalid' -v
```

Expected: FAIL with missing `Config` fields such as `ManagementActionsEnabled`.

- [ ] **Step 3: Implement config fields and env parsing**

In `config.go`, add fields to `Config`:

```go
ManagementActionsEnabled bool
ManagementAllowHighRisk  bool
ManagementActionTimeout  time.Duration
ManagementMaxOutputBytes int
```

In `FromEnv()`, parse values:

```go
managementActionsEnabled, err := boolEnv("OBS_MANAGEMENT_ACTIONS_ENABLED", false)
if err != nil {
	return Config{}, err
}
managementAllowHighRisk, err := boolEnv("OBS_MANAGEMENT_ALLOW_HIGH_RISK", false)
if err != nil {
	return Config{}, err
}
managementActionTimeout, err := durationEnv("OBS_MANAGEMENT_ACTION_TIMEOUT", 45*time.Minute)
if err != nil {
	return Config{}, err
}
managementMaxOutputBytes, err := intEnv("OBS_MANAGEMENT_MAX_OUTPUT_BYTES", 32768)
if err != nil {
	return Config{}, err
}
```

Set them in the `Config` literal and validate:

```go
if cfg.ManagementActionTimeout <= 0 {
	return Config{}, fmt.Errorf("OBS_MANAGEMENT_ACTION_TIMEOUT must be positive")
}
if cfg.ManagementMaxOutputBytes <= 0 {
	return Config{}, fmt.Errorf("OBS_MANAGEMENT_MAX_OUTPUT_BYTES must be positive")
}
```

- [ ] **Step 4: Run config tests and commit**

Run:

```bash
cd go/observability-mcp-service && go test ./internal/config -v
```

Expected: PASS.

Commit:

```bash
git add go/observability-mcp-service/internal/config/config.go go/observability-mcp-service/internal/config/config_test.go
git commit -m "feat(observability): add management action config"
```

## Task 2: Catalog And Policy

**Files:**
- Create: `go/observability-mcp-service/internal/management/types.go`
- Create: `go/observability-mcp-service/internal/management/catalog.go`
- Create: `go/observability-mcp-service/internal/management/policy.go`
- Test: `go/observability-mcp-service/internal/management/catalog_test.go`
- Test: `go/observability-mcp-service/internal/management/policy_test.go`

- [ ] **Step 1: Write failing catalog tests**

Create `catalog_test.go`:

```go
package management

import (
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultCatalogContainsInitialActions(t *testing.T) {
	catalog := DefaultCatalog()
	for _, id := range []string{"reload_grafana_alerting", "run_postgres_backup_verify"} {
		action, ok := catalog.Get(id)
		if !ok {
			t.Fatalf("missing action %q", id)
		}
		if action.RiskTier != RiskLowRiskMutation {
			t.Fatalf("%s risk = %q", id, action.RiskTier)
		}
		if action.ScriptPath == "" || filepath.IsAbs(action.ScriptPath) {
			t.Fatalf("%s script path = %q", id, action.ScriptPath)
		}
		if action.Timeout <= 0 || action.Timeout > 45*time.Minute {
			t.Fatalf("%s timeout = %s", id, action.Timeout)
		}
	}
}

func TestDefaultCatalogScriptsExist(t *testing.T) {
	catalog := DefaultCatalog()
	repoRoot := filepath.Clean(filepath.Join("..", "..", "..", ".."))
	if err := catalog.ValidateScripts(repoRoot); err != nil {
		t.Fatalf("ValidateScripts() error = %v", err)
	}
}

func TestCatalogValidationRejectsBadEntries(t *testing.T) {
	tests := []struct {
		name    string
		actions []Action
	}{
		{name: "duplicate id", actions: []Action{{ID: "a", Title: "A", Description: "desc", RiskTier: RiskDiagnostic, ScriptPath: "scripts/ops/a.sh", Timeout: time.Second}, {ID: "a", Title: "B", Description: "desc", RiskTier: RiskDiagnostic, ScriptPath: "scripts/ops/b.sh", Timeout: time.Second}}},
		{name: "absolute path", actions: []Action{{ID: "a", Title: "A", Description: "desc", RiskTier: RiskDiagnostic, ScriptPath: "/tmp/a.sh", Timeout: time.Second}}},
		{name: "path traversal", actions: []Action{{ID: "a", Title: "A", Description: "desc", RiskTier: RiskDiagnostic, ScriptPath: "../a.sh", Timeout: time.Second}}},
		{name: "invalid risk", actions: []Action{{ID: "a", Title: "A", Description: "desc", RiskTier: RiskTier("surprise"), ScriptPath: "scripts/ops/a.sh", Timeout: time.Second}}},
		{name: "no timeout", actions: []Action{{ID: "a", Title: "A", Description: "desc", RiskTier: RiskDiagnostic, ScriptPath: "scripts/ops/a.sh"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewCatalog(tc.actions); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestValidateScriptsRejectsMissingScript(t *testing.T) {
	catalog, err := NewCatalog([]Action{{ID: "a", Title: "A", Description: "desc", RiskTier: RiskDiagnostic, ScriptPath: "scripts/ops/missing.sh", Timeout: time.Second}})
	if err != nil {
		t.Fatal(err)
	}
	if err := catalog.ValidateScripts(t.TempDir()); err == nil {
		t.Fatal("expected missing script error")
	}
}
```

- [ ] **Step 2: Write failing policy tests**

Create `policy_test.go`:

```go
package management

import "testing"

func TestPolicyDecisions(t *testing.T) {
	tests := []struct {
		name   string
		policy Policy
		risk   RiskTier
		want   Decision
	}{
		{name: "disabled diagnostic blocked", policy: Policy{}, risk: RiskDiagnostic, want: DecisionBlock},
		{name: "disabled low risk blocked", policy: Policy{}, risk: RiskLowRiskMutation, want: DecisionBlock},
		{name: "enabled diagnostic allowed", policy: Policy{ActionsEnabled: true}, risk: RiskDiagnostic, want: DecisionAllow},
		{name: "enabled low risk allowed", policy: Policy{ActionsEnabled: true}, risk: RiskLowRiskMutation, want: DecisionAllow},
		{name: "high risk preview", policy: Policy{ActionsEnabled: true}, risk: RiskHighRiskMutation, want: DecisionPreviewOnly},
		{name: "high risk enabled", policy: Policy{ActionsEnabled: true, AllowHighRisk: true}, risk: RiskHighRiskMutation, want: DecisionAllow},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.policy.Evaluate(Action{ID: "a", RiskTier: tc.risk})
			if got.Decision != tc.want {
				t.Fatalf("Decision = %q, want %q; reason=%s", got.Decision, tc.want, got.Reason)
			}
		})
	}
}
```

- [ ] **Step 3: Run tests and verify failure**

Run:

```bash
cd go/observability-mcp-service && go test ./internal/management -v
```

Expected: FAIL because the package and symbols do not exist.

- [ ] **Step 4: Implement types, catalog, and policy**

Create `types.go` with:

```go
package management

import "time"

type RiskTier string
type Decision string
type Status string

const (
	RiskDiagnostic       RiskTier = "diagnostic"
	RiskLowRiskMutation RiskTier = "low_risk_mutation"
	RiskHighRiskMutation RiskTier = "high_risk_mutation"

	DecisionAllow       Decision = "allow"
	DecisionBlock       Decision = "block"
	DecisionPreviewOnly Decision = "preview_only"

	StatusPreviewed Status = "previewed"
	StatusBlocked   Status = "blocked"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusTimedOut  Status = "timed_out"
)

type Action struct {
	ID               string        `json:"id"`
	Title            string        `json:"title"`
	Description      string        `json:"description"`
	RiskTier         RiskTier      `json:"risk_tier"`
	ScriptPath       string        `json:"script_path"`
	AllowedArgs      []Argument    `json:"allowed_args,omitempty"`
	Timeout          time.Duration `json:"-"`
	TimeoutText      string        `json:"timeout"`
	RequiresIncident bool          `json:"requires_incident"`
	Idempotent        bool          `json:"idempotent"`
	Preflight        string        `json:"preflight"`
	Postflight       string        `json:"postflight"`
	RedactPatterns   []string      `json:"-"`
	NextSteps        string        `json:"next_steps"`
}

type Argument struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
}

type PolicyDecision struct {
	Decision Decision `json:"decision"`
	Reason   string   `json:"reason"`
}
```

Create `catalog.go` with `NewCatalog`, `Get`, `List`, `ValidateScripts(repoRoot string) error`, `DefaultCatalog`, and validation matching the tests. `ValidateScripts` must reject missing scripts and paths that escape `repoRoot`. `DefaultCatalog()` must include only:

```go
Action{
	ID: "reload_grafana_alerting",
	Title: "Reload Grafana alerting",
	Description: "Restart Grafana so committed alerting provisioning is loaded and verified.",
	RiskTier: RiskLowRiskMutation,
	ScriptPath: "scripts/ops/2026-05-09-reload-grafana-alerting.sh",
	Timeout: 2 * time.Minute,
	TimeoutText: "2m0s",
	Idempotent: true,
	Preflight: "Script verifies the expected Grafana alerting ConfigMap content before restart.",
	Postflight: "Script waits for rollout and verifies live Grafana alert expression and active alert count.",
	NextSteps: "Inspect script output and rerun system health evidence if the action fails.",
}
```

and:

```go
Action{
	ID: "run_postgres_backup_verify",
	Title: "Run Postgres backup verification",
	Description: "Create a manual Job from the committed Postgres backup verification CronJob and wait for completion.",
	RiskTier: RiskLowRiskMutation,
	ScriptPath: "scripts/ops/2026-05-15-run-postgres-backup-verify.sh",
	Timeout: 40 * time.Minute,
	TimeoutText: "40m0s",
	Idempotent: true,
	Preflight: "Script creates a timestamped Job from the existing CronJob.",
	Postflight: "Script waits for completion and prints bounded pod logs.",
	NextSteps: "Review Job logs and investigate backup-verification alerts if the action fails.",
}
```

Create `policy.go`:

```go
package management

type Policy struct {
	ActionsEnabled bool
	AllowHighRisk  bool
}

func (p Policy) Evaluate(action Action) PolicyDecision {
	if !p.ActionsEnabled {
		return PolicyDecision{Decision: DecisionBlock, Reason: "management actions are disabled; set OBS_MANAGEMENT_ACTIONS_ENABLED=true"}
	}
	if action.RiskTier == RiskHighRiskMutation && !p.AllowHighRisk {
		return PolicyDecision{Decision: DecisionPreviewOnly, Reason: "high-risk management actions require OBS_MANAGEMENT_ALLOW_HIGH_RISK=true"}
	}
	return PolicyDecision{Decision: DecisionAllow, Reason: "action is allowed by management policy"}
}
```

- [ ] **Step 5: Run tests and commit**

Run:

```bash
cd go/observability-mcp-service && go test ./internal/management -v
```

Expected: PASS.

Commit:

```bash
git add go/observability-mcp-service/internal/management
git commit -m "feat(observability): add management action catalog"
```

## Task 3: Runner And Redaction

**Files:**
- Create: `go/observability-mcp-service/internal/management/redact.go`
- Create: `go/observability-mcp-service/internal/management/runner.go`
- Test: `go/observability-mcp-service/internal/management/runner_test.go`

- [ ] **Step 1: Write failing runner tests**

Create `runner_test.go`:

```go
package management

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunnerExecutesCatalogScriptAndBoundsOutput(t *testing.T) {
	repo := t.TempDir()
	script := filepath.Join(repo, "scripts/ops/test.sh")
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte("#!/usr/bin/env bash\nprintf 'abcdef'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	runner := Runner{RepoRoot: repo, MaxOutputBytes: 4, MaxTimeout: time.Second}
	result := runner.Run(context.Background(), Action{ID: "test", ScriptPath: "scripts/ops/test.sh", Timeout: time.Second})
	if result.Status != StatusSucceeded {
		t.Fatalf("status = %q stderr=%q", result.Status, result.Stderr)
	}
	if result.Stdout != "abcd" || !result.OutputTruncated {
		t.Fatalf("stdout=%q truncated=%t", result.Stdout, result.OutputTruncated)
	}
}

func TestRunnerRejectsUnsafeScriptPath(t *testing.T) {
	runner := Runner{RepoRoot: t.TempDir(), MaxOutputBytes: 1024, MaxTimeout: time.Second}
	result := runner.Run(context.Background(), Action{ID: "bad", ScriptPath: "../bad.sh", Timeout: time.Second})
	if result.Status != StatusFailed || !strings.Contains(result.Stderr, "unsafe script path") {
		t.Fatalf("result = %+v", result)
	}
}

func TestRunnerRedactsSecretLookingOutput(t *testing.T) {
	repo := t.TempDir()
	script := filepath.Join(repo, "scripts/ops/secret.sh")
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte("#!/usr/bin/env bash\necho 'token=super-secret-value'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	runner := Runner{RepoRoot: repo, MaxOutputBytes: 1024, MaxTimeout: time.Second}
	result := runner.Run(context.Background(), Action{ID: "secret", ScriptPath: "scripts/ops/secret.sh", Timeout: time.Second})
	if strings.Contains(result.Stdout, "super-secret-value") {
		t.Fatalf("secret was not redacted: %q", result.Stdout)
	}
	if result.RedactionsApplied == 0 {
		t.Fatalf("expected redaction count, got %+v", result)
	}
}

func TestRunnerTimesOut(t *testing.T) {
	repo := t.TempDir()
	script := filepath.Join(repo, "scripts/ops/slow.sh")
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte("#!/usr/bin/env bash\nsleep 2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	runner := Runner{RepoRoot: repo, MaxOutputBytes: 1024, MaxTimeout: time.Second}
	result := runner.Run(context.Background(), Action{ID: "slow", ScriptPath: "scripts/ops/slow.sh", Timeout: 10 * time.Millisecond})
	if result.Status != StatusTimedOut {
		t.Fatalf("status = %q, want timed_out", result.Status)
	}
}
```

- [ ] **Step 2: Run runner tests and verify failure**

Run:

```bash
cd go/observability-mcp-service && go test ./internal/management -run Runner -v
```

Expected: FAIL because `Runner` and result fields do not exist.

- [ ] **Step 3: Implement runner result and redaction**

Add to `types.go`:

```go
type ActionResult struct {
	ActionID          string         `json:"action_id"`
	RiskTier          RiskTier       `json:"risk_tier"`
	Decision          Decision       `json:"decision"`
	Status            Status         `json:"status"`
	ScriptPath        string         `json:"script_path"`
	IncidentKey       string         `json:"incident_key,omitempty"`
	StartedAt         time.Time      `json:"started_at,omitempty"`
	CompletedAt       time.Time      `json:"completed_at,omitempty"`
	DurationMillis    int64          `json:"duration_ms,omitempty"`
	ExitCode          int            `json:"exit_code,omitempty"`
	Stdout            string         `json:"stdout,omitempty"`
	Stderr            string         `json:"stderr,omitempty"`
	OutputTruncated   bool           `json:"output_truncated,omitempty"`
	RedactionsApplied int            `json:"redactions_applied,omitempty"`
	PolicyReason      string         `json:"policy_reason,omitempty"`
	HistoryEventIDs   []int64        `json:"history_event_ids,omitempty"`
	Warning           string         `json:"warning,omitempty"`
}
```

Create `redact.go` with `boundOutput` and `redactOutput`. Use regexes for `(?i)(token|secret|password|authorization)=...` and `(?i)bearer\\s+...`; replace values with `[REDACTED]` and return a count.

Create `runner.go` with:

```go
type Runner struct {
	RepoRoot       string
	MaxOutputBytes int
	MaxTimeout     time.Duration
}

func (r Runner) Run(ctx context.Context, action Action) ActionResult
```

Implementation details:
- Clean and join `RepoRoot` plus `action.ScriptPath`.
- Reject absolute paths, `..`, and paths that escape `RepoRoot`.
- Use `exec.CommandContext(ctxWithTimeout, "bash", fullPath)`.
- Use `action.Timeout` unless `MaxTimeout` is set and lower; the lower timeout is the effective context deadline.
- Set `cmd.Dir = r.RepoRoot`.
- Capture stdout/stderr with `bytes.Buffer`.
- Map success to `StatusSucceeded`, context deadline to `StatusTimedOut`, and nonzero exit to `StatusFailed`.
- Always bound and redact output.

- [ ] **Step 4: Run runner tests and commit**

Run:

```bash
cd go/observability-mcp-service && go test ./internal/management -v
```

Expected: PASS.

Commit:

```bash
git add go/observability-mcp-service/internal/management
git commit -m "feat(observability): add management action runner"
```

## Task 4: History Persistence

**Files:**
- Modify: `go/observability-mcp-service/internal/history/types.go`
- Modify: `go/observability-mcp-service/internal/history/sqlite.go`
- Modify: `go/observability-mcp-service/internal/history/sqlite_test.go`

- [ ] **Step 1: Write failing history tests**

Add to `sqlite_test.go`:

```go
func TestRecordManagementActionCreatesIncidentTimeline(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	event, err := db.RecordManagementAction(ctx, ManagementActionInput{
		IncidentKey:   "grafana-alerting",
		IncidentTitle: "Grafana alerting reload",
		ActionID:      "reload_grafana_alerting",
		RiskTier:      "low_risk_mutation",
		Decision:      "allow",
		Status:        "succeeded",
		Summary:       "reload_grafana_alerting succeeded",
		DetailsJSON:   []byte(`{"status":"succeeded"}`),
	})
	if err != nil {
		t.Fatalf("RecordManagementAction() error = %v", err)
	}
	if event.ID == 0 || event.Type != EventManagementActionCompleted {
		t.Fatalf("event = %+v", event)
	}
	history, err := db.GetIncidentHistory(ctx, "grafana-alerting")
	if err != nil {
		t.Fatalf("GetIncidentHistory() error = %v", err)
	}
	if len(history.Events) != 1 {
		t.Fatalf("events = %d, want 1", len(history.Events))
	}
	if history.Events[0].Details != `{"status":"succeeded"}` {
		t.Fatalf("details = %q", history.Events[0].Details)
	}
}

func TestRecordManagementActionRequiresTitleForNewIncident(t *testing.T) {
	db := openTestDB(t)
	_, err := db.RecordManagementAction(context.Background(), ManagementActionInput{
		IncidentKey: "missing-title",
		ActionID:    "reload_grafana_alerting",
		RiskTier:    "low_risk_mutation",
		Decision:    "allow",
		Status:      "succeeded",
		Summary:     "reload_grafana_alerting succeeded",
	})
	if err == nil {
		t.Fatal("expected missing title error")
	}
}

func TestListManagementActionsFilters(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	_, _ = db.RecordManagementAction(ctx, ManagementActionInput{IncidentKey: "one", IncidentTitle: "One", ActionID: "reload_grafana_alerting", RiskTier: "low_risk_mutation", Decision: "allow", Status: "succeeded", Summary: "one"})
	_, _ = db.RecordManagementAction(ctx, ManagementActionInput{IncidentKey: "two", IncidentTitle: "Two", ActionID: "run_postgres_backup_verify", RiskTier: "low_risk_mutation", Decision: "block", Status: "blocked", Summary: "two"})
	events, err := db.ListManagementActions(ctx, ManagementActionFilter{ActionID: "reload_grafana_alerting", Limit: 10})
	if err != nil {
		t.Fatalf("ListManagementActions() error = %v", err)
	}
	if len(events) != 1 || events[0].Summary != "one" {
		t.Fatalf("events = %+v", events)
	}
}
```

- [ ] **Step 2: Run history tests and verify failure**

Run:

```bash
cd go/observability-mcp-service && go test ./internal/history -run Management -v
```

Expected: FAIL because management history types and methods do not exist.

- [ ] **Step 3: Implement history types and methods**

In `types.go`, add event constants:

```go
EventManagementActionPreviewed = "management_action_previewed"
EventManagementActionStarted   = "management_action_started"
EventManagementActionCompleted = "management_action_completed"
EventManagementActionFailed    = "management_action_failed"
EventManagementActionBlocked   = "management_action_blocked"
```

Add:

```go
type ManagementActionInput struct {
	IncidentKey   string
	IncidentTitle string
	Severity      string
	Service       string
	ActionID      string
	RiskTier      string
	Decision      string
	Status        string
	Summary       string
	DetailsJSON   []byte
}

type ManagementActionFilter struct {
	IncidentKey string
	ActionID    string
	Status      string
	Decision    string
	Limit       int
}
```

Extend `Store` with:

```go
RecordManagementAction(context.Context, ManagementActionInput) (Event, error)
ListManagementActions(context.Context, ManagementActionFilter) ([]Event, error)
```

In `sqlite.go`, implement `RecordManagementAction` by reusing `upsertIncident` and `insertTimelineEvent`. Choose event type from status:
- `previewed` -> `EventManagementActionPreviewed`
- `blocked` -> `EventManagementActionBlocked`
- `succeeded` -> `EventManagementActionCompleted`
- `failed` or `timed_out` -> `EventManagementActionFailed`
- default -> `EventManagementActionStarted`

Implement `ListManagementActions` by joining `timeline_events` to `incidents`, filtering event types to the five management action constants, and filtering JSON details with `LIKE` for `"action_id":"<id>"`, `"status":"<status>"`, and `"decision":"<decision>"`.

- [ ] **Step 4: Update fake history stores**

Update fake stores in `internal/workflows/service_test.go` and any compiler errors with no-op methods:

```go
func (f *fakeHistoryStore) RecordManagementAction(context.Context, history.ManagementActionInput) (history.Event, error) {
	return history.Event{ID: 1}, f.err
}

func (f *fakeHistoryStore) ListManagementActions(context.Context, history.ManagementActionFilter) ([]history.Event, error) {
	return []history.Event{}, f.err
}
```

- [ ] **Step 5: Run history tests and commit**

Run:

```bash
cd go/observability-mcp-service && go test ./internal/history ./internal/workflows -v
```

Expected: PASS.

Commit:

```bash
git add go/observability-mcp-service/internal/history go/observability-mcp-service/internal/workflows/service_test.go
git commit -m "feat(observability): persist management action history"
```

## Task 5: Management Service

**Files:**
- Create: `go/observability-mcp-service/internal/management/service.go`
- Test: `go/observability-mcp-service/internal/management/service_test.go`

- [ ] **Step 1: Write failing service tests**

Create `service_test.go`:

```go
package management

import (
	"context"
	"testing"
	"time"

	"github.com/kabradshaw1/portfolio/go/observability-mcp-service/internal/history"
)

type fakeRunner struct{ result ActionResult }

func (f fakeRunner) Run(context.Context, Action) ActionResult { return f.result }

type fakeStore struct {
	inputs []history.ManagementActionInput
	events []history.Event
}

func (f *fakeStore) RecordManagementAction(_ context.Context, in history.ManagementActionInput) (history.Event, error) {
	f.inputs = append(f.inputs, in)
	return history.Event{ID: int64(len(f.inputs)), Type: history.EventManagementActionCompleted, Summary: in.Summary}, nil
}

func (f *fakeStore) ListManagementActions(context.Context, history.ManagementActionFilter) ([]history.Event, error) {
	return f.events, nil
}

func TestServicePreviewBlocksDisabledPolicy(t *testing.T) {
	catalog := mustCatalog(t, []Action{{ID: "a", Title: "A", Description: "desc", RiskTier: RiskLowRiskMutation, ScriptPath: "scripts/ops/a.sh", Timeout: time.Second, TimeoutText: "1s"}})
	service := NewService(catalog, Policy{}, fakeRunner{}, nil)
	result := service.Preview(context.Background(), ActionRequest{ActionID: "a"})
	if result.Status != StatusBlocked || result.Decision != DecisionBlock {
		t.Fatalf("result = %+v", result)
	}
}

func TestServiceExecuteRunsAllowedActionAndRecordsHistory(t *testing.T) {
	catalog := mustCatalog(t, []Action{{ID: "a", Title: "A", Description: "desc", RiskTier: RiskLowRiskMutation, ScriptPath: "scripts/ops/a.sh", Timeout: time.Second, TimeoutText: "1s"}})
	store := &fakeStore{}
	service := NewService(catalog, Policy{ActionsEnabled: true}, fakeRunner{result: ActionResult{Status: StatusSucceeded, Stdout: "ok"}}, store)
	result := service.Execute(context.Background(), ActionRequest{ActionID: "a", IncidentKey: "inc", IncidentTitle: "Incident"})
	if result.Status != StatusSucceeded || result.Decision != DecisionAllow {
		t.Fatalf("result = %+v", result)
	}
	if len(store.inputs) != 1 || store.inputs[0].ActionID != "a" {
		t.Fatalf("history inputs = %+v", store.inputs)
	}
	if len(result.HistoryEventIDs) != 1 {
		t.Fatalf("history ids = %+v", result.HistoryEventIDs)
	}
}

func TestServiceExecuteRequiresTitleForNewIncident(t *testing.T) {
	catalog := mustCatalog(t, []Action{{ID: "a", Title: "A", Description: "desc", RiskTier: RiskLowRiskMutation, ScriptPath: "scripts/ops/a.sh", Timeout: time.Second, TimeoutText: "1s"}})
	service := NewService(catalog, Policy{ActionsEnabled: true}, fakeRunner{result: ActionResult{Status: StatusSucceeded}}, &fakeStore{})
	result := service.Execute(context.Background(), ActionRequest{ActionID: "a", IncidentKey: "inc"})
	if result.Status != StatusBlocked || result.PolicyReason != "incident_title is required when incident_key creates action history" {
		t.Fatalf("result = %+v", result)
	}
}

func mustCatalog(t *testing.T, actions []Action) Catalog {
	t.Helper()
	catalog, err := NewCatalog(actions)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}
```

- [ ] **Step 2: Run service tests and verify failure**

Run:

```bash
cd go/observability-mcp-service && go test ./internal/management -run Service -v
```

Expected: FAIL because `Service`, `ActionRequest`, and `NewService` do not exist.

- [ ] **Step 3: Implement service**

Add to `types.go`:

```go
type ActionRequest struct {
	ActionID       string         `json:"action_id"`
	Args           map[string]any `json:"args,omitempty"`
	IncidentKey    string         `json:"incident_key,omitempty"`
	IncidentTitle  string         `json:"incident_title,omitempty"`
	Severity       string         `json:"severity,omitempty"`
	Service        string         `json:"service,omitempty"`
}
```

In `service.go`, define:

```go
type RunnerInterface interface {
	Run(context.Context, Action) ActionResult
}

type HistoryStore interface {
	RecordManagementAction(context.Context, history.ManagementActionInput) (history.Event, error)
	ListManagementActions(context.Context, history.ManagementActionFilter) ([]history.Event, error)
}

type Service struct {
	catalog Catalog
	policy  Policy
	runner  RunnerInterface
	history HistoryStore
}
```

Implement:
- `List() []Action`
- `Preview(ctx context.Context, req ActionRequest) ActionResult`
- `Execute(ctx context.Context, req ActionRequest) ActionResult`
- `History(ctx context.Context, filter history.ManagementActionFilter) ([]history.Event, error)`

Rules:
- Unknown `ActionID` returns `StatusBlocked`, `DecisionBlock`, `PolicyReason: "unknown management action"`.
- If `IncidentKey` is set and `IncidentTitle` is empty, block before execution.
- Preview never runs the runner.
- Execute runs only when policy decision is `allow`.
- Record one final event per preview/block/execution result. If history is nil, set `Warning: "management action history is disabled"`.

- [ ] **Step 4: Run service tests and commit**

Run:

```bash
cd go/observability-mcp-service && go test ./internal/management -v
```

Expected: PASS.

Commit:

```bash
git add go/observability-mcp-service/internal/management
git commit -m "feat(observability): add management action service"
```

## Task 6: MCP Tool Handlers

**Files:**
- Modify: `go/observability-mcp-service/internal/mcpserver/server.go`
- Modify: `go/observability-mcp-service/internal/mcpserver/server_test.go`

- [ ] **Step 1: Write failing MCP tests**

In `server_test.go`, import management:

```go
"github.com/kabradshaw1/portfolio/go/observability-mcp-service/internal/management"
```

Add assertions to the tool registration test for:

```go
"list_management_actions", "preview_management_action", "execute_management_action", "get_management_action_history"
```

Add handler tests:

```go
func TestManagementActionHandlers(t *testing.T) {
	fake := &fakeWorkflow{}
	result, err := previewManagementActionHandler(fake)(context.Background(), callReq(map[string]any{"action_id": "reload_grafana_alerting"}))
	if err != nil || result.IsError {
		t.Fatalf("preview failed: result=%#v err=%v", result, err)
	}
	if fake.managementRequest.ActionID != "reload_grafana_alerting" {
		t.Fatalf("request = %+v", fake.managementRequest)
	}

	result, err = executeManagementActionHandler(fake)(context.Background(), callReq(map[string]any{
		"action_id": "reload_grafana_alerting",
		"incident_key": "inc",
		"incident_title": "Incident",
	}))
	if err != nil || result.IsError {
		t.Fatalf("execute failed: result=%#v err=%v", result, err)
	}
	if fake.managementRequest.IncidentKey != "inc" {
		t.Fatalf("request = %+v", fake.managementRequest)
	}
}

func TestManagementActionHistoryHandlerValidatesLimit(t *testing.T) {
	result, err := managementActionHistoryHandler(&fakeWorkflow{})(context.Background(), callReq(map[string]any{"limit": 101}))
	if err != nil {
		t.Fatalf("handler transport error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected tool error")
	}
}
```

Extend `fakeWorkflow` with:

```go
managementRequest management.ActionRequest
```

and methods:

```go
func (f *fakeWorkflow) ListManagementActions(context.Context) ([]management.Action, error) {
	return []management.Action{{ID: "reload_grafana_alerting"}}, nil
}

func (f *fakeWorkflow) PreviewManagementAction(_ context.Context, req management.ActionRequest) (management.ActionResult, error) {
	f.managementRequest = req
	return management.ActionResult{ActionID: req.ActionID, Status: management.StatusPreviewed}, nil
}

func (f *fakeWorkflow) ExecuteManagementAction(_ context.Context, req management.ActionRequest) (management.ActionResult, error) {
	f.managementRequest = req
	return management.ActionResult{ActionID: req.ActionID, Status: management.StatusSucceeded}, nil
}

func (f *fakeWorkflow) ListManagementActionHistory(context.Context, history.ManagementActionFilter) ([]history.Event, error) {
	return []history.Event{}, nil
}
```

- [ ] **Step 2: Run MCP tests and verify failure**

Run:

```bash
cd go/observability-mcp-service && go test ./internal/mcpserver -run Management -v
```

Expected: FAIL because handlers and interface methods do not exist.

- [ ] **Step 3: Implement MCP interface and handlers**

In `server.go`, extend `WorkflowService` with:

```go
ListManagementActions(context.Context) ([]management.Action, error)
PreviewManagementAction(context.Context, management.ActionRequest) (management.ActionResult, error)
ExecuteManagementAction(context.Context, management.ActionRequest) (management.ActionResult, error)
ListManagementActionHistory(context.Context, history.ManagementActionFilter) ([]history.Event, error)
```

Register tools in `New()` after incident tools:

```go
addTool(srv, "list_management_actions", "List cataloged observability management actions.", listManagementActionsSchema(), listManagementActionsHandler(service))
addTool(srv, "preview_management_action", "Validate and preview a cataloged management action without executing it.", managementActionSchema(), previewManagementActionHandler(service))
addTool(srv, "execute_management_action", "Execute an allowed cataloged management action.", managementActionSchema(), executeManagementActionHandler(service))
addTool(srv, "get_management_action_history", "List persisted management action events.", managementActionHistorySchema(), managementActionHistoryHandler(service))
```

Add schemas with `additionalProperties:false`. `managementActionSchema` requires `action_id` and allows `args`, `incident_key`, `incident_title`, `severity`, and `service`. History schema allows `incident_key`, `action_id`, `status`, `decision`, and `limit` with max 100.

Implement handlers using existing `decodeArgs`, `decodeOptionalArgs`, `jsonResult`, and `toolError`.

- [ ] **Step 4: Run MCP tests and commit**

Run:

```bash
cd go/observability-mcp-service && go test ./internal/mcpserver -v
```

Expected: PASS.

Commit:

```bash
git add go/observability-mcp-service/internal/mcpserver
git commit -m "feat(observability): expose management action tools"
```

## Task 7: Workflow And Main Wiring

**Files:**
- Modify: `go/observability-mcp-service/internal/workflows/service.go`
- Modify: `go/observability-mcp-service/internal/workflows/service_test.go`
- Modify: `go/observability-mcp-service/cmd/observability-mcp/main.go`
- Modify: `go/observability-mcp-service/cmd/observability-mcp/main_test.go`

- [ ] **Step 1: Write failing workflow wiring tests**

In `workflows/service_test.go`, import management and add:

```go
func TestManagementActionsUnavailableUntilConfigured(t *testing.T) {
	service := NewService(nil, nil, nil, 10)
	if _, err := service.ListManagementActions(context.Background()); err == nil {
		t.Fatal("expected management service disabled error")
	}
}
```

In `main_test.go`, add:

```go
func TestRunWiresManagementWhenEnabled(t *testing.T) {
	clearEnv(t)
	t.Setenv("OBS_MANAGEMENT_ACTIONS_ENABLED", "true")
	t.Setenv("OBS_MANAGEMENT_ACTION_TIMEOUT", "30s")
	err := run(context.Background(), log.New(&bytes.Buffer{}, "", 0), func(ctx context.Context, application *app) error {
		actions, err := application.service.ListManagementActions(ctx)
		if err != nil {
			t.Fatalf("ListManagementActions() error = %v", err)
		}
		if len(actions) == 0 {
			t.Fatal("expected management actions")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
}
```

Add management env resets to `clearEnv(t)`.

- [ ] **Step 2: Run wiring tests and verify failure**

Run:

```bash
cd go/observability-mcp-service && go test ./internal/workflows ./cmd/observability-mcp -run Management -v
```

Expected: FAIL because workflow service has no management methods.

- [ ] **Step 3: Add workflow delegation**

In `workflows.Service`, add:

```go
management *management.Service
```

Add:

```go
func (s *Service) WithManagement(m *management.Service) *Service {
	s.management = m
	return s
}
```

Implement the four management methods by returning `errors.New("management actions are disabled")` when `s.management == nil`, otherwise delegating to the management service.

- [ ] **Step 4: Wire main**

In `main.go`, import `path/filepath`, `runtime`, and `internal/management`.

Add helper:

```go
func repoRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
}
```

After history setup, build:

```go
catalog := management.DefaultCatalog()
if err := catalog.ValidateScripts(repoRoot()); err != nil {
	return fmt.Errorf("management catalog: %w", err)
}
runner := management.Runner{RepoRoot: repoRoot(), MaxOutputBytes: cfg.ManagementMaxOutputBytes, MaxTimeout: cfg.ManagementActionTimeout}
managementService := management.NewService(
	catalog,
	management.Policy{ActionsEnabled: cfg.ManagementActionsEnabled, AllowHighRisk: cfg.ManagementAllowHighRisk},
	runner,
	historyDB,
)
service.WithManagement(managementService)
```

Keep `historyDB` as a variable outside the history `if` so it can be nil when history is disabled.

- [ ] **Step 5: Run wiring tests and commit**

Run:

```bash
cd go/observability-mcp-service && go test ./internal/workflows ./cmd/observability-mcp -v
```

Expected: PASS.

Commit:

```bash
git add go/observability-mcp-service/internal/workflows go/observability-mcp-service/cmd/observability-mcp
git commit -m "feat(observability): wire management actions"
```

## Task 8: README And End-To-End Checks

**Files:**
- Modify: `go/observability-mcp-service/README.md`

- [ ] **Step 1: Update README**

In the configuration table, add:

```markdown
| `OBS_MANAGEMENT_ACTIONS_ENABLED` | `false` | Enables cataloged management action execution. Read-only evidence tools work without this. |
| `OBS_MANAGEMENT_ALLOW_HIGH_RISK` | `false` | Allows high-risk cataloged actions to execute instead of preview-only. Keep false for normal use. |
| `OBS_MANAGEMENT_ACTION_TIMEOUT` | `45m` | Default maximum action execution timeout unless a catalog entry is lower. |
| `OBS_MANAGEMENT_MAX_OUTPUT_BYTES` | `32768` | Maximum stdout/stderr bytes returned and stored for each action. |
```

In the tools list, add:

```markdown
- `list_management_actions`
- `preview_management_action`
- `execute_management_action`
- `get_management_action_history`
```

Replace the Safety paragraph with wording that says:

```markdown
By default, external observability and runtime systems remain read-only. When
`OBS_MANAGEMENT_ACTIONS_ENABLED=true`, the MCP can execute cataloged management
actions only. Those actions must map to committed repo scripts, use fixed
command shapes, bounded inputs, timeouts, output redaction, and incident-history
logging. The MCP still does not accept free-form shell, kubectl, SSH commands,
Grafana mutation payloads, arbitrary URLs, or arbitrary script paths.
```

- [ ] **Step 2: Run focused tests**

Run:

```bash
cd go/observability-mcp-service && go test ./internal/config ./internal/history ./internal/management ./internal/mcpserver ./internal/workflows ./cmd/observability-mcp -v
```

Expected: PASS.

- [ ] **Step 3: Run full Go preflight**

Run from repo root:

```bash
make preflight-go
```

Expected: PASS. If blocked by local toolchain, capture the exact missing tool or failing command in the final handoff.

- [ ] **Step 4: Commit docs and any final fixes**

Commit:

```bash
git add go/observability-mcp-service/README.md
git commit -m "docs(observability): document management actions"
```

## Task 9: Final Review

**Files:**
- Review all changed files from Tasks 1-8.

- [ ] **Step 1: Inspect final diff**

Run:

```bash
git status --short
git log --oneline -8
git diff qa...HEAD -- go/observability-mcp-service
```

Expected: the diff contains only management-action implementation, tests, and README updates.

- [ ] **Step 2: Verify safety properties in code**

Use `rg` to confirm there is no free-form command API:

```bash
rg -n "kubectl|ssh|CommandContext|ScriptPath|Args|shell|/bin/sh" go/observability-mcp-service/internal
```

Expected: `CommandContext` appears only in `internal/management/runner.go`; `kubectl` and `ssh` appear only in static script metadata or README text, not as accepted user input.

- [ ] **Step 3: Confirm final preflight evidence**

Run:

```bash
make preflight-go
```

Expected: PASS.

- [ ] **Step 4: Prepare PR**

Because this changes Go application behavior, implementation should be done in a feature worktree/branch and opened as a PR to `qa`.

Use a PR summary with:

```markdown
## Summary
- added opt-in cataloged management actions for the Observability MCP
- added policy-gated runner, output redaction, and action history persistence
- documented the agent-autonomous ops-as-code safety model

## Tests
- make preflight-go
```
