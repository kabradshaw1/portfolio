-- migration-lint: ignore=MIG003 reason="dev-only schema fix before any prod orders existed; type widening from VARCHAR(20) to TEXT (binary-compatible in PG)"
ALTER TABLE orders ALTER COLUMN saga_step TYPE TEXT;
