package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/metrics"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/model"
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
	}

	products, total, err := h.svc.List(c.Request.Context(), params)
	if err != nil {
		_ = c.Error(err)
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
