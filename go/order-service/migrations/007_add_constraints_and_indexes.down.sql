DROP INDEX IF EXISTS idx_returns_status;
DROP INDEX IF EXISTS idx_orders_saga_step;
ALTER TABLE order_items DROP CONSTRAINT IF EXISTS chk_order_items_price;
ALTER TABLE order_items DROP CONSTRAINT IF EXISTS chk_order_items_quantity;
ALTER TABLE orders DROP CONSTRAINT IF EXISTS chk_orders_total;
