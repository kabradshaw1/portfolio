package model

import (
	"time"

	"github.com/google/uuid"
)

type Return struct {
	ID        uuid.UUID `json:"id"`
	OrderID   uuid.UUID `json:"orderId"`
	UserID    uuid.UUID `json:"userId"`
	Status    string    `json:"status"`
	Reason    string    `json:"reason"`
	ItemIDs   []string  `json:"itemIds"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type InitiateReturnRequest struct {
	ItemIDs []string `json:"itemIds" binding:"required"`
	Reason  string   `json:"reason"  binding:"required"`
}
