//go:build integration

package db_test

import (
	"context"
	"database/sql"
	"io"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestPgStatStatementsAndAutoExplain(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		"postgres:17-alpine",
		postgres.WithDatabase("appdb"),
		postgres.WithUsername("appuser"),
		postgres.WithPassword("apppass"),
		testcontainers.WithCmdArgs(
			"-c", "shared_preload_libraries=pg_stat_statements,auto_explain",
			"-c", "pg_stat_statements.max=5000",
			"-c", "pg_stat_statements.track=top",
			"-c", "pg_stat_statements.track_utility=off",
			"-c", "auto_explain.log_min_duration=500ms",
			"-c", "auto_explain.log_analyze=true",
			"-c", "auto_explain.log_buffers=true",
			"-c", "auto_explain.log_timing=true",
			"-c", "auto_explain.log_format=json",
			"-c", "auto_explain.sample_rate=1.0",
		),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { _ = pgContainer.Terminate(ctx) })

	dsn, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("dsn: %v", err)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	if _, err := db.ExecContext(ctx, `CREATE EXTENSION IF NOT EXISTS pg_stat_statements`); err != nil {
		t.Fatalf("create extension: %v", err)
	}
	if _, err := db.ExecContext(ctx, `SELECT pg_stat_statements_reset()`); err != nil {
		t.Fatalf("reset stats: %v", err)
	}

	for i := 0; i < 3; i++ {
		if _, err := db.ExecContext(ctx, `SELECT pg_sleep(0.6)`); err != nil {
			t.Fatalf("slow query: %v", err)
		}
	}

	var calls int64
	var meanMs float64
	err = db.QueryRowContext(ctx, `
		SELECT calls, mean_exec_time
		FROM pg_stat_statements
		WHERE query LIKE '%pg_sleep%'
		ORDER BY calls DESC LIMIT 1
	`).Scan(&calls, &meanMs)
	if err != nil {
		t.Fatalf("query pg_stat_statements: %v", err)
	}
	if calls < 3 {
		t.Errorf("expected calls >= 3, got %d", calls)
	}
	if meanMs < 500 {
		t.Errorf("expected mean_exec_time >= 500ms, got %.1fms", meanMs)
	}

	logs, err := pgContainer.Logs(ctx)
	if err != nil {
		t.Fatalf("logs: %v", err)
	}
	defer logs.Close()
	raw, err := io.ReadAll(logs)
	if err != nil {
		t.Fatalf("read logs: %v", err)
	}
	logText := string(raw)
	if !strings.Contains(logText, "duration:") || !strings.Contains(logText, "plan:") {
		t.Errorf("expected auto_explain duration/plan markers in logs")
	}
	if !strings.Contains(logText, `"Plan":`) {
		t.Errorf("expected JSON plan body in logs")
	}
}
