package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/kabradshaw1/portfolio/go/cart-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/cart-service/internal/validate"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

type CartServiceInterface interface {
	GetCart(ctx context.Context, userID uuid.UUID) ([]model.CartItem, error)
	AddItem(ctx context.Context, userID, productID uuid.UUID, quantity int) (*model.CartItem, error)
	UpdateQuantity(ctx context.Context, itemID, userID uuid.UUID, quantity int) error
	RemoveItem(ctx context.Context, itemID, userID uuid.UUID) error
}

type CartHandler struct {
	svc CartServiceInterface
}

func NewCartHandler(svc CartServiceInterface) *CartHandler {
	return &CartHandler{svc: svc}
}

func (h *CartHandler) GetCart(c *gin.Context) {
	userID, err := uuid.Parse(c.GetString("userId"))
	if err != nil {
		_ = c.Error(apperror.BadRequest("INVALID_ID", "invalid user ID"))
		return
	}

	items, err := h.svc.GetCart(c.Request.Context(), userID)
	if err != nil {
		_ = c.Error(err)
		return
	}

	var total int
	for _, item := range items {
		total += item.ProductPrice * item.Quantity
	}

	c.JSON(http.StatusOK, model.CartResponse{
		Items: items,
		Total: total,
	})
}

func (h *CartHandler) AddItem(c *gin.Context) {
	userID, err := uuid.Parse(c.GetString("userId"))
	if err != nil {
		_ = c.Error(apperror.BadRequest("INVALID_ID", "invalid user ID"))
		return
	}

	var req model.AddToCartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(apperror.BadRequest("INVALID_JSON", "invalid request body"))
		return
	}

	if errs := validate.AddToCart(req.ProductID, req.Quantity); len(errs) > 0 {
		_ = c.Error(apperror.Validation(errs))
		return
	}

	productID, _ := uuid.Parse(req.ProductID)

	item, err := h.svc.AddItem(c.Request.Context(), userID, productID, req.Quantity)
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusCreated, item)
}

func (h *CartHandler) UpdateQuantity(c *gin.Context) {
	userID, err := uuid.Parse(c.GetString("userId"))
	if err != nil {
		_ = c.Error(apperror.BadRequest("INVALID_ID", "invalid user ID"))
		return
	}

	itemID, err := uuid.Parse(c.Param("itemId"))
	if err != nil {
		_ = c.Error(apperror.BadRequest("INVALID_ID", "invalid item ID"))
		return
	}

	var req model.UpdateCartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(apperror.BadRequest("INVALID_JSON", "invalid request body"))
		return
	}

	if errs := validate.UpdateCart(req.Quantity); len(errs) > 0 {
		_ = c.Error(apperror.Validation(errs))
		return
	}

	if err := h.svc.UpdateQuantity(c.Request.Context(), itemID, userID, req.Quantity); err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "quantity updated"})
}

func (h *CartHandler) RemoveItem(c *gin.Context) {
	userID, err := uuid.Parse(c.GetString("userId"))
	if err != nil {
		_ = c.Error(apperror.BadRequest("INVALID_ID", "invalid user ID"))
		return
	}

	itemID, err := uuid.Parse(c.Param("itemId"))
	if err != nil {
		_ = c.Error(apperror.BadRequest("INVALID_ID", "invalid item ID"))
		return
	}

	if err := h.svc.RemoveItem(c.Request.Context(), itemID, userID); err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "item removed"})
}
