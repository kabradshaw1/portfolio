-- Daily revenue by product and category
CREATE MATERIALIZED VIEW mv_daily_revenue AS
SELECT
    date_trunc('day', o.created_at)::date AS day,
    oi.product_id,
    p.name AS product_name,
    p.category,
    SUM(oi.quantity * oi.price_at_purchase) AS revenue_cents,
    SUM(oi.quantity) AS units_sold,
    COUNT(DISTINCT o.id) AS order_count
FROM orders o
JOIN order_items oi ON oi.order_id = o.id
JOIN products p ON p.id = oi.product_id
WHERE o.status = 'completed'
GROUP BY 1, 2, 3, 4;

-- Unique index required for REFRESH MATERIALIZED VIEW CONCURRENTLY
-- migration-lint: ignore=MIG001 reason="materialized view created in same migration above; index target is empty at creation time"
CREATE UNIQUE INDEX idx_mv_daily_revenue_pk ON mv_daily_revenue (day, product_id);

-- Product performance: units, revenue, return rate, AOV
CREATE MATERIALIZED VIEW mv_product_performance AS
SELECT
    p.id AS product_id,
    p.name AS product_name,
    p.category,
    p.stock AS current_stock,
    COALESCE(SUM(oi.quantity), 0) AS total_units_sold,
    COALESCE(SUM(oi.quantity * oi.price_at_purchase), 0) AS total_revenue_cents,
    COUNT(DISTINCT o.id) AS total_orders,
    CASE
        WHEN COUNT(DISTINCT o.id) > 0
        THEN COALESCE(SUM(oi.quantity * oi.price_at_purchase), 0) / COUNT(DISTINCT o.id)
        ELSE 0
    END AS avg_order_value_cents,
    COALESCE(r.return_count, 0) AS return_count,
    CASE
        WHEN COUNT(DISTINCT o.id) > 0
        THEN ROUND(COALESCE(r.return_count, 0)::numeric / COUNT(DISTINCT o.id) * 100, 2)
        ELSE 0
    END AS return_rate_pct
FROM products p
LEFT JOIN order_items oi ON oi.product_id = p.id
LEFT JOIN orders o ON o.id = oi.order_id AND o.status = 'completed'
LEFT JOIN (
    SELECT oi2.product_id, COUNT(*) AS return_count
    FROM returns ret
    JOIN order_items oi2 ON oi2.order_id = ret.order_id
    WHERE ret.status = 'approved'
    GROUP BY oi2.product_id
) r ON r.product_id = p.id
GROUP BY p.id, p.name, p.category, p.stock, r.return_count;

-- migration-lint: ignore=MIG001 reason="materialized view created in same migration above; index target is empty at creation time"
CREATE UNIQUE INDEX idx_mv_product_performance_pk ON mv_product_performance (product_id);

-- Customer summary: CLV proxy
CREATE MATERIALIZED VIEW mv_customer_summary AS
SELECT
    o.user_id,
    COUNT(DISTINCT o.id) AS order_count,
    COALESCE(SUM(o.total), 0) AS total_spend_cents,
    MIN(o.created_at) AS first_order_at,
    MAX(o.created_at) AS last_order_at,
    CASE
        WHEN COUNT(DISTINCT o.id) > 0
        THEN COALESCE(SUM(o.total), 0) / COUNT(DISTINCT o.id)
        ELSE 0
    END AS avg_order_value_cents
FROM orders o
WHERE o.status = 'completed'
GROUP BY o.user_id;

-- migration-lint: ignore=MIG001 reason="materialized view created in same migration above; index target is empty at creation time"
CREATE UNIQUE INDEX idx_mv_customer_summary_pk ON mv_customer_summary (user_id);
