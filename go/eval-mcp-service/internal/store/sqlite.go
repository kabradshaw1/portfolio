package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("not found")

type DB struct {
	db *sql.DB
}

type Experiment struct {
	ID             int64      `json:"id"`
	Name           string     `json:"name"`
	DatasetID      string     `json:"dataset_id"`
	Collection     string     `json:"collection"`
	BaselineEvalID string     `json:"baseline_eval_id,omitempty"`
	FocusMetric    string     `json:"focus_metric"`
	Hypothesis     string     `json:"hypothesis,omitempty"`
	Notes          string     `json:"notes,omitempty"`
	Conclusion     string     `json:"conclusion,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	Runs           []RunLabel `json:"runs,omitempty"`
}

type RunLabel struct {
	Label     string    `json:"label"`
	EvalID    string    `json:"eval_id"`
	Notes     string    `json:"notes,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateExperimentInput struct {
	Name           string `json:"name"`
	DatasetID      string `json:"dataset_id"`
	Collection     string `json:"collection"`
	BaselineEvalID string `json:"baseline_eval_id,omitempty"`
	FocusMetric    string `json:"focus_metric"`
	Hypothesis     string `json:"hypothesis,omitempty"`
	Notes          string `json:"notes,omitempty"`
}

func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	configureSQLitePool(sqlDB)
	return &DB{db: sqlDB}, nil
}

func OpenSQL(db *sql.DB) *DB {
	configureSQLitePool(db)
	return &DB{db: db}
}

func (d *DB) Close() error { return d.db.Close() }

func configureSQLitePool(db *sql.DB) {
	// SQLite PRAGMA foreign_keys is scoped to a connection. Keep this local
	// metadata store on one pooled connection so migration enables FK behavior
	// for all later operations.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
}

func (d *DB) Migrate(ctx context.Context) error {
	if _, err := d.db.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		return fmt.Errorf("enable sqlite foreign keys: %w", err)
	}
	if _, err := d.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS experiments (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE,
	dataset_id TEXT NOT NULL,
	collection TEXT NOT NULL DEFAULT 'documents',
	baseline_eval_id TEXT NOT NULL DEFAULT '',
	focus_metric TEXT NOT NULL DEFAULT 'context_precision',
	hypothesis TEXT NOT NULL DEFAULT '',
	notes TEXT NOT NULL DEFAULT '',
	conclusion TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS experiment_runs (
	experiment_id INTEGER NOT NULL REFERENCES experiments(id) ON DELETE CASCADE,
	label TEXT NOT NULL,
	eval_id TEXT NOT NULL,
	notes TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (experiment_id, label)
);
`); err != nil {
		return fmt.Errorf("migrate sqlite: %w", err)
	}
	return nil
}

func (d *DB) CreateExperiment(ctx context.Context, in CreateExperimentInput) (int64, error) {
	collection := in.Collection
	if collection == "" {
		collection = "documents"
	}
	focusMetric := in.FocusMetric
	if focusMetric == "" {
		focusMetric = "context_precision"
	}

	res, err := d.db.ExecContext(ctx, `
INSERT INTO experiments (
	name,
	dataset_id,
	collection,
	baseline_eval_id,
	focus_metric,
	hypothesis,
	notes
) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		in.Name,
		in.DatasetID,
		collection,
		in.BaselineEvalID,
		focusMetric,
		in.Hypothesis,
		in.Notes,
	)
	if err != nil {
		return 0, fmt.Errorf("create experiment: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get experiment id: %w", err)
	}
	return id, nil
}

func (d *DB) ListExperiments(ctx context.Context) ([]Experiment, error) {
	rows, err := d.db.QueryContext(ctx, `
SELECT id, name, dataset_id, collection, baseline_eval_id, focus_metric, hypothesis, notes, conclusion, created_at, updated_at
FROM experiments
ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("list experiments: %w", err)
	}
	defer rows.Close()

	var experiments []Experiment
	for rows.Next() {
		var exp Experiment
		if err := scanExperiment(rows, &exp); err != nil {
			return nil, err
		}
		experiments = append(experiments, exp)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list experiments rows: %w", err)
	}
	return experiments, nil
}

func (d *DB) GetExperiment(ctx context.Context, id int64) (Experiment, error) {
	var exp Experiment
	row := d.db.QueryRowContext(ctx, `
SELECT id, name, dataset_id, collection, baseline_eval_id, focus_metric, hypothesis, notes, conclusion, created_at, updated_at
FROM experiments
WHERE id = ?`, id)
	if err := scanExperiment(row, &exp); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Experiment{}, fmt.Errorf("experiment %d: %w", id, ErrNotFound)
		}
		return Experiment{}, err
	}

	runs, err := d.listRuns(ctx, id)
	if err != nil {
		return Experiment{}, err
	}
	exp.Runs = runs
	return exp, nil
}

func (d *DB) AttachRun(ctx context.Context, experimentID int64, label, evalID, notes string) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin attach run: %w", err)
	}
	defer tx.Rollback()

	var exists int
	err = tx.QueryRowContext(ctx, `SELECT 1 FROM experiments WHERE id = ?`, experimentID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("experiment %d: %w", experimentID, ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("load experiment: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO experiment_runs (experiment_id, label, eval_id, notes)
VALUES (?, ?, ?, ?)
ON CONFLICT(experiment_id, label) DO UPDATE SET
	eval_id = excluded.eval_id,
	notes = excluded.notes,
	created_at = CURRENT_TIMESTAMP`,
		experimentID,
		label,
		evalID,
		notes,
	); err != nil {
		return fmt.Errorf("attach run: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE experiments
SET updated_at = CURRENT_TIMESTAMP
WHERE id = ?`, experimentID); err != nil {
		return fmt.Errorf("touch experiment: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit attach run: %w", err)
	}
	return nil
}

func (d *DB) RecordConclusion(ctx context.Context, experimentID int64, conclusion string) error {
	res, err := d.db.ExecContext(ctx, `
UPDATE experiments
SET conclusion = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?`, conclusion, experimentID)
	if err != nil {
		return fmt.Errorf("record conclusion: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("record conclusion rows affected: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("experiment %d: %w", experimentID, ErrNotFound)
	}
	return nil
}

type experimentScanner interface {
	Scan(dest ...any) error
}

func scanExperiment(scanner experimentScanner, exp *Experiment) error {
	if err := scanner.Scan(
		&exp.ID,
		&exp.Name,
		&exp.DatasetID,
		&exp.Collection,
		&exp.BaselineEvalID,
		&exp.FocusMetric,
		&exp.Hypothesis,
		&exp.Notes,
		&exp.Conclusion,
		&exp.CreatedAt,
		&exp.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return err
		}
		return fmt.Errorf("scan experiment: %w", err)
	}
	return nil
}

func (d *DB) listRuns(ctx context.Context, experimentID int64) ([]RunLabel, error) {
	rows, err := d.db.QueryContext(ctx, `
SELECT label, eval_id, notes, created_at
FROM experiment_runs
WHERE experiment_id = ?
ORDER BY created_at ASC, label ASC`, experimentID)
	if err != nil {
		return nil, fmt.Errorf("list experiment runs: %w", err)
	}
	defer rows.Close()

	var runs []RunLabel
	for rows.Next() {
		var run RunLabel
		if err := rows.Scan(&run.Label, &run.EvalID, &run.Notes, &run.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan experiment run: %w", err)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list experiment runs rows: %w", err)
	}
	return runs, nil
}

func (d *DB) requireExperiment(ctx context.Context, id int64) error {
	var exists int
	err := d.db.QueryRowContext(ctx, `SELECT 1 FROM experiments WHERE id = ?`, id).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("experiment %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("load experiment: %w", err)
	}
	return nil
}
