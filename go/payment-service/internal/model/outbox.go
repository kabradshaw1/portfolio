package model

import (
	"time"

	"github.com/google/uuid"
)

type OutboxMessage struct {
	ID         uuid.UUID `json:"id"`
	Exchange   string    `json:"exchange"`
	RoutingKey string    `json:"routingKey"`
	Payload    []byte    `json:"payload"`
	Published  bool      `json:"published"`
	CreatedAt  time.Time `json:"createdAt"`
}
