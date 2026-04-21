package service

import (
	"context"

	"github.com/google/uuid"

	"github.com/kabradshaw1/portfolio/go/order-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

var ErrOrderNotOwned = apperror.NotFound("ORDER_NOT_FOUND", "order not found")

type ReturnRepositoryInterface interface {
	Create(ctx context.Context, orderID, userID uuid.UUID, itemIDs []string, reason string) (*model.Return, error)
}

type OrderLookup interface {
	GetOrder(ctx context.Context, orderID uuid.UUID) (*model.Order, error)
}

type ReturnService struct {
	returns ReturnRepositoryInterface
	orders  OrderLookup
}

func NewReturnService(returns ReturnRepositoryInterface, orders OrderLookup) *ReturnService {
	return &ReturnService{returns: returns, orders: orders}
}

// Initiate verifies the order belongs to userID before creating the return.
func (s *ReturnService) Initiate(ctx context.Context, userID, orderID uuid.UUID, itemIDs []string, reason string) (*model.Return, error) {
	order, err := s.orders.GetOrder(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if order.UserID != userID {
		return nil, ErrOrderNotOwned
	}
	return s.returns.Create(ctx, orderID, userID, itemIDs, reason)
}
