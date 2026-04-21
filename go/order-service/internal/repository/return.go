package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	gobreaker "github.com/sony/gobreaker/v2"

	"github.com/kabradshaw1/portfolio/go/order-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
)

type ReturnRepository struct {
	pool     *pgxpool.Pool
	breaker  *gobreaker.CircuitBreaker[any]
	retryCfg resilience.RetryConfig
}

func NewReturnRepository(pool *pgxpool.Pool, breaker *gobreaker.CircuitBreaker[any]) *ReturnRepository {
	cfg := resilience.DefaultRetryConfig()
	cfg.IsRetryable = resilience.IsPgRetryable
	return &ReturnRepository{pool: pool, breaker: breaker, retryCfg: cfg}
}

func (r *ReturnRepository) Create(ctx context.Context, orderID, userID uuid.UUID, itemIDs []string, reason string) (*model.Return, error) {
	return resilience.Call(ctx, r.breaker, r.retryCfg, func(ctx context.Context) (*model.Return, error) {
		itemsJSON, err := json.Marshal(itemIDs)
		if err != nil {
			return nil, fmt.Errorf("marshal itemIDs: %w", err)
		}

		var ret model.Return
		var returnedItems []byte
		err = r.pool.QueryRow(ctx,
			`INSERT INTO returns (order_id, user_id, status, reason, item_ids)
			 VALUES ($1, $2, 'requested', $3, $4)
			 RETURNING id, order_id, user_id, status, reason, item_ids, created_at, updated_at`,
			orderID, userID, reason, itemsJSON,
		).Scan(&ret.ID, &ret.OrderID, &ret.UserID, &ret.Status, &ret.Reason, &returnedItems, &ret.CreatedAt, &ret.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("insert return: %w", err)
		}
		if err := json.Unmarshal(returnedItems, &ret.ItemIDs); err != nil {
			return nil, fmt.Errorf("unmarshal item_ids: %w", err)
		}
		return &ret, nil
	})
}
