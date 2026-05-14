package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestExperimentLifecycle(t *testing.T) {
	ctx := context.Background()
	db := openMigratedDB(t)
	defer db.Close()

	id, err := db.CreateExperiment(ctx, CreateExperimentInput{
		Name:           "rerank experiment",
		DatasetID:      "dataset-1",
		Collection:     "documents",
		BaselineEvalID: "eval-base",
		FocusMetric:    "context_recall",
		Hypothesis:     "reranking improves recall",
		Notes:          "local metadata only",
	})
	if err != nil {
		t.Fatalf("CreateExperiment error: %v", err)
	}
	if id == 0 {
		t.Fatal("CreateExperiment returned zero ID")
	}

	if err := db.AttachRun(ctx, id, "baseline", "eval-base", "original run"); err != nil {
		t.Fatalf("AttachRun baseline error: %v", err)
	}
	if err := db.AttachRun(ctx, id, "candidate", "eval-candidate", "rerank run"); err != nil {
		t.Fatalf("AttachRun candidate error: %v", err)
	}
	if err := db.RecordConclusion(ctx, id, "candidate improved recall"); err != nil {
		t.Fatalf("RecordConclusion error: %v", err)
	}

	got, err := db.GetExperiment(ctx, id)
	if err != nil {
		t.Fatalf("GetExperiment error: %v", err)
	}
	if got.ID != id || got.Name != "rerank experiment" || got.DatasetID != "dataset-1" || got.Collection != "documents" {
		t.Fatalf("experiment = %#v", got)
	}
	if got.BaselineEvalID != "eval-base" || got.FocusMetric != "context_recall" || got.Hypothesis != "reranking improves recall" || got.Notes != "local metadata only" {
		t.Fatalf("experiment metadata = %#v", got)
	}
	if got.Conclusion != "candidate improved recall" {
		t.Fatalf("Conclusion = %q", got.Conclusion)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("timestamps not populated: %#v", got)
	}
	if len(got.Runs) != 2 {
		t.Fatalf("Runs = %#v", got.Runs)
	}
	assertRun(t, got.Runs[0], "baseline", "eval-base", "original run")
	assertRun(t, got.Runs[1], "candidate", "eval-candidate", "rerank run")

	list, err := db.ListExperiments(ctx)
	if err != nil {
		t.Fatalf("ListExperiments error: %v", err)
	}
	if len(list) != 1 || list[0].ID != id {
		t.Fatalf("experiments = %#v", list)
	}
}

func TestGetExperimentNotFound(t *testing.T) {
	ctx := context.Background()
	db := openMigratedDB(t)
	defer db.Close()

	_, err := db.GetExperiment(ctx, 42)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetExperiment error = %v, want ErrNotFound", err)
	}
}

func TestMutationsReturnNotFoundForMissingExperiment(t *testing.T) {
	ctx := context.Background()
	db := openMigratedDB(t)
	defer db.Close()

	if err := db.AttachRun(ctx, 42, "baseline", "eval-1", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("AttachRun error = %v, want ErrNotFound", err)
	}
	if err := db.RecordConclusion(ctx, 42, "no experiment"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("RecordConclusion error = %v, want ErrNotFound", err)
	}
}

func TestAttachRunReplacesExistingLabel(t *testing.T) {
	ctx := context.Background()
	db := openMigratedDB(t)
	defer db.Close()

	id, err := db.CreateExperiment(ctx, CreateExperimentInput{
		Name:      "label replacement",
		DatasetID: "dataset-1",
	})
	if err != nil {
		t.Fatalf("CreateExperiment error: %v", err)
	}
	if err := db.AttachRun(ctx, id, "baseline", "eval-old", "old baseline"); err != nil {
		t.Fatalf("AttachRun old baseline error: %v", err)
	}
	if err := db.AttachRun(ctx, id, "baseline", "eval-new", "new baseline"); err != nil {
		t.Fatalf("AttachRun new baseline error: %v", err)
	}

	got, err := db.GetExperiment(ctx, id)
	if err != nil {
		t.Fatalf("GetExperiment error: %v", err)
	}
	if len(got.Runs) != 1 {
		t.Fatalf("Runs = %#v", got.Runs)
	}
	assertRun(t, got.Runs[0], "baseline", "eval-new", "new baseline")
}

func TestAttachRunUpdatesExperimentTimestamp(t *testing.T) {
	ctx := context.Background()
	db := openMigratedDB(t)
	defer db.Close()

	id, err := db.CreateExperiment(ctx, CreateExperimentInput{
		Name:      "timestamp update",
		DatasetID: "dataset-1",
	})
	if err != nil {
		t.Fatalf("CreateExperiment error: %v", err)
	}
	setUpdatedAt(t, db, id, "2000-01-01 00:00:00")
	before, err := db.GetExperiment(ctx, id)
	if err != nil {
		t.Fatalf("GetExperiment before attach error: %v", err)
	}

	if err := db.AttachRun(ctx, id, "baseline", "eval-1", "baseline"); err != nil {
		t.Fatalf("AttachRun error: %v", err)
	}
	afterAttach, err := db.GetExperiment(ctx, id)
	if err != nil {
		t.Fatalf("GetExperiment after attach error: %v", err)
	}
	if !afterAttach.UpdatedAt.After(before.UpdatedAt) {
		t.Fatalf("UpdatedAt after attach = %s, want after %s", afterAttach.UpdatedAt, before.UpdatedAt)
	}

	setUpdatedAt(t, db, id, "2000-01-01 00:00:00")
	beforeReplace, err := db.GetExperiment(ctx, id)
	if err != nil {
		t.Fatalf("GetExperiment before replace error: %v", err)
	}
	if err := db.AttachRun(ctx, id, "baseline", "eval-2", "replacement"); err != nil {
		t.Fatalf("AttachRun replace error: %v", err)
	}
	afterReplace, err := db.GetExperiment(ctx, id)
	if err != nil {
		t.Fatalf("GetExperiment after replace error: %v", err)
	}
	if !afterReplace.UpdatedAt.After(beforeReplace.UpdatedAt) {
		t.Fatalf("UpdatedAt after replace = %s, want after %s", afterReplace.UpdatedAt, beforeReplace.UpdatedAt)
	}
}

func TestDeleteExperimentCascadesRuns(t *testing.T) {
	ctx := context.Background()
	db := openMigratedDB(t)
	defer db.Close()

	id, err := db.CreateExperiment(ctx, CreateExperimentInput{
		Name:      "cascade",
		DatasetID: "dataset-1",
	})
	if err != nil {
		t.Fatalf("CreateExperiment error: %v", err)
	}
	if err := db.AttachRun(ctx, id, "baseline", "eval-1", "baseline"); err != nil {
		t.Fatalf("AttachRun error: %v", err)
	}
	if _, err := db.db.ExecContext(ctx, `DELETE FROM experiments WHERE id = ?`, id); err != nil {
		t.Fatalf("delete experiment: %v", err)
	}

	var count int
	if err := db.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM experiment_runs WHERE experiment_id = ?`, id).Scan(&count); err != nil {
		t.Fatalf("count experiment runs: %v", err)
	}
	if count != 0 {
		t.Fatalf("experiment_runs count = %d, want 0", count)
	}
}

func TestListExperimentsNewestFirst(t *testing.T) {
	ctx := context.Background()
	db := openMigratedDB(t)
	defer db.Close()

	firstID, err := db.CreateExperiment(ctx, CreateExperimentInput{
		Name:      "first",
		DatasetID: "dataset-1",
	})
	if err != nil {
		t.Fatalf("CreateExperiment first error: %v", err)
	}
	secondID, err := db.CreateExperiment(ctx, CreateExperimentInput{
		Name:      "second",
		DatasetID: "dataset-2",
	})
	if err != nil {
		t.Fatalf("CreateExperiment second error: %v", err)
	}

	got, err := db.ListExperiments(ctx)
	if err != nil {
		t.Fatalf("ListExperiments error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("experiments = %#v", got)
	}
	if got[0].ID != secondID || got[1].ID != firstID {
		t.Fatalf("experiment order = %#v", got)
	}
}

func openMigratedDB(t *testing.T) *DB {
	t.Helper()

	db, err := Open(filepath.Join(t.TempDir(), "eval-mcp.db"))
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	if err := db.Migrate(context.Background()); err != nil {
		_ = db.Close()
		t.Fatalf("Migrate error: %v", err)
	}
	return db
}

func setUpdatedAt(t *testing.T, db *DB, id int64, updatedAt string) {
	t.Helper()

	if _, err := db.db.ExecContext(context.Background(), `UPDATE experiments SET updated_at = ? WHERE id = ?`, updatedAt, id); err != nil {
		t.Fatalf("set updated_at: %v", err)
	}
}

func assertRun(t *testing.T, got RunLabel, wantLabel, wantEvalID, wantNotes string) {
	t.Helper()

	if got.Label != wantLabel || got.EvalID != wantEvalID || got.Notes != wantNotes {
		t.Fatalf("run = %#v", got)
	}
	if got.CreatedAt.IsZero() {
		t.Fatalf("run timestamp not populated: %#v", got)
	}
}
