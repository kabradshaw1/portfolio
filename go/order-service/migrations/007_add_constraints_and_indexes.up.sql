-- Data integrity: prevent invalid totals, quantities, and prices.
ALTER TABLE orders ADD CONSTRAINT chk_orders_total CHECK (total > 0);
ALTER TABLE order_items ADD CONSTRAINT chk_order_items_quantity CHECK (quantity > 0);
ALTER TABLE order_items ADD CONSTRAINT chk_order_items_price CHECK (price_at_purchase > 0);

-- Index for FindIncompleteSagas which filters by saga_step.
-- Without this, the query does a sequential scan on the orders table.
CREATE INDEX idx_orders_saga_step ON orders (saga_step);

-- Index for querying returns by status (common access pattern).
CREATE INDEX idx_returns_status ON returns (status);
