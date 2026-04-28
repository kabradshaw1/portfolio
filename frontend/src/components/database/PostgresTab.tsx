import { PillarSection } from "@/components/database/PillarSection";
import {
  StickyTocChips,
  StickyTocSidebar,
  type StickyTocItem,
} from "@/components/database/StickyToc";

const tocItems: StickyTocItem[] = [
  { id: "optimization", label: "Query Optimization" },
  { id: "schema", label: "Schema Design" },
  { id: "migrations", label: "Migration Safety" },
  { id: "reliability", label: "Reliability & Recovery" },
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
          id="reliability"
          title="Reliability & Recovery"
          narrative={
            <>
              <p>
                Production-grade SQL isn&apos;t only about queries. Postgres needs scheduled
                backups, monitored health, and a written runbook for the day someone has to restore
                from one. The portfolio&apos;s Postgres deployment ships with all three.
              </p>
            </>
          }
          bullets={[
            <>
              Automated <code>pg_dump</code> CronJob writing to a persistent volume on the Minikube
              node; retention policy in the manifest
            </>,
            <>
              Pod Disruption Budget on the StatefulSet (<code>maxUnavailable: 1</code>) so node
              drains don&apos;t block on a single-replica DB
            </>,
            <>
              <code>postgres_exporter</code> sidecar feeding Prometheus; Grafana dashboard surfaces
              connection counts, replication lag, table sizes, and slow queries
            </>,
            <>
              Alert rules: backup-job failure, replication-lag-too-high, disk-full,
              long-running-transaction
            </>,
            <>
              Written recovery runbook: step-by-step from <code>pg_dump</code> artifact to a
              restored database
            </>,
          ]}
          links={[
            {
              label: "Read the runbook",
              href: "https://github.com/kabradshaw1/portfolio/blob/main/docs/runbooks/postgres-recovery.md",
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
