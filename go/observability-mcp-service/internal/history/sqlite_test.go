package history

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

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

func TestSQLiteMigrateIsIdempotent(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("second Migrate() error = %v", err)
	}
}

func TestSQLiteForeignKeysAreEnforced(t *testing.T) {
	db := openTestDB(t)

	_, err := db.db.ExecContext(context.Background(), `
		INSERT INTO timeline_events (incident_id, event_type, summary, details, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		int64(404),
		EventNoteAdded,
		"orphan event",
		"",
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err == nil {
		t.Fatal("insert orphan timeline event error = nil, want foreign key constraint error")
	}
}

func TestRecordSnapshotCreatesIncidentTimelineAndImmutableSnapshot(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	snapshot, err := db.RecordSnapshot(ctx, sampleSnapshot("checkout-warning"))
	if err != nil {
		t.Fatalf("RecordSnapshot() error = %v", err)
	}
	if snapshot.ID == 0 {
		t.Fatal("RecordSnapshot() returned snapshot with ID 0")
	}
	if snapshot.IncidentID == nil || *snapshot.IncidentID == 0 {
		t.Fatalf("RecordSnapshot() IncidentID = %v, want populated", snapshot.IncidentID)
	}
	if snapshot.TimelineEventID == nil || *snapshot.TimelineEventID == 0 {
		t.Fatalf("RecordSnapshot() TimelineEventID = %v, want populated", snapshot.TimelineEventID)
	}
	if snapshot.Status != "warning" {
		t.Fatalf("RecordSnapshot() Status = %q, want warning", snapshot.Status)
	}
	if string(snapshot.BundleJSON) != `{"tool":"investigate_checkout","status":"warning"}` {
		t.Fatalf("RecordSnapshot() BundleJSON = %s", snapshot.BundleJSON)
	}
	if len(snapshot.SourceStatuses) != 1 || snapshot.SourceStatuses[0].Name != "prometheus" || snapshot.SourceStatuses[0].Status != "ok" {
		t.Fatalf("RecordSnapshot() SourceStatuses = %#v, want prometheus ok", snapshot.SourceStatuses)
	}

	if _, err := db.AddIncidentNote(ctx, AddNoteInput{
		IncidentKey: "checkout-warning",
		Note:        "Mitigated by scaling order workers.",
		Status:      StatusMitigated,
	}); err != nil {
		t.Fatalf("AddIncidentNote() error = %v", err)
	}

	gotSnapshot, err := db.GetSnapshot(ctx, snapshot.ID)
	if err != nil {
		t.Fatalf("GetSnapshot() error = %v", err)
	}
	if gotSnapshot.Status != "warning" {
		t.Fatalf("GetSnapshot() Status = %q, want original warning after incident status transition", gotSnapshot.Status)
	}
	if gotSnapshot.WarningCount != 1 || gotSnapshot.SignalCount != 2 || gotSnapshot.LogCount != 3 {
		t.Fatalf("GetSnapshot() counts = warnings:%d signals:%d logs:%d, want 1/2/3", gotSnapshot.WarningCount, gotSnapshot.SignalCount, gotSnapshot.LogCount)
	}
	if len(gotSnapshot.SourceStatuses) != 1 || gotSnapshot.SourceStatuses[0].Name != "prometheus" || gotSnapshot.SourceStatuses[0].Status != "ok" {
		t.Fatalf("GetSnapshot() SourceStatuses = %#v, want persisted prometheus ok", gotSnapshot.SourceStatuses)
	}

	history, err := db.GetIncidentHistory(ctx, "checkout-warning")
	if err != nil {
		t.Fatalf("GetIncidentHistory() error = %v", err)
	}
	if history.Incident.Status != StatusMitigated {
		t.Fatalf("GetIncidentHistory() Incident.Status = %q, want %q", history.Incident.Status, StatusMitigated)
	}
	if len(history.Events) != 3 {
		t.Fatalf("GetIncidentHistory() events = %d, want 3", len(history.Events))
	}
	if history.Events[0].Type != EventEvidenceSnapshot {
		t.Fatalf("first event type = %q, want %q", history.Events[0].Type, EventEvidenceSnapshot)
	}
	if history.Events[0].Snapshot == nil || history.Events[0].Snapshot.ID != snapshot.ID {
		t.Fatalf("first event snapshot = %#v, want snapshot ID %d", history.Events[0].Snapshot, snapshot.ID)
	}
	if history.Events[1].Type != EventNoteAdded {
		t.Fatalf("second event type = %q, want %q", history.Events[1].Type, EventNoteAdded)
	}
	if history.Events[2].Type != EventStatusChanged {
		t.Fatalf("third event type = %q, want %q", history.Events[2].Type, EventStatusChanged)
	}
}

func TestListIncidentsFiltersByStatusServiceSeverity(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	inputs := []SnapshotInput{
		sampleSnapshot("checkout-warning"),
		sampleSnapshot("payment-critical"),
		sampleSnapshot("cart-warning"),
	}
	inputs[1].IncidentTitle = "Payment failures"
	inputs[1].Severity = "critical"
	inputs[1].Service = "go-payment-service"
	inputs[1].Status = StatusInvestigating
	inputs[2].IncidentTitle = "Cart failures"
	inputs[2].Service = "go-cart-service"
	inputs[2].Status = StatusMitigated

	for _, input := range inputs {
		if _, err := db.RecordSnapshot(ctx, input); err != nil {
			t.Fatalf("RecordSnapshot(%q) error = %v", input.IncidentKey, err)
		}
	}
	if _, err := db.RecordSnapshot(ctx, sampleSnapshot("checkout-warning")); err != nil {
		t.Fatalf("second RecordSnapshot(checkout-warning) error = %v", err)
	}

	got, err := db.ListIncidents(ctx, ListFilter{
		Status:   "warning",
		Service:  "go-order-service",
		Severity: "warning",
	})
	if err != nil {
		t.Fatalf("ListIncidents() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListIncidents() returned %d incidents, want 1", len(got))
	}
	if got[0].Incident.Key != "checkout-warning" {
		t.Fatalf("ListIncidents()[0].Incident.Key = %q, want checkout-warning", got[0].Incident.Key)
	}
	if got[0].SnapshotCount != 2 {
		t.Fatalf("ListIncidents()[0].SnapshotCount = %d, want 2", got[0].SnapshotCount)
	}
	if got[0].LastEventAt.IsZero() {
		t.Fatal("ListIncidents()[0].LastEventAt is zero")
	}

	got, err = db.ListIncidents(ctx, ListFilter{Service: "go-payment-service", Severity: "critical"})
	if err != nil {
		t.Fatalf("ListIncidents(payment critical) error = %v", err)
	}
	if len(got) != 1 || got[0].Incident.Key != "payment-critical" {
		t.Fatalf("ListIncidents(payment critical) = %#v, want payment-critical only", got)
	}

	got, err = db.ListIncidents(ctx, ListFilter{Limit: -1})
	if err != nil {
		t.Fatalf("ListIncidents(default limit) error = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("ListIncidents(default limit) returned %d incidents, want 3", len(got))
	}
}

func TestAddIncidentNoteCanTransitionStatus(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if _, err := db.RecordSnapshot(ctx, sampleSnapshot("checkout-warning")); err != nil {
		t.Fatalf("RecordSnapshot() error = %v", err)
	}

	event, err := db.AddIncidentNote(ctx, AddNoteInput{
		IncidentKey: "checkout-warning",
		Note:        "Rollback completed.",
		Status:      StatusResolved,
	})
	if err != nil {
		t.Fatalf("AddIncidentNote() error = %v", err)
	}
	if event.Type != EventNoteAdded {
		t.Fatalf("AddIncidentNote() event type = %q, want %q", event.Type, EventNoteAdded)
	}
	if event.Summary != "Rollback completed." {
		t.Fatalf("AddIncidentNote() summary = %q, want note text", event.Summary)
	}

	history, err := db.GetIncidentHistory(ctx, "checkout-warning")
	if err != nil {
		t.Fatalf("GetIncidentHistory() error = %v", err)
	}
	if history.Incident.Status != StatusResolved {
		t.Fatalf("incident status = %q, want %q", history.Incident.Status, StatusResolved)
	}
	if len(history.Events) != 3 {
		t.Fatalf("events = %d, want evidence_snapshot, note_added, status_changed", len(history.Events))
	}
	if history.Events[2].Type != EventStatusChanged {
		t.Fatalf("last event type = %q, want %q", history.Events[2].Type, EventStatusChanged)
	}
}

func TestAddIncidentNoteRecordsStatusChangedWhenStatusIsNonEmptyAndUnchanged(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if _, err := db.RecordSnapshot(ctx, sampleSnapshot("checkout-warning")); err != nil {
		t.Fatalf("RecordSnapshot() error = %v", err)
	}

	if _, err := db.AddIncidentNote(ctx, AddNoteInput{
		IncidentKey: "checkout-warning",
		Note:        "Still investigating.",
		Status:      "warning",
	}); err != nil {
		t.Fatalf("AddIncidentNote() error = %v", err)
	}

	history, err := db.GetIncidentHistory(ctx, "checkout-warning")
	if err != nil {
		t.Fatalf("GetIncidentHistory() error = %v", err)
	}
	if history.Incident.Status != "warning" {
		t.Fatalf("incident status = %q, want warning", history.Incident.Status)
	}
	if len(history.Events) != 3 {
		t.Fatalf("events = %d, want evidence_snapshot, note_added, status_changed", len(history.Events))
	}
	if history.Events[2].Type != EventStatusChanged {
		t.Fatalf("last event type = %q, want %q", history.Events[2].Type, EventStatusChanged)
	}
	if history.Events[2].Summary != "Status reaffirmed as warning" {
		t.Fatalf("last event summary = %q, want reaffirmed status", history.Events[2].Summary)
	}
}

func TestGetSnapshotReturnsErrNotFound(t *testing.T) {
	db := openTestDB(t)

	_, err := db.GetSnapshot(context.Background(), 404)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetSnapshot() error = %v, want ErrNotFound", err)
	}
}
