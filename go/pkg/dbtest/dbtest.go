// Package dbtest provides test helpers for spinning up real PostgreSQL
// containers via testcontainers-go.  Services call SetupPostgres from
// TestMain so the container is created once per test binary.
package dbtest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestDB holds a running PostgreSQL container and its connection pool.
type TestDB struct {
	Pool      *pgxpool.Pool
	container testcontainers.Container
}

// SetupPostgres starts a PostgreSQL container, runs migrations from
// migrationsDir, and returns a connected pool. Call Teardown when done.
func SetupPostgres(ctx context.Context, migrationsDir string) (*TestDB, error) {
	absDir, err := filepath.Abs(migrationsDir)
	if err != nil {
		return nil, fmt.Errorf("resolve migrations dir: %w", err)
	}

	// Collect .up.sql files for init scripts (run in lexicographic order).
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}
	var initScripts []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".sql" && strings.Contains(e.Name(), ".up.") {
			initScripts = append(initScripts, filepath.Join(absDir, e.Name()))
		}
	}

	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
		postgres.WithInitScripts(initScripts...),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("start postgres container: %w", err)
	}

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("get connection string: %w", err)
	}

	poolCfg, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("parse pool config: %w", err)
	}
	poolCfg.MaxConns = 10
	poolCfg.MinConns = 2

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("create pool: %w", err)
	}

	return &TestDB{Pool: pool, container: container}, nil
}

// Teardown closes the pool and terminates the container.
func (tdb *TestDB) Teardown(ctx context.Context) {
	if tdb.Pool != nil {
		tdb.Pool.Close()
	}
	if tdb.container != nil {
		_ = tdb.container.Terminate(ctx)
	}
}

// SeedSQL executes a SQL file against the pool.
func (tdb *TestDB) SeedSQL(ctx context.Context, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read seed file: %w", err)
	}
	_, err = tdb.Pool.Exec(ctx, string(data))
	return err
}

// CaptureExplain runs EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) for a query
// and writes the JSON output to outPath.
func CaptureExplain(ctx context.Context, pool *pgxpool.Pool, outPath, query string, args ...any) error {
	explainQuery := "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) " + query
	var planJSON []byte
	err := pool.QueryRow(ctx, explainQuery, args...).Scan(&planJSON)
	if err != nil {
		return fmt.Errorf("explain analyze: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("create benchdata dir: %w", err)
	}
	return os.WriteFile(outPath, planJSON, 0o644)
}
