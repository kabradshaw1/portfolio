package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/model"
)

type ProductServiceInterface interface {
	List(ctx context.Context, params model.ProductListParams) ([]model.Product, int, error)
	GetByID(ctx context.Context, id uuid.UUID) (*model.Product, error)
	Categories(ctx context.Context) ([]string, error)
}

type ProductHandler struct {
	svc ProductServiceInterface
}

func NewProductHandler(svc ProductServiceInterface) *ProductHandler {
	return &ProductHandler{svc: svc}
}

func (h *ProductHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	category := c.Query("category")
	sort := c.DefaultQuery("sort", "created_at_desc")

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	params := model.ProductListParams{
		Category: category,
		Sort:     sort,
		Page:     page,
		Limit:    limit,
	}

	products, total, err := h.svc.List(c.Request.Context(), params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list products"})
		return
	}

	c.JSON(http.StatusOK, model.ProductListResponse{
		Products: products,
		Total:    total,
		Page:     page,
		Limit:    limit,
	})
}

func (h *ProductHandler) GetByID(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid product ID"})
		return
	}

	product, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "product not found"})
		return
	}

	c.JSON(http.StatusOK, product)
}

func (h *ProductHandler) Categories(c *gin.Context) {
	categories, err := h.svc.Categories(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list categories"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"categories": categories})
}
