package model

import (
	"time"

	"github.com/google/uuid"
)

type Product struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Price       int       `json:"price"`
	Category    string    `json:"category"`
	ImageURL    string    `json:"imageUrl"`
	Stock       int       `json:"stock"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type ProductListParams struct {
	Category string
	Query    string
	Sort     string
	Page     int
	Limit    int
	Cursor   string
}

type ProductListResponse struct {
	Products   []Product `json:"products"`
	Total      int       `json:"total,omitempty"`      // only in offset mode
	Page       int       `json:"page,omitempty"`       // only in offset mode
	Limit      int       `json:"limit"`
	NextCursor string    `json:"nextCursor,omitempty"`
	HasMore    bool      `json:"hasMore"`
}
