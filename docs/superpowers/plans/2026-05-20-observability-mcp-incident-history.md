# Observability MCP Incident History Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add durable local incident history, notes/status, evidence snapshots, and evidence-window comparison to `go/observability-mcp-service`.

**Architecture:** Add a focused SQLite store under `internal/history` and keep SQL out of MCP handlers and observability clients. The workflow service owns evidence capture, best-effort persistence, and comparison logic; the MCP server owns JSON schema validation and tool wiring. Existing investigation tools continue to work when history is disabled or persistence fails.

**Tech Stack:** Go 1.26.1, `modernc.org/sqlite`, MCP Go SDK, local SQLite, existing `workflows.EvidenceBundle` JSON.

---

### File Structure

- Create `go/observability-mcp-service/internal/history/types.go`: public history DTOs, status/event constants, store interface inputs/outputs.
- Create `go/observability-mcp-service/internal/history/sqlite.go`: SQLite open, migration, incident upsert, snapshot insert, list/history/note queries.
- Create `go/observability-mcp-service/internal/history/sqlite_test.go`: migration and store behavior tests.
- Modify `go/observability-mcp-service/internal/config/config.go`: history env vars and bool parsing.
- Modify `go/observability-mcp-service/internal/config/config_test.go`: config defaults, overrides, disabled state, invalid bool.
- Modify `go/observability-mcp-service/internal/workflows/types.go`: persistence metadata, snapshot summaries, comparison response, optional persistence metadata on bundles.
- Modify `go/observability-mcp-service/internal/workflows/service.go`: add history store dependency, capture helpers, note/list/history/comparison methods.
- Modify `go/observability-mcp-service/internal/workflows/service_test.go`: best-effort persistence and comparison tests.
- Modify `go/observability-mcp-service/internal/mcpserver/server.go`: new tools and existing investigation tool schemas with optional persistence metadata.
- Modify `go/observability-mcp-service/internal/mcpserver/server_test.go`: handler/schema tests.
- Modify `go/observability-mcp-service/cmd/observability-mcp/main.go`: open/migrate/close history DB when enabled.
- Modify `go/observability-mcp-service/cmd/observability-mcp/main_test.go`: startup behavior tests.
- Modify `go/observability-mcp-service/README.md`: document history env vars and tools.
- Modify `go/observability-mcp-service/go.mod` and `go/observability-mcp-service/go.sum`: add `modernc.org/sqlite`.

### Task 1: Add History Store Types

**Files:**
- Create: `go/observability-mcp-service/internal/history/types.go`
- Test: `go/observability-mcp-service/internal/history/sqlite_test.go`

- [ ] **Step 1: Create type definitions**

Create `internal/history/types.go` with:

```go
package history

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("history record not found")

const (
	StatusInvestigating = "investigating"
	StatusMitigated     = "mitigated"
	StatusResolved      = "resolved"

	EventEvidenceSnapshot = "evidence_snapshot"
	EventNoteAdded        = "note_added"
	EventStatusChanged    = "status_changed"
)

type IncidentUpsert struct {
	Key      string
	Title    string
	Status   string
	Severity string
	Service  string
}

type Incident struct {
	ID        int64     `json:"id"`
	Key       string    `json:"incident_key"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	Severity  string    `json:"severity,omitempty"`
	Service   string    `json:"service,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type SnapshotInput struct {
	IncidentKey    string
	IncidentTitle  string
	Severity       string
	Service        string
	Tool           string
	WindowFrom     time.Time
	WindowTo       time.Time
	WindowDuration string
	Status         string
	Partial        bool
	CriticalCount  int
	WarningCount   int
	SignalCount    int
	LogCount       int
	TraceCount     int
	SourceStatuses []SourceStatus
	BundleJSON     []byte
}

type SourceStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type Snapshot struct {
	ID             int64          `json:"id"`
	IncidentID     *int64         `json:"incident_id,omitempty"`
	TimelineEventID *int64        `json:"timeline_event_id,omitempty"`
	Tool           string         `json:"tool"`
	WindowFrom     time.Time      `json:"window_from"`
	WindowTo       time.Time      `json:"window_to"`
	WindowDuration string         `json:"window_duration"`
	Status         string         `json:"status"`
	Partial        bool           `json:"partial"`
	CriticalCount  int            `json:"critical_findings"`
	WarningCount   int            `json:"warning_findings"`
	SignalCount    int            `json:"signal_count"`
	LogCount       int            `json:"log_count"`
	TraceCount     int            `json:"trace_count"`
	SourceStatuses []SourceStatus `json:"source_statuses"`
	BundleJSON     []byte         `json:"-"`
	CreatedAt      time.Time      `json:"created_at"`
}

type Event struct {
	ID         int64     `json:"id"`
	IncidentID int64     `json:"incident_id"`
	Type       string    `json:"event_type"`
	Summary    string    `json:"summary"`
	Details    string    `json:"details,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	Snapshot   *Snapshot  `json:"snapshot,omitempty"`
}

type IncidentSummary struct {
	Incident      Incident `json:"incident"`
	SnapshotCount int      `json:"snapshot_count"`
	LastEventAt   time.Time `json:"last_event_at"`
}

type IncidentHistory struct {
	Incident Incident `json:"incident"`
	Events   []Event  `json:"events"`
}

type ListFilter struct {
	Status   string
	Service  string
	Severity string
	Limit    int
}

type AddNoteInput struct {
	IncidentKey string
	Note        string
	Status      string
}

type Store interface {
	Close() error
	Migrate(context.Context) error
	RecordSnapshot(context.Context, SnapshotInput) (Snapshot, error)
	ListIncidents(context.Context, ListFilter) ([]IncidentSummary, error)
	GetIncidentHistory(context.Context, string) (IncidentHistory, error)
	AddIncidentNote(context.Context, AddNoteInput) (Event, error)
	GetSnapshot(context.Context, int64) (Snapshot, error)
}

func NormalizeStatus(status string) string {
	if status == "" {
		return StatusInvestigating
	}
	return status
}

func ValidStatus(status string) bool {
	switch status {
	case StatusInvestigating, StatusMitigated, StatusResolved:
		return true
	default:
		return false
	}
}
```

- [ ] **Step 2: Run package test to verify missing implementation still compiles or reports missing files only**

Run: `cd go/observability-mcp-service && go test ./internal/history`

Expected: the package compiles and reports `[no test files]` because `types.go` exists and no tests have been added yet.

- [ ] **Step 3: Commit**

```bash
git add go/observability-mcp-service/internal/history/types.go
git commit -m "feat: define observability history types"
```

### Task 2: Add SQLite Store And Tests

**Files:**
- Create: `go/observability-mcp-service/internal/history/sqlite.go`
- Create: `go/observability-mcp-service/internal/history/sqlite_test.go`
- Modify: `go/observability-mcp-service/go.mod`
- Modify: `go/observability-mcp-service/go.sum`

- [ ] **Step 1: Add SQLite dependency**

Run: `cd go/observability-mcp-service && go get modernc.org/sqlite@v1.39.1`

Expected: `go.mod` includes `modernc.org/sqlite`.

- [ ] **Step 2: Write failing migration and snapshot tests**

Create `internal/history/sqlite_test.go` with tests named:

```go
func TestSQLiteMigrateIsIdempotent(t *testing.T)
func TestRecordSnapshotCreatesIncidentTimelineAndImmutableSnapshot(t *testing.T)
func TestListIncidentsFiltersByStatusServiceSeverity(t *testing.T)
func TestAddIncidentNoteCanTransitionStatus(t *testing.T)
func TestGetSnapshotReturnsErrNotFound(t *testing.T)
```

Use helper setup:

```go
func openTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	return db
}
```

Use sample input:

```go
func sampleSnapshot(key string) SnapshotInput {
	return SnapshotInput{
		IncidentKey:    key,
		IncidentTitle:  "Checkout failures",
		Severity:       "warning",
		Service:        "go-order-service",
		Tool:           "investigate_checkout",
		WindowFrom:     time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC),
		WindowTo:       time.Date(2026, 5, 20, 12, 15, 0, 0, time.UTC),
		WindowDuration: "15m0s",
		Status:         "warning",
		Partial:        false,
		CriticalCount:  0,
		WarningCount:   1,
		SignalCount:    2,
		LogCount:       3,
		TraceCount:     0,
		SourceStatuses: []SourceStatus{{Name: "prometheus", Status: "ok"}},
		BundleJSON:     []byte(`{"tool":"investigate_checkout","status":"warning"}`),
	}
}
```

- [ ] **Step 3: Run tests and verify failure**

Run: `cd go/observability-mcp-service && go test ./internal/history`

Expected: FAIL because `Open`, `DB`, and store methods are undefined.

- [ ] **Step 4: Implement SQLite store**

Create `internal/history/sqlite.go` with:

```go
package history

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	db *sql.DB
}

func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create history db dir: %w", err)
	}
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open history sqlite: %w", err)
	}
	return &DB{db: sqlDB}, nil
}

func OpenSQL(db *sql.DB) *DB {
	return &DB{db: db}
}

func (d *DB) Close() error {
	return d.db.Close()
}
```

Implement `Migrate` with `CREATE TABLE IF NOT EXISTS incidents`, `timeline_events`, and `evidence_snapshots`, plus indexes:

```sql
CREATE UNIQUE INDEX IF NOT EXISTS incidents_key_unique ON incidents(incident_key);
CREATE INDEX IF NOT EXISTS timeline_events_incident_created_idx ON timeline_events(incident_id, created_at, id);
CREATE INDEX IF NOT EXISTS evidence_snapshots_tool_created_idx ON evidence_snapshots(tool, created_at);
```

Implement methods using transactions for `RecordSnapshot` and `AddIncidentNote`.
Store times as RFC3339Nano strings. Marshal/unmarshal `SourceStatuses` as JSON.

- [ ] **Step 5: Run tests and fix compile/runtime issues**

Run: `cd go/observability-mcp-service && go test ./internal/history -v`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add go/observability-mcp-service/internal/history go/observability-mcp-service/go.mod go/observability-mcp-service/go.sum
git commit -m "feat: persist observability incident history"
```

### Task 3: Add History Configuration

**Files:**
- Modify: `go/observability-mcp-service/internal/config/config.go`
- Modify: `go/observability-mcp-service/internal/config/config_test.go`

- [ ] **Step 1: Write failing config tests**

Add tests:

```go
func TestFromEnvHistoryDefaults(t *testing.T) {
	clearEnv(t)
	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error = %v", err)
	}
	if !cfg.HistoryEnabled {
		t.Fatal("expected history enabled by default")
	}
	if cfg.HistoryAutoCapture {
		t.Fatal("expected auto capture disabled by default")
	}
	if !strings.HasSuffix(cfg.HistoryDBPath, "observability-mcp/history.db") {
		t.Fatalf("HistoryDBPath = %q", cfg.HistoryDBPath)
	}
}

func TestFromEnvHistoryOverrides(t *testing.T) {
	clearEnv(t)
	t.Setenv("OBS_HISTORY_ENABLED", "false")
	t.Setenv("OBS_HISTORY_AUTO_CAPTURE", "true")
	t.Setenv("OBS_HISTORY_DB_PATH", "/tmp/obs-history.db")
	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error = %v", err)
	}
	if cfg.HistoryEnabled {
		t.Fatal("expected history disabled")
	}
	if !cfg.HistoryAutoCapture {
		t.Fatal("expected auto capture enabled")
	}
	if cfg.HistoryDBPath != "/tmp/obs-history.db" {
		t.Fatalf("HistoryDBPath = %q", cfg.HistoryDBPath)
	}
}

func TestFromEnvRejectsInvalidHistoryBool(t *testing.T) {
	clearEnv(t)
	t.Setenv("OBS_HISTORY_ENABLED", "maybe")
	if _, err := FromEnv(); err == nil {
		t.Fatal("expected invalid bool error")
	}
}
```

Add `strings` import to the test file and clear the three new env vars in `clearEnv`.

- [ ] **Step 2: Run config tests and verify failure**

Run: `cd go/observability-mcp-service && go test ./internal/config`

Expected: FAIL because history config fields do not exist.

- [ ] **Step 3: Implement config**

Add fields:

```go
HistoryEnabled     bool
HistoryDBPath      string
HistoryAutoCapture bool
```

Add default helper:

```go
func defaultHistoryDBPath() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".codex", "data", "observability-mcp", "history.db")
	}
	return filepath.Join(".codex", "data", "observability-mcp", "history.db")
}
```

Add bool parser:

```go
func boolEnv(key string, fallback bool) (bool, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s: %w", key, err)
	}
	return parsed, nil
}
```

Use `OBS_HISTORY_ENABLED`, `OBS_HISTORY_DB_PATH`, and `OBS_HISTORY_AUTO_CAPTURE` in `FromEnv`.

- [ ] **Step 4: Run config tests**

Run: `cd go/observability-mcp-service && go test ./internal/config -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add go/observability-mcp-service/internal/config
git commit -m "feat: configure observability history storage"
```

### Task 4: Wire History Store Into Workflows

**Files:**
- Modify: `go/observability-mcp-service/internal/workflows/types.go`
- Modify: `go/observability-mcp-service/internal/workflows/service.go`
- Modify: `go/observability-mcp-service/internal/workflows/service_test.go`

- [ ] **Step 1: Add failing workflow tests**

Add a fake history store to `service_test.go`:

```go
type fakeHistoryStore struct {
	snapshots []history.SnapshotInput
	err       error
}

func (f *fakeHistoryStore) Close() error { return nil }
func (f *fakeHistoryStore) Migrate(context.Context) error { return nil }
func (f *fakeHistoryStore) RecordSnapshot(_ context.Context, in history.SnapshotInput) (history.Snapshot, error) {
	f.snapshots = append(f.snapshots, in)
	if f.err != nil {
		return history.Snapshot{}, f.err
	}
	return history.Snapshot{ID: int64(len(f.snapshots)), Tool: in.Tool, Status: in.Status}, nil
}
func (f *fakeHistoryStore) ListIncidents(context.Context, history.ListFilter) ([]history.IncidentSummary, error) { return nil, f.err }
func (f *fakeHistoryStore) GetIncidentHistory(context.Context, string) (history.IncidentHistory, error) { return history.IncidentHistory{}, f.err }
func (f *fakeHistoryStore) AddIncidentNote(context.Context, history.AddNoteInput) (history.Event, error) { return history.Event{}, f.err }
func (f *fakeHistoryStore) GetSnapshot(context.Context, int64) (history.Snapshot, error) { return history.Snapshot{}, f.err }
```

Add tests:

```go
func TestInvestigateCheckoutPersistsWhenIncidentKeyProvided(t *testing.T)
func TestPersistenceFailureAddsBundleWarningButKeepsEvidence(t *testing.T)
func TestHistoryDisabledDoesNotPersist(t *testing.T)
```

Expected assertions:

```go
got := service.InvestigateCheckout(context.Background(), time.Minute, CaptureOptions{IncidentKey: "inc-1", IncidentTitle: "Checkout failures", Severity: "warning"})
if got.History == nil || got.History.SnapshotID == 0 {
	t.Fatalf("expected persisted snapshot metadata, got %+v", got.History)
}
if len(store.snapshots) != 1 {
	t.Fatalf("snapshots recorded = %d", len(store.snapshots))
}
```

- [ ] **Step 2: Run workflow tests and verify failure**

Run: `cd go/observability-mcp-service && go test ./internal/workflows`

Expected: FAIL because `CaptureOptions`, history fields, and new method signatures do not exist.

- [ ] **Step 3: Add workflow history types**

In `workflows/types.go`, add:

```go
type CaptureOptions struct {
	IncidentKey   string `json:"incident_key,omitempty"`
	IncidentTitle string `json:"incident_title,omitempty"`
	Severity      string `json:"severity,omitempty"`
	Service       string `json:"service,omitempty"`
	Persist       *bool  `json:"persist,omitempty"`
}

type HistoryMetadata struct {
	IncidentKey string `json:"incident_key,omitempty"`
	SnapshotID  int64  `json:"snapshot_id,omitempty"`
	Warning     string `json:"warning,omitempty"`
}
```

Add `History *HistoryMetadata `json:"history,omitempty"` to `EvidenceBundle`.

- [ ] **Step 4: Update workflow service constructor and methods**

Add fields to `Service`:

```go
historyStore history.Store
autoCapture  bool
```

Add option methods:

```go
func (s *Service) WithHistory(store history.Store, autoCapture bool) *Service {
	s.historyStore = store
	s.autoCapture = autoCapture
	return s
}
```

Change investigation method signatures to include `CaptureOptions` for tools that should persist:

```go
func (s *Service) InvestigateCheckout(ctx context.Context, window time.Duration, capture CaptureOptions) EvidenceBundle
```

Keep `SearchLogs` and `GetTrace` unchanged in issue 252. Explicit snapshot recording for those outputs goes through `record_evidence_snapshot`.

Add helper:

```go
func (s *Service) maybeRecordSnapshot(ctx context.Context, b *EvidenceBundle, capture CaptureOptions) {
	if s.historyStore == nil {
		return
	}
	shouldPersist := s.autoCapture || capture.IncidentKey != ""
	if capture.Persist != nil {
		shouldPersist = *capture.Persist
	}
	if !shouldPersist {
		return
	}
	bundleJSON, err := json.Marshal(b)
	if err != nil {
		b.History = &HistoryMetadata{IncidentKey: capture.IncidentKey, Warning: "marshal evidence bundle: " + err.Error()}
		return
	}
	critical, warnings := countFindings(b.Findings)
	snapshot, err := s.historyStore.RecordSnapshot(ctx, history.SnapshotInput{
		IncidentKey:    capture.IncidentKey,
		IncidentTitle:  capture.IncidentTitle,
		Severity:       capture.Severity,
		Service:        capture.Service,
		Tool:           b.Tool,
		WindowFrom:     b.Window.From,
		WindowTo:       b.Window.To,
		WindowDuration: b.Window.Duration,
		Status:         b.Status,
		Partial:        b.Partial,
		CriticalCount:  critical,
		WarningCount:   warnings,
		SignalCount:    len(b.Signals),
		LogCount:       len(b.Logs),
		TraceCount:     len(b.Traces),
		SourceStatuses: historySourceStatuses(b.Sources),
		BundleJSON:     bundleJSON,
	})
	if err != nil {
		b.History = &HistoryMetadata{IncidentKey: capture.IncidentKey, Warning: err.Error()}
		return
	}
	b.History = &HistoryMetadata{IncidentKey: capture.IncidentKey, SnapshotID: snapshot.ID}
}
```

Call this helper after `finalize`.

- [ ] **Step 5: Run workflow tests**

Run: `cd go/observability-mcp-service && go test ./internal/workflows -v`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add go/observability-mcp-service/internal/workflows
git commit -m "feat: capture observability evidence history"
```

### Task 5: Add History Comparison Workflow Methods

**Files:**
- Modify: `go/observability-mcp-service/internal/workflows/types.go`
- Modify: `go/observability-mcp-service/internal/workflows/service.go`
- Modify: `go/observability-mcp-service/internal/workflows/service_test.go`

- [ ] **Step 1: Write failing comparison tests**

Add tests:

```go
func TestCompareEvidenceSnapshotsReportsStatusAndCountDeltas(t *testing.T)
func TestCompareEvidenceSnapshotsReportsSourceAvailabilityChanges(t *testing.T)
func TestCompareEvidenceSnapshotsReportsSignalValueDeltas(t *testing.T)
```

Use two `history.Snapshot` values with `BundleJSON` containing minimal `EvidenceBundle` JSON. Assert result includes deltas for status, partial, sources, findings, logs, traces, and signal values by name.

- [ ] **Step 2: Run tests and verify failure**

Run: `cd go/observability-mcp-service && go test ./internal/workflows`

Expected: FAIL because comparison types and methods do not exist.

- [ ] **Step 3: Add comparison types**

In `workflows/types.go`, add:

```go
type EvidenceComparison struct {
	BaselineSnapshotID int64             `json:"baseline_snapshot_id"`
	CandidateSnapshotID int64            `json:"candidate_snapshot_id,omitempty"`
	StatusChange       string            `json:"status_change,omitempty"`
	PartialChange      string            `json:"partial_change,omitempty"`
	SourceChanges      []ComparisonDelta `json:"source_changes,omitempty"`
	SignalChanges      []ComparisonDelta `json:"signal_changes,omitempty"`
	CountChanges       []ComparisonDelta `json:"count_changes,omitempty"`
	Summary            []string          `json:"summary"`
}

type ComparisonDelta struct {
	Name      string `json:"name"`
	Before    string `json:"before"`
	After     string `json:"after"`
	Direction string `json:"direction"`
}
```

- [ ] **Step 4: Implement comparison**

Add method:

```go
func (s *Service) CompareEvidenceSnapshots(ctx context.Context, baselineID, candidateID int64) (EvidenceComparison, error)
```

Use `historyStore.GetSnapshot` for each snapshot, unmarshal `BundleJSON` into `EvidenceBundle`, compare by signal name, and populate summary strings such as:

```go
"status changed from warning to ok"
"log_count decreased from 12 to 2"
"source loki changed from error to ok"
```

- [ ] **Step 5: Run workflow tests**

Run: `cd go/observability-mcp-service && go test ./internal/workflows -v`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add go/observability-mcp-service/internal/workflows
git commit -m "feat: compare observability evidence snapshots"
```

### Task 6: Add MCP History Tools And Input Schemas

**Files:**
- Modify: `go/observability-mcp-service/internal/mcpserver/server.go`
- Modify: `go/observability-mcp-service/internal/mcpserver/server_test.go`

- [ ] **Step 1: Write failing MCP handler tests**

Add tests:

```go
func TestHistoryToolsAreRegistered(t *testing.T)
func TestWindowHandlerPassesCaptureOptions(t *testing.T)
func TestAddIncidentNoteHandlerRequiresIncidentKeyAndNote(t *testing.T)
func TestCompareEvidenceWindowsHandlerRequiresBaselineSnapshotID(t *testing.T)
```

Extend `fakeWorkflow` with fields for capture options and methods for:

```go
ListIncidents(context.Context, history.ListFilter) ([]history.IncidentSummary, error)
GetIncidentHistory(context.Context, string) (history.IncidentHistory, error)
AddIncidentNote(context.Context, history.AddNoteInput) (history.Event, error)
CompareEvidenceSnapshots(context.Context, int64, int64) (workflows.EvidenceComparison, error)
```

- [ ] **Step 2: Run MCP tests and verify failure**

Run: `cd go/observability-mcp-service && go test ./internal/mcpserver`

Expected: FAIL because interface methods, handlers, and schemas do not exist.

- [ ] **Step 3: Extend WorkflowService interface and register tools**

Add methods to `WorkflowService` and `New`:

```go
addTool(srv, "list_incidents", "List persisted observability incidents.", listIncidentsSchema(), listIncidentsHandler(service))
addTool(srv, "get_incident_history", "Return incident timeline and evidence summaries.", incidentHistorySchema(), incidentHistoryHandler(service))
addTool(srv, "add_incident_note", "Append an incident note and optionally transition status.", addIncidentNoteSchema(), addIncidentNoteHandler(service))
addTool(srv, "compare_evidence_windows", "Compare two persisted observability evidence snapshots.", compareEvidenceWindowsSchema(), compareEvidenceWindowsHandler(service))
```

- [ ] **Step 4: Update investigation schemas**

Replace simple `windowInput` with:

```go
type investigationInput struct {
	Window        string `json:"window,omitempty"`
	IncidentKey   string `json:"incident_key,omitempty"`
	IncidentTitle string `json:"incident_title,omitempty"`
	Severity      string `json:"severity,omitempty"`
	Service       string `json:"service,omitempty"`
	Persist       *bool  `json:"persist,omitempty"`
}
```

Use this in `get_system_health`, `investigate_checkout`, `investigate_ai_pipeline`, `investigate_eval_run`, `investigate_streaming_analytics`, and `get_service_evidence` where the workflow method supports capture.

- [ ] **Step 5: Implement handlers**

Handlers should validate:

```go
if in.IncidentKey == "" { return toolError("incident_key is required"), nil }
if in.Note == "" { return toolError("note is required"), nil }
if in.BaselineSnapshotID <= 0 { return toolError("baseline_snapshot_id is required"), nil }
```

Return `jsonResult(result)` for successful calls and `toolError(err.Error())` for workflow errors.

- [ ] **Step 6: Run MCP tests**

Run: `cd go/observability-mcp-service && go test ./internal/mcpserver -v`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add go/observability-mcp-service/internal/mcpserver
git commit -m "feat: expose observability incident history tools"
```

### Task 7: Wire Store In Main And Document Configuration

**Files:**
- Modify: `go/observability-mcp-service/cmd/observability-mcp/main.go`
- Modify: `go/observability-mcp-service/cmd/observability-mcp/main_test.go`
- Modify: `go/observability-mcp-service/README.md`

- [ ] **Step 1: Write failing startup tests**

In `cmd/observability-mcp/main_test.go`, add tests:

```go
func TestRunOpensHistoryStoreWhenEnabled(t *testing.T)
func TestRunSkipsHistoryStoreWhenDisabled(t *testing.T)
```

Set:

```go
t.Setenv("OBS_HISTORY_DB_PATH", filepath.Join(t.TempDir(), "history.db"))
t.Setenv("OBS_HISTORY_ENABLED", "true")
```

Assert startup succeeds and the DB file exists when enabled. Assert startup succeeds without creating the DB when disabled.

- [ ] **Step 2: Run main tests and verify failure**

Run: `cd go/observability-mcp-service && go test ./cmd/observability-mcp`

Expected: FAIL because main does not open the history store.

- [ ] **Step 3: Wire history store**

In `main.go`, import `internal/history`, open/migrate when `cfg.HistoryEnabled`, defer close, and call:

```go
service := workflows.NewService(prom, loki, jaeger, cfg.MaxLogLines)
if cfg.HistoryEnabled {
	historyDB, err := history.Open(cfg.HistoryDBPath)
	if err != nil {
		return fmt.Errorf("history open: %w", err)
	}
	defer historyDB.Close()
	if err := historyDB.Migrate(ctx); err != nil {
		return fmt.Errorf("history migrate: %w", err)
	}
	service.WithHistory(historyDB, cfg.HistoryAutoCapture)
}
```

- [ ] **Step 4: Update README**

Add configuration rows for:

```markdown
| `OBS_HISTORY_ENABLED` | `true` | Enables local SQLite incident history. |
| `OBS_HISTORY_DB_PATH` | `~/.codex/data/observability-mcp/history.db` | Local SQLite history database path. |
| `OBS_HISTORY_AUTO_CAPTURE` | `false` | Persist investigation evidence even without `incident_key`. |
```

Add tool bullets for new history tools.

- [ ] **Step 5: Run main tests**

Run: `cd go/observability-mcp-service && go test ./cmd/observability-mcp -v`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add go/observability-mcp-service/cmd/observability-mcp go/observability-mcp-service/README.md
git commit -m "feat: initialize observability history store"
```

### Task 8: Final Verification

**Files:**
- No new files.

- [ ] **Step 1: Run focused service tests**

Run:

```bash
cd go/observability-mcp-service && go test ./... -v
```

Expected: PASS.

- [ ] **Step 2: Run Go preflight**

Run:

```bash
make preflight-go
```

Expected: PASS. If blocked by local toolchain or platform limits, capture the exact blocker and leave remaining verification to CI.

- [ ] **Step 3: Inspect final diff**

Run:

```bash
git status --short
git diff --stat
git diff -- go/observability-mcp-service
```

Expected: only issue 252 files changed; no unrelated changes.

- [ ] **Step 4: Final commit if needed**

If verification fixes created additional changes:

```bash
git add go/observability-mcp-service
git commit -m "test: verify observability incident history"
```

Expected: working tree contains no uncommitted issue 252 implementation changes.
