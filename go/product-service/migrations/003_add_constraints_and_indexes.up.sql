-- Data integrity: prevent invalid prices and stock at the database level.
-- migration-lint: ignore=MIG004 reason="early-stage migration; ran on empty products in dev/QA before launch — would use NOT VALID + VALIDATE today"
ALTER TABLE products ADD CONSTRAINT chk_products_price CHECK (price > 0);
-- migration-lint: ignore=MIG004 reason="early-stage migration; ran on empty products in dev/QA before launch — would use NOT VALID + VALIDATE today"
ALTER TABLE products ADD CONSTRAINT chk_products_stock CHECK (stock >= 0);

-- Partial index for low-stock inventory alerting queries.
-- Only indexes rows with stock < 10, keeping the index small.
-- migration-lint: ignore=MIG001 reason="early-stage migration; ran on empty products in dev/QA before launch — would be CONCURRENTLY today"
CREATE INDEX idx_products_low_stock ON products (stock) WHERE stock < 10;
