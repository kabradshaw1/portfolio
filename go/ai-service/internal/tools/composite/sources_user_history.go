package composite

import (
	"context"
	"database/sql"
)

// PostgresUserHistory pulls a user's order history and cart items from
// Postgres. The order query joins orders→order_items because order_items
// does not carry user_id directly; the join is the only way to filter by
// user. CartItems queries cart_items directly — it does carry user_id.
//
// Neither table stores a product name, so Name is always empty in v1.
// When product names are needed (e.g. for richer rationale text), add a
// join to the products table once that FK is stable.
//
// RecentlyViewed always returns nil — no recently_viewed schema exists yet.
// Swap it for a real query once the table is added.
type PostgresUserHistory struct {
	OrdersDB *sql.DB // orderdb: holds orders + order_items
	CartDB   *sql.DB // cartdb: holds cart_items
}

// Orders returns all order-line-item signals for the given user.
// It joins orders → order_items so that the user_id filter works across the
// table boundary. Each row becomes one HistoricalItem with Source = "order:<order_id>".
func (p PostgresUserHistory) Orders(ctx context.Context, userID string) ([]HistoricalItem, error) {
	// Empty userID short-circuits to nil rather than round-tripping the DB.
	// Unlike sibling adapters in sources_postgres.go (which let an empty arg
	// run safely against a single-table query), Orders runs a JOIN whose cost
	// is non-trivial — guarding here keeps the recommend tool snappy for
	// anonymous sessions where the userID arrives empty.
	if userID == "" {
		return nil, nil
	}
	const q = `
		SELECT oi.order_id, oi.product_id
		FROM order_items oi
		JOIN orders o ON o.id = oi.order_id
		WHERE o.user_id = $1
		ORDER BY o.created_at DESC
		LIMIT 200`
	rows, err := p.OrdersDB.QueryContext(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []HistoricalItem
	for rows.Next() {
		var it HistoricalItem
		var orderID string
		if err := rows.Scan(&orderID, &it.ProductID); err != nil {
			return nil, err
		}
		it.Source = "order:" + orderID
		out = append(out, it)
	}
	return out, rows.Err()
}

// CartItems returns all current cart entries for the given user.
// The Source is set to "cart:current" for every row — there is no
// distinct cart-session concept in the schema.
func (p PostgresUserHistory) CartItems(ctx context.Context, userID string) ([]HistoricalItem, error) {
	// Empty userID short-circuits to nil for consistency with Orders.
	if userID == "" {
		return nil, nil
	}
	const q = `SELECT product_id FROM cart_items WHERE user_id = $1`
	rows, err := p.CartDB.QueryContext(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []HistoricalItem
	for rows.Next() {
		var it HistoricalItem
		if err := rows.Scan(&it.ProductID); err != nil {
			return nil, err
		}
		it.Source = "cart:current"
		out = append(out, it)
	}
	return out, rows.Err()
}

// RecentlyViewed always returns nil — no recently_viewed table exists in
// the current schema. The recommend_with_rationale tool degrades gracefully
// when this source is empty; the embedding-average path simply uses order
// and cart signals only.
func (p PostgresUserHistory) RecentlyViewed(_ context.Context, _ string) ([]HistoricalItem, error) {
	return nil, nil
}
