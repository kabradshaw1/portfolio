-- Step 1: Rename existing orders table
ALTER TABLE order_items DROP CONSTRAINT IF EXISTS order_items_order_id_fkey;
ALTER TABLE orders RENAME TO orders_old;

-- Step 2: Create partitioned orders table with same schema
CREATE TABLE orders (
    id         UUID NOT NULL,
    user_id    UUID NOT NULL,
    status     VARCHAR(20) NOT NULL DEFAULT 'pending',
    saga_step  VARCHAR(20),
    total      INTEGER NOT NULL CHECK (total > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- Step 3: Create partitions (monthly, 2026-01 through 2027-06)
CREATE TABLE orders_2026_01 PARTITION OF orders FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');
CREATE TABLE orders_2026_02 PARTITION OF orders FOR VALUES FROM ('2026-02-01') TO ('2026-03-01');
CREATE TABLE orders_2026_03 PARTITION OF orders FOR VALUES FROM ('2026-03-01') TO ('2026-04-01');
CREATE TABLE orders_2026_04 PARTITION OF orders FOR VALUES FROM ('2026-04-01') TO ('2026-05-01');
CREATE TABLE orders_2026_05 PARTITION OF orders FOR VALUES FROM ('2026-05-01') TO ('2026-06-01');
CREATE TABLE orders_2026_06 PARTITION OF orders FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');
CREATE TABLE orders_2026_07 PARTITION OF orders FOR VALUES FROM ('2026-07-01') TO ('2026-08-01');
CREATE TABLE orders_2026_08 PARTITION OF orders FOR VALUES FROM ('2026-08-01') TO ('2026-09-01');
CREATE TABLE orders_2026_09 PARTITION OF orders FOR VALUES FROM ('2026-09-01') TO ('2026-10-01');
CREATE TABLE orders_2026_10 PARTITION OF orders FOR VALUES FROM ('2026-10-01') TO ('2026-11-01');
CREATE TABLE orders_2026_11 PARTITION OF orders FOR VALUES FROM ('2026-11-01') TO ('2026-12-01');
CREATE TABLE orders_2026_12 PARTITION OF orders FOR VALUES FROM ('2026-12-01') TO ('2027-01-01');
CREATE TABLE orders_2027_01 PARTITION OF orders FOR VALUES FROM ('2027-01-01') TO ('2027-02-01');
CREATE TABLE orders_2027_02 PARTITION OF orders FOR VALUES FROM ('2027-02-01') TO ('2027-03-01');
CREATE TABLE orders_2027_03 PARTITION OF orders FOR VALUES FROM ('2027-03-01') TO ('2027-04-01');
CREATE TABLE orders_2027_04 PARTITION OF orders FOR VALUES FROM ('2027-04-01') TO ('2027-05-01');
CREATE TABLE orders_2027_05 PARTITION OF orders FOR VALUES FROM ('2027-05-01') TO ('2027-06-01');
CREATE TABLE orders_2027_06 PARTITION OF orders FOR VALUES FROM ('2027-06-01') TO ('2027-07-01');

-- Default partition for anything outside the defined ranges
CREATE TABLE orders_default PARTITION OF orders DEFAULT;

-- Step 4: Copy data from old table
INSERT INTO orders (id, user_id, status, saga_step, total, created_at, updated_at)
SELECT id, user_id, status, saga_step, total, created_at, updated_at FROM orders_old;

-- Step 5: Re-create indexes on partitioned table
CREATE INDEX idx_orders_user ON orders(user_id);
CREATE INDEX idx_orders_status ON orders(status);
CREATE INDEX idx_orders_saga_step ON orders(saga_step);
CREATE INDEX idx_orders_created_at ON orders(created_at);
-- Composite index for cursor pagination (keyset)
CREATE INDEX idx_orders_user_cursor ON orders(user_id, created_at DESC, id DESC);

-- Step 6: Re-create FK from order_items
ALTER TABLE order_items ADD CONSTRAINT order_items_order_id_fkey
    FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE CASCADE;

-- Step 7: Drop old table
DROP TABLE orders_old;
