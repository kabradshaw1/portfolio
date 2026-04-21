CREATE INDEX IF NOT EXISTS idx_products_price_id ON products (price, id);
CREATE INDEX IF NOT EXISTS idx_products_name_id ON products (name, id);
CREATE INDEX IF NOT EXISTS idx_products_created_at_id ON products (created_at DESC, id DESC);
