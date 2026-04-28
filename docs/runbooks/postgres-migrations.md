# Safe PostgreSQL Migrations — Recipe Book

A working catalog of safe patterns for `golang-migrate` migrations against
production Postgres. Each recipe is paired with a rule in `migration-lint`
(`go/cmd/migration-lint/`). The linter is the gate; this document is the fix.

> **Convention:** all recipes assume `golang-migrate` numbering. Each step in a
> multi-step recipe ships as its own `NNN_*.up.sql` file deployed in a
> separate release.

---

## Recipe 1 — Add a NOT NULL column to a busy table

**Goal:** add `tier TEXT NOT NULL DEFAULT 'standard'` to `users`.

### Unsafe one-shot

```sql
ALTER TABLE users ADD COLUMN tier TEXT NOT NULL DEFAULT now()::text;
```

**Why unsafe:** a volatile default forces PG to rewrite every row. On a
100M-row table that means an `ACCESS EXCLUSIVE` lock for the duration of the
rewrite — minutes, sometimes longer. PG 11+ fast-paths *constant* defaults;
volatile expressions still rewrite.

### Safe multi-step

```sql
-- 042_add_users_tier_column.up.sql
ALTER TABLE users ADD COLUMN tier TEXT;
```

```sql
-- 043_backfill_users_tier.up.sql
-- Run as a chunked job from the application instead of inline if the table is large.
UPDATE users SET tier = 'standard' WHERE tier IS NULL;
```

```sql
-- 044_users_tier_set_default.up.sql
ALTER TABLE users ALTER COLUMN tier SET DEFAULT 'standard';
```

```sql
-- 045_users_tier_set_not_null.up.sql
ALTER TABLE users ALTER COLUMN tier SET NOT NULL;
```

### Linter rule

`MIG002` — `ADD COLUMN ... NOT NULL` with a non-constant default.

### Cross-references

- `docs/adr/database/migration-lint.md`
- Recipe 5 (CHECK constraint) uses a similar two-phase shape

---

## Recipe 2 — Drop a column

**Goal:** remove `users.deprecated_field`.

### Unsafe one-shot

```sql
ALTER TABLE users DROP COLUMN deprecated_field;
```

**Why unsafe:** the DDL itself is fast — PG just marks the column attisdropped.
The danger is the *application*: any running code that still references the
column will start erroring. Linters can't see your application graph; only you
can.

### Safe procedure

1. **Stop writing the column** (deploy code that no longer SETs it).
2. **Stop reading the column** (deploy code that no longer SELECTs it).
3. **Verify in production** — check `pg_stat_all_tables` plus app-side metrics
   that confirm callers stopped touching the column.
4. **Drop** with an ignore directive that records the verification.

```sql
-- migration-lint: ignore=MIG005 reason="last write 2026-04-01; pg_stat confirms zero reads since 2026-04-15; deploy 2026-04-22 removed final reference"
ALTER TABLE users DROP COLUMN deprecated_field;
```

### Linter rule

`MIG005` — DROP COLUMN must include a documented ignore directive.

---

## Recipe 3 — Rename a column (expand-and-contract)

**Goal:** rename `users.email` to `users.email_address`.

### Unsafe one-shot

```sql
ALTER TABLE users RENAME COLUMN email TO email_address;
```

**Why unsafe:** rolling deploys mean old replicas of your application are still
issuing queries against `users.email` while new replicas query
`users.email_address`. Either side errors.

### Safe multi-step

1. **Add new column** (nullable):

```sql
-- 060_add_email_address_column.up.sql
ALTER TABLE users ADD COLUMN email_address TEXT;
```

2. **Backfill + dual-write trigger** during the transition window:

```sql
-- 061_dual_write_email.up.sql
CREATE OR REPLACE FUNCTION users_email_dual_write() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.email_address IS NULL THEN NEW.email_address := NEW.email; END IF;
  IF NEW.email IS NULL THEN NEW.email := NEW.email_address; END IF;
  RETURN NEW;
END;
$$;
CREATE TRIGGER users_email_dual_write_trg BEFORE INSERT OR UPDATE ON users
  FOR EACH ROW EXECUTE FUNCTION users_email_dual_write();

UPDATE users SET email_address = email WHERE email_address IS NULL;
```

3. **Switch reads** in the application (deploy).
4. **Switch writes** in the application (deploy). Drop the trigger.

```sql
-- 062_drop_dual_write.up.sql
DROP TRIGGER users_email_dual_write_trg ON users;
DROP FUNCTION users_email_dual_write();
```

5. **Drop the old column** with an ignore directive (see recipe 2).

### Linter rule

`MIG006` — RENAME COLUMN requires an ignore directive (don't actually rename —
use expand-and-contract).

---

## Recipe 4 — Add an index without locking writers

**Goal:** index `orders.user_id` on a 50M-row table.

### Unsafe one-shot

```sql
CREATE INDEX idx_orders_user_id ON orders (user_id);
```

**Why unsafe:** plain `CREATE INDEX` takes `ACCESS EXCLUSIVE` for the whole
build. Writers are blocked.

### Safe pattern (one statement per migration file)

```sql
-- 070_orders_user_id_idx.up.sql
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_orders_user_id ON orders (user_id);
```

**Two non-negotiables:**

1. The migration file contains exactly **one** statement. `CREATE INDEX
   CONCURRENTLY` cannot run inside a transaction, and `golang-migrate` wraps
   each migration file in one.
2. Use `IF NOT EXISTS` so a half-built index from a prior failed run can be
   retried. (Drop and rebuild it manually if it's invalid — `\d+` shows
   `INVALID` next to the index name.)

### Linter rules

- `MIG001` — bare `CREATE INDEX` against a table not created in the same file.
- `MIG007` — `CONCURRENTLY` mixed with any other statement.

### Cross-references

- The worked example: `go/product-service/migrations/005_add_product_search_index.up.sql`
  (with the matching extension migration in `004_enable_pg_trgm.up.sql`)

---

## Recipe 5 — Add a CHECK constraint

**Goal:** enforce `orders.total > 0`.

### Unsafe one-shot

```sql
ALTER TABLE orders ADD CONSTRAINT total_positive CHECK (total > 0);
```

**Why unsafe:** PG validates the constraint by full-table scan under
`ACCESS EXCLUSIVE`.

### Safe two-phase

```sql
-- 080_check_total_positive_not_valid.up.sql
ALTER TABLE orders ADD CONSTRAINT total_positive CHECK (total > 0) NOT VALID;
```

```sql
-- 081_validate_total_positive.up.sql
ALTER TABLE orders VALIDATE CONSTRAINT total_positive;
```

`NOT VALID` skips the scan; `VALIDATE` does it later under a `SHARE UPDATE
EXCLUSIVE` lock that doesn't block writes.

### Linter rule

`MIG004` — CHECK constraint without `NOT VALID`.

---

## Recipe 6 — Rename a table

**Goal:** rename `users` to `accounts`.

### Unsafe one-shot

```sql
ALTER TABLE users RENAME TO accounts;
```

**Why unsafe:** same rolling-deploy problem as column rename.

### Safe procedure (compatibility view)

```sql
-- 090_rename_users_to_accounts.up.sql
ALTER TABLE users RENAME TO accounts;
CREATE VIEW users AS SELECT * FROM accounts;
```

The view bridges the deploy window. After the application has been fully cut
over to `accounts`, drop the view in a follow-up migration.

### Linter rule

None today — table renames don't have a dedicated rule. Add one if it becomes
a recurring pattern.

---

## Recipe 7 — Change a column type

**Goal:** change `orders.total` from `INTEGER` to `BIGINT`.

### Unsafe one-shot

```sql
ALTER TABLE orders ALTER COLUMN total TYPE BIGINT;
```

**Why unsafe:** type changes generally rewrite every row. (`varchar(N)` →
`varchar(M)` where M > N is the famous binary-compatible exception, but plan
for the rewrite by default.)

### Safe expand-and-contract

1. Add `total_v2 BIGINT` (nullable).
2. Dual-write the new column from the application.
3. Backfill in chunks.
4. Switch reads to `total_v2`.
5. Drop `total` (recipe 2).
6. Optionally rename `total_v2` → `total` (recipe 3).

### Linter rule

`MIG003` — ALTER COLUMN TYPE.

---

## Recipe 8 — Partition an existing table

**Goal:** partition `orders` by month on `created_at`.

This is genuinely complex enough that it gets its own ADR rather than a
fits-in-a-section recipe. See `docs/adr/ecommerce/go-sql-optimization-reporting.md`
for the full migration sequence and the FK trade-offs that come with composite
primary keys.

### Linter rule

The migration is in production and has its violations annotated with ignore
directives. The runbook entry is here so future readers know where to look.
