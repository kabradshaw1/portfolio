import { PillarSection } from "@/components/database/PillarSection";
import {
  StickyTocChips,
  StickyTocSidebar,
  type StickyTocItem,
} from "@/components/database/StickyToc";

const tocItems: StickyTocItem[] = [
  { id: "optimization", label: "Query Optimization" },
  { id: "observability", label: "Query Observability" },
  { id: "reliability", label: "Reliability & Backups" },
  { id: "migrations", label: "Migration Safety" },
  { id: "schema", label: "Schema Design" },
];

export function PostgresTab() {
  return (
    <div data-testid="postgres-tab" className="md:grid md:grid-cols-[1fr_220px] md:gap-10">
      {/* Mobile chip TOC: top of tab content, hidden at md+. */}
      <div className="md:hidden">
        <StickyTocChips items={tocItems} />
      </div>

      <div className="space-y-16 min-w-0">
        <PillarSection
          id="optimization"
          title="Query Optimization & Benchmarking"
          narrative={
            <>
              <p>
                Functional ORM-style code is a starting point, not the finish line.
                The Go services were re-benchmarked against a real PostgreSQL 16
                container and rewritten where the data showed it.
              </p>
              <p>
                <code>testcontainers-go</code> made the benchmarks runnable on any
                Docker-equipped machine; results were captured to{" "}
                <code>go/benchdata/</code> for interview-ready evidence.
              </p>
              <div className="mt-6 overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b text-left">
                      <th className="pb-2 pr-4 font-medium text-foreground">
                        Optimization
                      </th>
                      <th className="pb-2 pr-4 font-medium text-foreground">Before</th>
                      <th className="pb-2 pr-4 font-medium text-foreground">After</th>
                      <th className="pb-2 font-medium text-foreground">Speedup</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y">
                    <tr>
                      <td className="py-2 pr-4">Order creation (20 items)</td>
                      <td className="py-2 pr-4">4.5 ms</td>
                      <td className="py-2 pr-4">1.3 ms</td>
                      <td className="py-2 font-medium text-foreground">3.5&times;</td>
                    </tr>
                    <tr>
                      <td className="py-2 pr-4">Product search</td>
                      <td className="py-2 pr-4">1.0 ms</td>
                      <td className="py-2 pr-4">0.55 ms</td>
                      <td className="py-2 font-medium text-foreground">1.9&times;</td>
                    </tr>
                    <tr>
                      <td className="py-2 pr-4">Order creation (5 items)</td>
                      <td className="py-2 pr-4">1.5 ms</td>
                      <td className="py-2 pr-4">0.8 ms</td>
                      <td className="py-2 font-medium text-foreground">1.8&times;</td>
                    </tr>
                    <tr>
                      <td className="py-2 pr-4">Category filter</td>
                      <td className="py-2 pr-4">430 &micro;s</td>
                      <td className="py-2 pr-4">327 &micro;s</td>
                      <td className="py-2 font-medium text-foreground">1.3&times;</td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </>
          }
          bullets={[
            <>
              Real-DB benchmarks via <code>testcontainers-go</code> against PostgreSQL 16; results in{" "}
              <code>go/benchdata/baseline-results.txt</code> and <code>optimized-results.txt</code>
            </>,
            <>
              <strong>Batch INSERT for order items: 3.5× speedup on 20-item orders</strong> (4.5ms → 1.3ms),
              single round trip instead of N
            </>,
            <>
              <code>COUNT(*) OVER()</code> window function replaces COUNT-then-data double query
            </>,
            <>
              CTE-based atomic conflict resolution in cart updates (
              <code>WITH updated AS (UPDATE … RETURNING) SELECT EXISTS(...)</code>)
            </>,
            <>
              pgx prepared-statement cache enabled (<code>QueryExecModeCacheDescribe</code>)
            </>,
            <>
              Targeted indexes: <code>idx_orders_saga_step</code>, partial{" "}
              <code>idx_products_low_stock WHERE stock &lt; 10</code>, composite{" "}
              <code>idx_cart_items_user_reserved</code>
            </>,
            <>
              Typed pgx error checks: <code>errors.Is(err, pgx.ErrNoRows)</code>,{" "}
              <code>errors.As(err, &amp;pgconn.PgError)</code> for code <code>23505</code>
            </>,
          ]}
          links={[
            {
              label: "Read the ADR",
              href: "https://github.com/kabradshaw1/portfolio/blob/main/docs/adr/ecommerce/go-database-optimization.md",
            },
          ]}
        />

        <PillarSection
          id="observability"
          title="Query Observability — pg_stat_statements + auto_explain"
          narrative={
            <>
              <p>
                Slow queries don&apos;t fix themselves; they have to be found first.
                Postgres ships two extensions that do exactly this — <code>pg_stat_statements</code>{" "}
                aggregates per-query latency, call counts, and IO; <code>auto_explain</code>{" "}
                captures full execution plans for any query that crosses a duration threshold.
              </p>
              <p>
                Both are wired into the portfolio so the slow query a hiring manager would
                normally have to take on faith is instead visible in Grafana.
              </p>
            </>
          }
          bullets={[
            <>
              <code>shared_preload_libraries=&apos;pg_stat_statements,auto_explain&apos;</code> set on
              the Postgres deployment — server-wide enablement; restart picked up by the existing{" "}
              <code>Recreate</code> strategy.
            </>,
            <>
              Per-database <code>CREATE EXTENSION IF NOT EXISTS pg_stat_statements</code> for all
              7 prod databases, bootstrapped by an idempotent K8s Job{" "}
              (<code>postgres-extensions-bootstrap</code>).
            </>,
            <>
              <code>auto_explain.log_min_duration=500ms</code>, <code>log_analyze=true</code>,{" "}
              <code>log_format=json</code> — every query over 500ms writes a JSON plan to Postgres
              logs, which Promtail ships to Loki keyed by <code>query_id</code>.
            </>,
            <>
              Custom <code>postgres_exporter</code> queries surface the top-50 by mean latency
              and the top-50 by IO, with <code>query_text</code> truncated to 200 chars to bound
              label cardinality.
            </>,
            <>
              Three Prometheus alerts: hard ceiling on per-query mean (&gt; 1s for 10m),
              regression detection (mean &gt; 2× the 7-day baseline for 15m), and an{" "}
              <code>auto_explain</code>-stalled canary that fires when no plan log lines arrive
              in 24h.
            </>,
            <>
              Read-only <code>grafana_reader</code> role (<code>pg_monitor</code> predefined
              role) lets a Grafana PostgreSQL data source render live &ldquo;top slow
              queries&rdquo; tables without leaking write access.
            </>,
          ]}
          links={[
            {
              label: "Read the ADR",
              href: "https://github.com/kabradshaw1/portfolio/blob/main/docs/adr/observability/2026-04-27-pg-query-observability.md",
            },
            {
              label: "Detailed observability story",
              href: "/observability",
            },
          ]}
        />

        <PillarSection
          id="reliability"
          title="Reliability & Backups"
          narrative={
            <>
              <p>
                Production-grade SQL isn&apos;t only about queries. Postgres needs scheduled
                backups, continuous WAL archiving, monitored health, and a written runbook for
                the day someone has to restore.
              </p>
              <p>
                The portfolio&apos;s Postgres deployment ships all four — and verifies the
                backups are actually restorable, because a backup that hasn&apos;t been
                restored is a hope, not a guarantee.
              </p>
            </>
          }
          bullets={[
            <>
              Daily <code>pg_dump --format=custom</code> per database (7 prod DBs), 7-day
              retention; backups land on a hostPath PV (<code>/backups/postgres</code>) separate
              from the Postgres data PVC so PVC corruption doesn&apos;t affect backups.
            </>,
            <>
              Postgres deployment uses <code>Recreate</code> strategy +{" "}
              <code>terminationGracePeriodSeconds: 90</code> +{" "}
              <code>preStop: pg_ctl stop -m fast</code> — the combination that prevents the
              WAL-corruption incident the data-integrity ADR documents.
            </>,
            <>
              <code>PodDisruptionBudget</code> with <code>maxUnavailable: 0</code> on the
              single-replica DB so node drains don&apos;t take it out involuntarily.
            </>,
            <>
              <code>archive_mode=on</code> + custom <code>archive_command</code> wrapper script{" "}
              (<code>pg-archive-wal.sh</code>, atomic via temp + rename) ships every WAL segment
              to a 10Gi <code>wal-archive</code> PV.
            </>,
            <>
              <code>archive_timeout=300</code> forces a WAL switch every 5 min during idle
              periods, so RPO drops from ≤ 24h to ≤ 5m.
            </>,
            <>
              Weekly <code>pg_basebackup</code> CronJob (Sundays 03:00 UTC) writes{" "}
              <code>--format=tar --gzip --wal-method=fetch</code> tarballs; retains 4 weeklies +
              WAL back to the second-newest base backup. Uses a dedicated <code>replicator</code>{" "}
              role with only <code>REPLICATION LOGIN</code> (not <code>taskuser</code>).
            </>,
            <>
              Three <code>pg_stat_archiver</code>-based alerts: archive command failing, WAL
              archive stale, base backup stale.
            </>,
            <>
              Daily <code>postgres-backup-verify</code> CronJob restores yesterday&apos;s dump
              into a throwaway database, runs <code>pg_restore --list | wc -l</code> and a
              row-count smoke check, pushes success/failure to Pushgateway as a Prometheus
              metric.
            </>,
            <>
              Two verification alerts: verification <em>failed</em> (immediate, severity
              critical) and verification <em>stale</em> (no successful verify in 26h, severity
              warning).
            </>,
            <>
              The verification metric is on the existing PostgreSQL dashboard alongside the
              <code>pg_dump</code>-stale and basebackup-stale panels — three operational
              signals on one screen.
            </>,
            <>
              <code>postgres_exporter</code> sidecar feeding Prometheus; Grafana dashboard
              surfaces connection counts, replication lag, table sizes, and slow queries.
            </>,
            <>
              Alert rules: backup-job failure, replication-lag-too-high, disk-full,
              long-running-transaction.
            </>,
            <>
              Four-scenario runbook (<code>docs/runbooks/postgres-recovery.md</code>): fresh
              PVC reset, full restore from <code>pg_dump</code>, partial restore (single
              database), point-in-time recovery to a specific timestamp.
            </>,
          ]}
          links={[
            {
              label: "Read the data-integrity ADR",
              href: "https://github.com/kabradshaw1/portfolio/blob/main/docs/adr/infrastructure/2026-04-24-postgres-data-integrity.md",
            },
            {
              label: "Read the WAL/PITR ADR",
              href: "https://github.com/kabradshaw1/portfolio/blob/main/docs/adr/database/wal-archiving-pitr.md",
            },
            {
              label: "Read the backup-verification ADR",
              href: "https://github.com/kabradshaw1/portfolio/blob/main/docs/adr/database/backup-verification.md",
            },
            {
              label: "Read the recovery runbook",
              href: "https://github.com/kabradshaw1/portfolio/blob/main/docs/runbooks/postgres-recovery.md",
            },
          ]}
        />

        <PillarSection
          id="migrations"
          title="Migration Safety — migration-lint"
          narrative={
            <>
              <p>
                <code>golang-migrate</code> catches syntactic errors when the migration runs against
                Docker; it doesn&apos;t catch operationally unsafe DDL that&apos;s syntactically
                valid. A custom Go linter (<code>migration-lint</code>) walks each{" "}
                <code>.up.sql</code> AST via <code>libpg_query</code> and flags eight common
                foot-guns at lint time, before any container starts.
              </p>
              <p>Each rule pairs with a recipe in a checked-in safe-migration runbook.</p>
            </>
          }
          bullets={[
            <>
              Custom Go CLI built on <code>pganalyze/pg_query_go</code> (CGO wrapper around{" "}
              <code>libpg_query</code>, the upstream PG parser)
            </>,
            <>
              Eight rules: CREATE INDEX without CONCURRENTLY (MIG001), NOT NULL ADD COLUMN with
              volatile default (MIG002), table-rewrite ALTER COLUMN TYPE (MIG003), CHECK without
              NOT VALID (MIG004), DROP COLUMN (MIG005), RENAME COLUMN (MIG006), CONCURRENTLY mixed
              with other DDL (MIG007), LOCK TABLE (MIG008)
            </>,
            <>
              Per-statement opt-out:{" "}
              <code>{`-- migration-lint: ignore=MIGNNN reason="..."`}</code> with mandatory{" "}
              <code>reason=&quot;…&quot;</code>
            </>,
            <>
              Wired into <code>make preflight-go-migrations</code> and the CI matrix as a hard
              prerequisite to the runtime migration pipeline
            </>,
            <>Companion 8-recipe runbook</>,
            <>
              Worked example: <code>CREATE INDEX CONCURRENTLY</code> in its own migration file (
              <code>go/product-service/migrations/005_add_product_search_index.up.sql</code>)
            </>,
          ]}
          links={[
            {
              label: "Read the ADR",
              href: "https://github.com/kabradshaw1/portfolio/blob/main/docs/adr/database/migration-lint.md",
            },
            {
              label: "Read the runbook",
              href: "https://github.com/kabradshaw1/portfolio/blob/main/docs/runbooks/postgres-migrations.md",
            },
          ]}
        />

        <PillarSection
          id="schema"
          title="Schema Design — Partitioning & Materialized Views"
          narrative={
            <>
              <p>
                Reporting workloads on a monotonically growing <code>orders</code> table forced a
                schema-design pass. Range partitioning by <code>created_at</code> prunes scan scope;
                three materialized views give constant-time reads for dashboard queries; CTE +
                window functions express the rolling-average business logic without
                application-side aggregation.
              </p>
            </>
          }
          bullets={[
            <>
              Range partitioning on <code>orders.created_at</code> (monthly), 18 months
              pre-provisioned with a default catch-all partition
            </>,
            <>
              Background goroutine creates partitions 3 months ahead daily; idempotent{" "}
              <code>CREATE TABLE IF NOT EXISTS</code>
            </>,
            <>
              Three materialized views (<code>mv_daily_revenue</code>,{" "}
              <code>mv_product_performance</code>, <code>mv_customer_summary</code>) refreshed{" "}
              <code>CONCURRENTLY</code> on a 15-min cadence
            </>,
            <>
              Unique indexes per MV to support <code>REFRESH CONCURRENTLY</code>
            </>,
            <>
              CTE-driven reporting with{" "}
              <code>SUM(...) OVER (ORDER BY day ROWS BETWEEN 6 PRECEDING AND CURRENT ROW)</code> for
              rolling 7/30-day averages
            </>,
            <>
              <code>DENSE_RANK()</code> for tie-aware top-N (turnover, top customers)
            </>,
            <>
              Composite primary key trade-off documented (<code>(id, created_at)</code> removes
              single-column FK target — referential integrity moves to the saga)
            </>,
          ]}
          links={[
            {
              label: "Read the ADR",
              href: "https://github.com/kabradshaw1/portfolio/blob/main/docs/adr/ecommerce/go-sql-optimization-reporting.md",
            },
          ]}
        />
      </div>

      {/* Desktop sidebar TOC: right column at md+. */}
      <aside className="hidden md:block">
        <StickyTocSidebar items={tocItems} />
      </aside>
    </div>
  );
}
