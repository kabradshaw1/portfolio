-- 004_enable_pg_trgm.up.sql
--
-- Enables the pg_trgm extension so the next migration can create a GIN
-- trigram index for product-name search. Split from the index creation
-- because CREATE INDEX CONCURRENTLY cannot share a migration file with
-- any other statement (golang-migrate wraps each file in a transaction).
-- See docs/runbooks/postgres-migrations.md (recipe 4).

CREATE EXTENSION IF NOT EXISTS pg_trgm;
