-- Data integrity: prevent invalid prices and stock at the database level.
ALTER TABLE products ADD CONSTRAINT chk_products_price CHECK (price > 0);
ALTER TABLE products ADD CONSTRAINT chk_products_stock CHECK (stock >= 0);

-- Partial index for low-stock inventory alerting queries.
-- Only indexes rows with stock < 10, keeping the index small.
CREATE INDEX idx_products_low_stock ON products (stock) WHERE stock < 10;
