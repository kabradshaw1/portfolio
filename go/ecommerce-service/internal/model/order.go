package model

import (
	"time"

	"github.com/google/uuid"
)

type OrderStatus string

const (
	OrderStatusPending    OrderStatus = "pending"
	OrderStatusProcessing OrderStatus = "processing"
	OrderStatusCompleted  OrderStatus = "completed"
	OrderStatusFailed     OrderStatus = "failed"
)

type Order struct {
	ID        uuid.UUID   `json:"id"`
	UserID    uuid.UUID   `json:"userId"`
	Status    OrderStatus `json:"status"`
	Total     int         `json:"total"`
	CreatedAt time.Time   `json:"createdAt"`
	UpdatedAt time.Time   `json:"updatedAt"`
	Items     []OrderItem `json:"items,omitempty"`
}

type OrderItem struct {
	ID              uuid.UUID `json:"id"`
	OrderID         uuid.UUID `json:"orderId"`
	ProductID       uuid.UUID `json:"productId"`
	Quantity        int       `json:"quantity"`
	PriceAtPurchase int       `json:"priceAtPurchase"`
	ProductName     string    `json:"productName,omitempty"`
}

type OrderMessage struct {
	OrderID string `json:"orderId"`
}

type OrderListParams struct {
	Cursor string
	Limit  int
}

type OrderListResponse struct {
	Orders     []Order `json:"orders"`
	NextCursor string  `json:"nextCursor,omitempty"`
	HasMore    bool    `json:"hasMore"`
}
