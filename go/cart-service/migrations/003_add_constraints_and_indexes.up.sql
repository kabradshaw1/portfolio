-- Composite index for Reserve/Release queries that filter on (user_id, reserved).
-- Without this, the UPDATE scans all cart_items for the user then filters by reserved.
CREATE INDEX idx_cart_items_user_reserved ON cart_items (user_id, reserved);
