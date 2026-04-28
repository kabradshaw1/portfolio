-- Composite index for Reserve/Release queries that filter on (user_id, reserved).
-- Without this, the UPDATE scans all cart_items for the user then filters by reserved.
-- migration-lint: ignore=MIG001 reason="early-stage migration; ran on empty cart_items in dev/QA before launch — would be CONCURRENTLY today"
CREATE INDEX idx_cart_items_user_reserved ON cart_items (user_id, reserved);
