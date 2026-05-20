package history

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const defaultListLimit = 100

type DB struct {
	db *sql.DB
}

func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create history db dir: %w", err)
	}
	sqlDB, err := sql.Open("sqlite", sqliteDSN(path))
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

func (d *DB) Migrate(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS incidents (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			incident_key TEXT NOT NULL,
			title TEXT NOT NULL,
			status TEXT NOT NULL,
			severity TEXT NOT NULL DEFAULT '',
			service TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS timeline_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			incident_id INTEGER NOT NULL,
			event_type TEXT NOT NULL,
			summary TEXT NOT NULL,
			details TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			FOREIGN KEY (incident_id) REFERENCES incidents(id)
		)`,
		`CREATE TABLE IF NOT EXISTS evidence_snapshots (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			incident_id INTEGER,
			timeline_event_id INTEGER,
			tool TEXT NOT NULL,
			window_from TEXT NOT NULL,
			window_to TEXT NOT NULL,
			window_duration TEXT NOT NULL,
			status TEXT NOT NULL,
			partial INTEGER NOT NULL,
			critical_count INTEGER NOT NULL,
			warning_count INTEGER NOT NULL,
			signal_count INTEGER NOT NULL,
			log_count INTEGER NOT NULL,
			trace_count INTEGER NOT NULL,
			source_statuses_json TEXT NOT NULL,
			bundle_json BLOB NOT NULL,
			created_at TEXT NOT NULL,
			FOREIGN KEY (incident_id) REFERENCES incidents(id),
			FOREIGN KEY (timeline_event_id) REFERENCES timeline_events(id)
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS incidents_key_unique ON incidents(incident_key)`,
		`CREATE INDEX IF NOT EXISTS timeline_events_incident_created_idx ON timeline_events(incident_id, created_at, id)`,
		`CREATE INDEX IF NOT EXISTS evidence_snapshots_tool_created_idx ON evidence_snapshots(tool, created_at)`,
	}
	for _, statement := range statements {
		if _, err := d.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migrate history sqlite: %w", err)
		}
	}
	return nil
}

func (d *DB) RecordSnapshot(ctx context.Context, input SnapshotInput) (Snapshot, error) {
	if input.IncidentKey == "" {
		return Snapshot{}, errors.New("incident key is required")
	}
	now := time.Now().UTC()
	sourceStatusesJSON, err := json.Marshal(input.SourceStatuses)
	if err != nil {
		return Snapshot{}, fmt.Errorf("marshal source statuses: %w", err)
	}

	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return Snapshot{}, fmt.Errorf("begin record snapshot transaction: %w", err)
	}
	defer rollback(tx)

	incidentID, err := upsertIncident(ctx, tx, IncidentUpsert{
		Key:      input.IncidentKey,
		Title:    input.IncidentTitle,
		Status:   StatusInvestigating,
		Severity: input.Severity,
		Service:  input.Service,
	}, now)
	if err != nil {
		return Snapshot{}, err
	}

	eventResult, err := tx.ExecContext(ctx, `
		INSERT INTO timeline_events (incident_id, event_type, summary, details, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		incidentID,
		EventEvidenceSnapshot,
		fmt.Sprintf("%s snapshot recorded", input.Tool),
		"",
		formatTime(now),
	)
	if err != nil {
		return Snapshot{}, fmt.Errorf("insert evidence snapshot event: %w", err)
	}
	eventID, err := eventResult.LastInsertId()
	if err != nil {
		return Snapshot{}, fmt.Errorf("read evidence snapshot event id: %w", err)
	}

	snapshotResult, err := tx.ExecContext(ctx, `
		INSERT INTO evidence_snapshots (
			incident_id, timeline_event_id, tool, window_from, window_to, window_duration,
			status, partial, critical_count, warning_count, signal_count, log_count, trace_count,
			source_statuses_json, bundle_json, created_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		incidentID,
		eventID,
		input.Tool,
		formatTime(input.WindowFrom),
		formatTime(input.WindowTo),
		input.WindowDuration,
		snapshotStatus(input.Status),
		boolInt(input.Partial),
		input.CriticalCount,
		input.WarningCount,
		input.SignalCount,
		input.LogCount,
		input.TraceCount,
		string(sourceStatusesJSON),
		input.BundleJSON,
		formatTime(now),
	)
	if err != nil {
		return Snapshot{}, fmt.Errorf("insert evidence snapshot: %w", err)
	}
	snapshotID, err := snapshotResult.LastInsertId()
	if err != nil {
		return Snapshot{}, fmt.Errorf("read evidence snapshot id: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Snapshot{}, fmt.Errorf("commit record snapshot transaction: %w", err)
	}

	timelineEventID := eventID
	return Snapshot{
		ID:              snapshotID,
		IncidentID:      &incidentID,
		TimelineEventID: &timelineEventID,
		Tool:            input.Tool,
		WindowFrom:      input.WindowFrom,
		WindowTo:        input.WindowTo,
		WindowDuration:  input.WindowDuration,
		Status:          snapshotStatus(input.Status),
		Partial:         input.Partial,
		CriticalCount:   input.CriticalCount,
		WarningCount:    input.WarningCount,
		SignalCount:     input.SignalCount,
		LogCount:        input.LogCount,
		TraceCount:      input.TraceCount,
		SourceStatuses:  input.SourceStatuses,
		BundleJSON:      append([]byte(nil), input.BundleJSON...),
		CreatedAt:       now,
	}, nil
}

func (d *DB) ListIncidents(ctx context.Context, filter ListFilter) ([]IncidentSummary, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = defaultListLimit
	}
	if limit > defaultListLimit {
		limit = defaultListLimit
	}

	var conditions []string
	var args []any
	if filter.Status != "" {
		conditions = append(conditions, "i.status = ?")
		args = append(args, filter.Status)
	}
	if filter.Service != "" {
		conditions = append(conditions, "i.service = ?")
		args = append(args, filter.Service)
	}
	if filter.Severity != "" {
		conditions = append(conditions, "i.severity = ?")
		args = append(args, filter.Severity)
	}

	query := `
		SELECT i.id, i.incident_key, i.title, i.status, i.severity, i.service, i.created_at, i.updated_at,
			COUNT(DISTINCT s.id) AS snapshot_count,
			COALESCE(MAX(e.created_at), i.updated_at) AS last_event_at
		FROM incidents i
		LEFT JOIN evidence_snapshots s ON s.incident_id = i.id
		LEFT JOIN timeline_events e ON e.incident_id = i.id`
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += `
		GROUP BY i.id
		ORDER BY last_event_at DESC, i.id DESC
		LIMIT ?`
	args = append(args, limit)

	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list incidents: %w", err)
	}
	defer rows.Close()

	var summaries []IncidentSummary
	for rows.Next() {
		var summary IncidentSummary
		var createdAt, updatedAt, lastEventAt string
		if err := rows.Scan(
			&summary.Incident.ID,
			&summary.Incident.Key,
			&summary.Incident.Title,
			&summary.Incident.Status,
			&summary.Incident.Severity,
			&summary.Incident.Service,
			&createdAt,
			&updatedAt,
			&summary.SnapshotCount,
			&lastEventAt,
		); err != nil {
			return nil, fmt.Errorf("scan incident summary: %w", err)
		}
		var err error
		summary.Incident.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		summary.Incident.UpdatedAt, err = parseTime(updatedAt)
		if err != nil {
			return nil, err
		}
		summary.LastEventAt, err = parseTime(lastEventAt)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate incident summaries: %w", err)
	}
	return summaries, nil
}

func (d *DB) GetIncidentHistory(ctx context.Context, key string) (IncidentHistory, error) {
	incident, err := d.getIncidentByKey(ctx, key)
	if err != nil {
		return IncidentHistory{}, err
	}

	rows, err := d.db.QueryContext(ctx, `
		SELECT id, incident_id, event_type, summary, details, created_at
		FROM timeline_events
		WHERE incident_id = ?
		ORDER BY created_at, id`, incident.ID)
	if err != nil {
		return IncidentHistory{}, fmt.Errorf("query incident timeline: %w", err)
	}
	defer rows.Close()

	history := IncidentHistory{Incident: incident}
	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			return IncidentHistory{}, err
		}
		if event.Type == EventEvidenceSnapshot {
			snapshot, err := d.getSnapshotByEvent(ctx, event.ID)
			if err != nil {
				return IncidentHistory{}, err
			}
			event.Snapshot = &snapshot
		}
		history.Events = append(history.Events, event)
	}
	if err := rows.Err(); err != nil {
		return IncidentHistory{}, fmt.Errorf("iterate incident timeline: %w", err)
	}
	return history, nil
}

func (d *DB) AddIncidentNote(ctx context.Context, input AddNoteInput) (Event, error) {
	if input.IncidentKey == "" {
		return Event{}, errors.New("incident key is required")
	}
	if input.Note == "" {
		return Event{}, errors.New("note is required")
	}
	if input.Status != "" && !ValidStatus(input.Status) {
		return Event{}, fmt.Errorf("invalid incident status %q", input.Status)
	}

	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return Event{}, fmt.Errorf("begin add incident note transaction: %w", err)
	}
	defer rollback(tx)

	incident, err := getIncidentByKeyTx(ctx, tx, input.IncidentKey)
	if err != nil {
		return Event{}, err
	}
	now := time.Now().UTC()
	event, err := insertTimelineEvent(ctx, tx, incident.ID, EventNoteAdded, input.Note, "", now)
	if err != nil {
		return Event{}, err
	}

	if input.Status != "" {
		if _, err := tx.ExecContext(ctx, `
			UPDATE incidents
			SET status = ?, updated_at = ?
			WHERE id = ?`, input.Status, formatTime(now), incident.ID); err != nil {
			return Event{}, fmt.Errorf("update incident status: %w", err)
		}
		statusSummary, statusDetails := statusEventText(incident.Status, input.Status)
		if _, err := insertTimelineEvent(
			ctx,
			tx,
			incident.ID,
			EventStatusChanged,
			statusSummary,
			statusDetails,
			now,
		); err != nil {
			return Event{}, err
		}
	} else {
		if _, err := tx.ExecContext(ctx, `
			UPDATE incidents
			SET updated_at = ?
			WHERE id = ?`, formatTime(now), incident.ID); err != nil {
			return Event{}, fmt.Errorf("update incident timestamp: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return Event{}, fmt.Errorf("commit add incident note transaction: %w", err)
	}
	return event, nil
}

func (d *DB) GetSnapshot(ctx context.Context, id int64) (Snapshot, error) {
	row := d.db.QueryRowContext(ctx, snapshotSelectSQL()+` WHERE s.id = ?`, id)
	snapshot, err := scanSnapshot(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Snapshot{}, ErrNotFound
		}
		return Snapshot{}, err
	}
	return snapshot, nil
}

func upsertIncident(ctx context.Context, tx *sql.Tx, incident IncidentUpsert, now time.Time) (int64, error) {
	status := NormalizeStatus(incident.Status)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO incidents (incident_key, title, status, severity, service, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(incident_key) DO UPDATE SET
			title = excluded.title,
			severity = excluded.severity,
			service = excluded.service,
			updated_at = excluded.updated_at`,
		incident.Key,
		incident.Title,
		status,
		incident.Severity,
		incident.Service,
		formatTime(now),
		formatTime(now),
	); err != nil {
		return 0, fmt.Errorf("upsert incident: %w", err)
	}

	var id int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM incidents WHERE incident_key = ?`, incident.Key).Scan(&id); err != nil {
		return 0, fmt.Errorf("select incident id: %w", err)
	}
	return id, nil
}

func (d *DB) getIncidentByKey(ctx context.Context, key string) (Incident, error) {
	row := d.db.QueryRowContext(ctx, `
		SELECT id, incident_key, title, status, severity, service, created_at, updated_at
		FROM incidents
		WHERE incident_key = ?`, key)
	incident, err := scanIncident(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Incident{}, ErrNotFound
		}
		return Incident{}, err
	}
	return incident, nil
}

func getIncidentByKeyTx(ctx context.Context, tx *sql.Tx, key string) (Incident, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT id, incident_key, title, status, severity, service, created_at, updated_at
		FROM incidents
		WHERE incident_key = ?`, key)
	incident, err := scanIncident(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Incident{}, ErrNotFound
		}
		return Incident{}, err
	}
	return incident, nil
}

func insertTimelineEvent(ctx context.Context, tx *sql.Tx, incidentID int64, eventType, summary, details string, createdAt time.Time) (Event, error) {
	result, err := tx.ExecContext(ctx, `
		INSERT INTO timeline_events (incident_id, event_type, summary, details, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		incidentID,
		eventType,
		summary,
		details,
		formatTime(createdAt),
	)
	if err != nil {
		return Event{}, fmt.Errorf("insert timeline event: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Event{}, fmt.Errorf("read timeline event id: %w", err)
	}
	return Event{
		ID:         id,
		IncidentID: incidentID,
		Type:       eventType,
		Summary:    summary,
		Details:    details,
		CreatedAt:  createdAt,
	}, nil
}

func (d *DB) getSnapshotByEvent(ctx context.Context, eventID int64) (Snapshot, error) {
	row := d.db.QueryRowContext(ctx, snapshotSelectSQL()+` WHERE s.timeline_event_id = ?`, eventID)
	snapshot, err := scanSnapshot(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Snapshot{}, ErrNotFound
		}
		return Snapshot{}, err
	}
	return snapshot, nil
}

func snapshotSelectSQL() string {
	return `
		SELECT s.id, s.incident_id, s.timeline_event_id, s.tool, s.window_from, s.window_to,
			s.window_duration, s.status, s.partial, s.critical_count, s.warning_count,
			s.signal_count, s.log_count, s.trace_count, s.source_statuses_json,
			s.bundle_json, s.created_at
		FROM evidence_snapshots s`
}

type scanner interface {
	Scan(dest ...any) error
}

func scanSnapshot(row scanner) (Snapshot, error) {
	var snapshot Snapshot
	var incidentID, timelineEventID sql.NullInt64
	var windowFrom, windowTo, createdAt, sourceStatusesJSON string
	var partial int
	if err := row.Scan(
		&snapshot.ID,
		&incidentID,
		&timelineEventID,
		&snapshot.Tool,
		&windowFrom,
		&windowTo,
		&snapshot.WindowDuration,
		&snapshot.Status,
		&partial,
		&snapshot.CriticalCount,
		&snapshot.WarningCount,
		&snapshot.SignalCount,
		&snapshot.LogCount,
		&snapshot.TraceCount,
		&sourceStatusesJSON,
		&snapshot.BundleJSON,
		&createdAt,
	); err != nil {
		return Snapshot{}, fmt.Errorf("scan snapshot: %w", err)
	}
	if incidentID.Valid {
		snapshot.IncidentID = &incidentID.Int64
	}
	if timelineEventID.Valid {
		snapshot.TimelineEventID = &timelineEventID.Int64
	}
	var err error
	snapshot.WindowFrom, err = parseTime(windowFrom)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot.WindowTo, err = parseTime(windowTo)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot.Partial = partial != 0
	if err := json.Unmarshal([]byte(sourceStatusesJSON), &snapshot.SourceStatuses); err != nil {
		return Snapshot{}, fmt.Errorf("unmarshal source statuses: %w", err)
	}
	return snapshot, nil
}

func scanIncident(row scanner) (Incident, error) {
	var incident Incident
	var createdAt, updatedAt string
	if err := row.Scan(
		&incident.ID,
		&incident.Key,
		&incident.Title,
		&incident.Status,
		&incident.Severity,
		&incident.Service,
		&createdAt,
		&updatedAt,
	); err != nil {
		return Incident{}, fmt.Errorf("scan incident: %w", err)
	}
	var err error
	incident.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return Incident{}, err
	}
	incident.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return Incident{}, err
	}
	return incident, nil
}

func scanEvent(row scanner) (Event, error) {
	var event Event
	var createdAt string
	if err := row.Scan(
		&event.ID,
		&event.IncidentID,
		&event.Type,
		&event.Summary,
		&event.Details,
		&createdAt,
	); err != nil {
		return Event{}, fmt.Errorf("scan timeline event: %w", err)
	}
	var err error
	event.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return Event{}, err
	}
	return event, nil
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse history timestamp %q: %w", value, err)
	}
	return parsed, nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func sqliteDSN(path string) string {
	u := url.URL{Scheme: "file", Path: path}
	q := u.Query()
	q.Add("_pragma", "foreign_keys(1)")
	u.RawQuery = q.Encode()
	return u.String()
}

func statusEventText(previous, next string) (string, string) {
	if previous == next {
		return fmt.Sprintf("Status reaffirmed as %s", next), "Requested status matched the current incident status."
	}
	return fmt.Sprintf("Status changed from %s to %s", previous, next), ""
}

func snapshotStatus(status string) string {
	if status == "" {
		return "unknown"
	}
	return status
}

func rollback(tx *sql.Tx) {
	_ = tx.Rollback()
}
