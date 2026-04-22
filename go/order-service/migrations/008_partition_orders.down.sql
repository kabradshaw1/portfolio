-- Reverse: re-create non-partitioned table and copy data back
ALTER TABLE order_items DROP CONSTRAINT IF EXISTS order_items_order_id_fkey;

CREATE TABLE orders_flat (
    id         UUID PRIMARY KEY,
    user_id    UUID NOT NULL,
    status     VARCHAR(20) NOT NULL DEFAULT 'pending',
    saga_step  VARCHAR(20),
    total      INTEGER NOT NULL CHECK (total > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO orders_flat (id, user_id, status, saga_step, total, created_at, updated_at)
SELECT id, user_id, status, saga_step, total, created_at, updated_at FROM orders;

DROP TABLE orders;
ALTER TABLE orders_flat RENAME TO orders;

CREATE INDEX idx_orders_user ON orders(user_id);
CREATE INDEX idx_orders_status ON orders(status);
CREATE INDEX idx_orders_saga_step ON orders(saga_step);

ALTER TABLE order_items ADD CONSTRAINT order_items_order_id_fkey
    FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE CASCADE;

ALTER TABLE returns ADD CONSTRAINT returns_order_id_fkey
    FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE CASCADE;
