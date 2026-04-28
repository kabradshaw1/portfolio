//go:build integration

package db_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestPgBouncerPreparedStatementReuse boots Postgres + PgBouncer (transaction
// mode, max_prepared_statements=200) and verifies pgx's CacheDescribe path
// works through the pooler and that backend count stays bounded.
func TestPgBouncerPreparedStatementReuse(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// 1. Start Postgres
	pgC, err := postgres.Run(ctx, "postgres:16",
		postgres.WithDatabase("appdb"),
		postgres.WithUsername("app"),
		postgres.WithPassword("app"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2)),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	defer pgC.Terminate(ctx)

	pgHost, _ := pgC.Host(ctx)
	pgPort, _ := pgC.MappedPort(ctx, "5432/tcp")
	pgInternal, _ := pgC.ContainerIP(ctx)

	// 2. Bootstrap pg_stat_statements + pgbouncer auth role on Postgres
	directURL := fmt.Sprintf("postgres://app:app@%s:%s/appdb?sslmode=disable", pgHost, pgPort.Port())
	bootstrap, err := pgxpool.New(ctx, directURL)
	if err != nil {
		t.Fatalf("bootstrap pool: %v", err)
	}
	for _, q := range []string{
		`CREATE EXTENSION IF NOT EXISTS pg_stat_statements`,
		`CREATE ROLE pgbouncer_auth LOGIN PASSWORD 'auth'`,
		`CREATE OR REPLACE FUNCTION public.pgbouncer_get_auth(p_usename TEXT)
		   RETURNS TABLE(usename TEXT, passwd TEXT)
		   LANGUAGE sql SECURITY DEFINER SET search_path = pg_catalog AS $$
		     SELECT usename::TEXT, passwd::TEXT FROM pg_shadow WHERE usename = p_usename;
		   $$`,
		`GRANT EXECUTE ON FUNCTION public.pgbouncer_get_auth(TEXT) TO pgbouncer_auth`,
		`CREATE TABLE widgets (id SERIAL PRIMARY KEY, name TEXT NOT NULL)`,
		`INSERT INTO widgets(name) SELECT 'w'||g FROM generate_series(1,100) g`,
	} {
		if _, err := bootstrap.Exec(ctx, q); err != nil {
			t.Fatalf("bootstrap %q: %v", q, err)
		}
	}
	bootstrap.Close()

	// 3. Start PgBouncer pointing at Postgres' internal IP
	pgbConfig := fmt.Sprintf(`
[databases]
appdb = host=%s port=5432 dbname=appdb

[pgbouncer]
listen_addr = 0.0.0.0
listen_port = 6432
auth_type = scram-sha-256
auth_user = pgbouncer_auth
auth_query = SELECT usename, passwd FROM public.pgbouncer_get_auth($1)
auth_file = /etc/pgbouncer/userlist.txt
pool_mode = transaction
max_client_conn = 200
default_pool_size = 5
max_prepared_statements = 200
ignore_startup_parameters = extra_float_digits,application_name
`, pgInternal)

	pgbReq := testcontainers.ContainerRequest{
		Image:        "edoburu/pgbouncer:1.23.1",
		ExposedPorts: []string{"6432/tcp"},
		WaitingFor:   wait.ForListeningPort("6432/tcp"),
		Files: []testcontainers.ContainerFile{
			{Reader: stringReader(pgbConfig), ContainerFilePath: "/etc/pgbouncer/pgbouncer.ini", FileMode: 0o644},
			{Reader: stringReader(`"pgbouncer_auth" "auth"` + "\n"), ContainerFilePath: "/etc/pgbouncer/userlist.txt", FileMode: 0o600},
		},
		Cmd: []string{"/etc/pgbouncer/pgbouncer.ini"},
	}
	pgbC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: pgbReq, Started: true,
	})
	if err != nil {
		t.Fatalf("start pgbouncer: %v", err)
	}
	defer pgbC.Terminate(ctx)
	pgbHost, _ := pgbC.Host(ctx)
	pgbPort, _ := pgbC.MappedPort(ctx, "6432/tcp")

	// 4. Open pgxpool through PgBouncer with CacheDescribe
	pooledURL := fmt.Sprintf("postgres://app:app@%s:%s/appdb?sslmode=disable", pgbHost, pgbPort.Port())
	cfg, err := pgxpool.ParseConfig(pooledURL)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfg.MaxConns = 20
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeCacheDescribe
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()

	// 5. Hammer the same parameterized query from 20 goroutines, 5 queries each = 100 total.
	var wg sync.WaitGroup
	errs := make(chan error, 100)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				var name string
				err := pool.QueryRow(ctx, "SELECT name FROM widgets WHERE id = $1", (i*5+j)%100+1).Scan(&name)
				if err != nil {
					errs <- err
					return
				}
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("query error: %v", err)
	}

	// 6. Assert: queryid is stable in pg_stat_statements (prepared-statement reuse).
	verify, _ := pgxpool.New(ctx, directURL)
	defer verify.Close()
	var calls int64
	err = verify.QueryRow(ctx, `
		SELECT calls FROM pg_stat_statements
		WHERE query LIKE 'SELECT name FROM widgets WHERE id = $%' LIMIT 1`).Scan(&calls)
	if err != nil {
		t.Fatalf("pg_stat_statements lookup: %v", err)
	}
	if calls < 100 {
		t.Errorf("expected calls >= 100, got %d (prepared statements may not be reused across pool)", calls)
	}

	// 7. Assert: backend count stayed bounded (default_pool_size = 5).
	var backends int
	err = verify.QueryRow(ctx, `
		SELECT count(*) FROM pg_stat_activity
		WHERE datname = 'appdb' AND application_name LIKE 'pgbouncer%'`).Scan(&backends)
	if err != nil {
		t.Fatalf("pg_stat_activity lookup: %v", err)
	}
	if backends > 6 { // pool=5 + small slop
		t.Errorf("expected ≤6 pgbouncer backends, got %d (fan-in not working)", backends)
	}
}

func stringReader(s string) *strings.Reader { return strings.NewReader(s) }
