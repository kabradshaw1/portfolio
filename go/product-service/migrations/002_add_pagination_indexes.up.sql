-- migration-lint: ignore=MIG001 reason="early-stage migration; ran on empty products table in dev/QA before launch — would be CONCURRENTLY today"
CREATE INDEX IF NOT EXISTS idx_products_price_id ON products (price, id);
-- migration-lint: ignore=MIG001 reason="early-stage migration; ran on empty products table in dev/QA before launch — would be CONCURRENTLY today"
CREATE INDEX IF NOT EXISTS idx_products_name_id ON products (name, id);
-- migration-lint: ignore=MIG001 reason="early-stage migration; ran on empty products table in dev/QA before launch — would be CONCURRENTLY today"
CREATE INDEX IF NOT EXISTS idx_products_created_at_id ON products (created_at DESC, id DESC);
