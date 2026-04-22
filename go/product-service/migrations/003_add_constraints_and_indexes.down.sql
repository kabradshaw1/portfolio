DROP INDEX IF EXISTS idx_products_low_stock;
ALTER TABLE products DROP CONSTRAINT IF EXISTS chk_products_stock;
ALTER TABLE products DROP CONSTRAINT IF EXISTS chk_products_price;
