-- 005_add_product_search_index.up.sql
--
-- Adds a trigram GIN index on products.name to accelerate ILIKE-based
-- product-name search.
--
-- Pattern: CREATE INDEX CONCURRENTLY in its own migration file. The file
-- intentionally contains exactly one statement — see the runbook recipe.
-- See docs/runbooks/postgres-migrations.md (recipe 4) for why.

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_products_name_trgm
  ON products USING gin (name gin_trgm_ops);
