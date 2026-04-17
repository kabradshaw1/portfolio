package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/validate"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

type ReturnServiceInterface interface {
	Initiate(ctx context.Context, userID, orderID uuid.UUID, itemIDs []string, reason string) (*model.Return, error)
}

type ReturnHandler struct {
	svc ReturnServiceInterface
}

func NewReturnHandler(svc ReturnServiceInterface) *ReturnHandler {
	return &ReturnHandler{svc: svc}
}

func (h *ReturnHandler) Initiate(c *gin.Context) {
	userID, err := uuid.Parse(c.GetString("userId"))
	if err != nil {
		_ = c.Error(apperror.BadRequest("INVALID_ID", "invalid user ID"))
		return
	}
	orderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		_ = c.Error(apperror.BadRequest("INVALID_ID", "invalid order ID"))
		return
	}
	var req model.InitiateReturnRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(apperror.BadRequest("INVALID_JSON", "invalid request body"))
		return
	}

	if errs := validate.InitiateReturn(req.ItemIDs, req.Reason); len(errs) > 0 {
		_ = c.Error(apperror.Validation(errs))
		return
	}

	ret, err := h.svc.Initiate(c.Request.Context(), userID, orderID, req.ItemIDs, req.Reason)
	if err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, ret)
}
