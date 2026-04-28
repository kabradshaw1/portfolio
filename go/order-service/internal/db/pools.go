// Package db wires the order-service's PostgreSQL connection pools.
//
// The service uses two physical pools that point at different Postgres
// instances:
//
//   - Primary    — the read/write primary (postgres.java-tasks). All OLTP
//                  traffic (orders CRUD, saga state, partition maintenance,
//                  materialized-view refresh) runs here.
//   - Reporting  — an async streaming read replica (postgres-replica.java-tasks)
//                  that serves the /reporting/* endpoints. Falls back to the
//                  primary DSN when DATABASE_URL_REPLICA is unset (local dev,
//                  CI, single-pod environments) or when the replica is
//                  unreachable at startup (degraded mode — the service stays
//                  up and reporting reads transparently hit the primary).
//
// Each pool sets a distinct `application_name` runtime parameter so primary
// vs reporting traffic is trivially distinguishable in pg_stat_activity —
// "did the reporting reads actually move off the primary?" becomes a single
// query instead of guesswork.
package db

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool tuning constants — kept here (not magic numbers in newPool) so that
// any future per-pool divergence (e.g., a smaller MaxConns on the replica)
// is a one-line change with explanatory context.
const (
	poolMaxConns          = 25
	poolMinConns          = 5
	poolMaxConnIdleTime   = 5 * time.Minute
	poolMaxConnLifetime   = 30 * time.Minute
	poolHealthCheckPeriod = 30 * time.Second
)

// Pools holds the physical connection pools used by the order-service.
type Pools struct {
	Primary   *pgxpool.Pool
	Reporting *pgxpool.Pool
}

// replicaConnectTimeout caps how long we wait for the replica at startup.
// Short by design: if the replica is unreachable we degrade to primary
// rather than blocking the pod from becoming Ready — and the cluster's
// internal DNS + Postgres typically settle in well under this budget.
const replicaConnectTimeout = 5 * time.Second

// New connects both pools.
//
// Behavior matrix:
//
//   - replicaDSN empty                  → Reporting is aliased to Primary
//     (single-Postgres dev/CI; no second connection).
//   - replicaDSN set, replica reachable → Reporting is its own pool against
//     the replica.
//   - replicaDSN set, replica unreachable → Reporting is aliased to Primary
//     and a warning is logged. The service stays up; reporting reads
//     transparently hit the primary until a future restart re-resolves the
//     replica. This avoids cross-namespace deployment-ordering deadlocks
//     (e.g., QA's ExternalName pointing at a not-yet-deployed prod replica).
//
// In every case Reporting is non-nil, so callers never need to nil-check.
func New(ctx context.Context, primaryDSN, replicaDSN string) (*Pools, error) {
	primary, err := newPool(ctx, primaryDSN, "order-service")
	if err != nil {
		return nil, fmt.Errorf("primary pool: %w", err)
	}

	if replicaDSN == "" || replicaDSN == primaryDSN {
		return &Pools{Primary: primary, Reporting: primary}, nil
	}

	replicaCtx, cancel := context.WithTimeout(ctx, replicaConnectTimeout)
	defer cancel()
	reporting, err := newPool(replicaCtx, replicaDSN, "order-service-reporting")
	if err != nil {
		slog.Warn("reporting pool unavailable, falling back to primary",
			"error", err.Error(),
		)
		return &Pools{Primary: primary, Reporting: primary}, nil
	}

	return &Pools{Primary: primary, Reporting: reporting}, nil
}

// Close shuts down both pools. Safe to call once on shutdown.
func (p *Pools) Close() {
	if p == nil {
		return
	}
	if p.Reporting != nil && p.Reporting != p.Primary {
		p.Reporting.Close()
	}
	if p.Primary != nil {
		p.Primary.Close()
	}
}

func newPool(ctx context.Context, dsn, appName string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	cfg.MaxConns = poolMaxConns
	cfg.MinConns = poolMinConns
	cfg.MaxConnIdleTime = poolMaxConnIdleTime
	cfg.MaxConnLifetime = poolMaxConnLifetime
	cfg.HealthCheckPeriod = poolHealthCheckPeriod
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeCacheDescribe

	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	cfg.ConnConfig.RuntimeParams["application_name"] = appName

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return pool, nil
}
