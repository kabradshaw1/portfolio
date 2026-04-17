package handler

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/metrics"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/pagination"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/validate"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
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
	query := c.Query("q")
	sort := c.DefaultQuery("sort", "created_at_desc")
	cursor := c.Query("cursor")

	if errs := validate.ProductListParams(sort, page, limit); len(errs) > 0 {
		_ = c.Error(apperror.Validation(errs))
		return
	}

	params := model.ProductListParams{
		Category: category,
		Query:    query,
		Sort:     sort,
		Page:     page,
		Limit:    limit,
		Cursor:   cursor,
	}

	products, total, err := h.svc.List(c.Request.Context(), params)
	if err != nil {
		_ = c.Error(err)
		return
	}

	if cursor != "" {
		// Cursor-based pagination response
		hasMore := len(products) > limit
		if hasMore {
			products = products[:limit]
		}
		var nextCursor string
		if hasMore && len(products) > 0 {
			nextCursor = buildProductCursor(products[len(products)-1], sort)
		}
		c.JSON(http.StatusOK, model.ProductListResponse{
			Products:   products,
			Limit:      limit,
			NextCursor: nextCursor,
			HasMore:    hasMore,
		})
		return
	}

	// Offset-based pagination response with forward-compatibility cursor
	var nextCursor string
	hasMore := total > page*limit
	if hasMore && len(products) > 0 {
		nextCursor = buildProductCursor(products[len(products)-1], sort)
	}
	c.JSON(http.StatusOK, model.ProductListResponse{
		Products:   products,
		Total:      total,
		Page:       page,
		Limit:      limit,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	})
}

// buildProductCursor encodes the last product in a page into a cursor for the next page.
func buildProductCursor(p model.Product, sort string) string {
	switch sort {
	case "price_asc", "price_desc":
		return pagination.EncodeCursor(fmt.Sprintf("%d", p.Price), p.ID)
	case "name_asc":
		return pagination.EncodeCursor(p.Name, p.ID)
	default: // created_at_desc
		return pagination.EncodeCursor(p.CreatedAt.Format(time.RFC3339Nano), p.ID)
	}
}

func (h *ProductHandler) GetByID(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		_ = c.Error(apperror.BadRequest("INVALID_ID", "invalid product ID"))
		return
	}

	product, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		_ = c.Error(err)
		return
	}

	metrics.ProductViews.Inc()
	c.JSON(http.StatusOK, product)
}

func (h *ProductHandler) Categories(c *gin.Context) {
	categories, err := h.svc.Categories(c.Request.Context())
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"categories": categories})
}
