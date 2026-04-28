-- Data integrity: prevent invalid totals, quantities, and prices.
-- migration-lint: ignore=MIG004 reason="early-stage migration; ran on empty orders in dev/QA before launch — would use NOT VALID + VALIDATE today"
ALTER TABLE orders ADD CONSTRAINT chk_orders_total CHECK (total > 0);
-- migration-lint: ignore=MIG004 reason="early-stage migration; ran on empty order_items in dev/QA before launch — would use NOT VALID + VALIDATE today"
ALTER TABLE order_items ADD CONSTRAINT chk_order_items_quantity CHECK (quantity > 0);
-- migration-lint: ignore=MIG004 reason="early-stage migration; ran on empty order_items in dev/QA before launch — would use NOT VALID + VALIDATE today"
ALTER TABLE order_items ADD CONSTRAINT chk_order_items_price CHECK (price_at_purchase > 0);

-- Index for FindIncompleteSagas which filters by saga_step.
-- Without this, the query does a sequential scan on the orders table.
-- migration-lint: ignore=MIG001 reason="early-stage migration; ran on empty orders in dev/QA before launch — would be CONCURRENTLY today"
CREATE INDEX idx_orders_saga_step ON orders (saga_step);

-- Index for querying returns by status (common access pattern).
-- migration-lint: ignore=MIG001 reason="early-stage migration; ran on empty returns in dev/QA before launch — would be CONCURRENTLY today"
CREATE INDEX idx_returns_status ON returns (status);
