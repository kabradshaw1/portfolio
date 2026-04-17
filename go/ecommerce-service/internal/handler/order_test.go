package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

type fakeOrderService struct {
	order *model.Order
}

func (f *fakeOrderService) Checkout(ctx context.Context, userID uuid.UUID) (*model.Order, error) {
	return nil, nil
}
func (f *fakeOrderService) GetOrder(ctx context.Context, orderID uuid.UUID) (*model.Order, error) {
	return f.order, nil
}
func (f *fakeOrderService) ListOrders(ctx context.Context, userID uuid.UUID, params model.OrderListParams) ([]model.Order, error) {
	return nil, nil
}

func orderTestRouter() *gin.Engine {
	r := gin.New()
	r.Use(apperror.ErrorHandler())
	return r
}

func TestOrderHandler_GetByID_ForbidsOtherUsers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	owner := uuid.New()
	other := uuid.New()
	orderID := uuid.New()
	svc := &fakeOrderService{order: &model.Order{ID: orderID, UserID: owner}}
	h := NewOrderHandler(svc)

	r := orderTestRouter()
	r.GET("/orders/:id", func(c *gin.Context) {
		c.Set("userId", other.String())
		h.GetByID(c)
	})

	req := httptest.NewRequest(http.MethodGet, "/orders/"+orderID.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for non-owner, got %d", w.Code)
	}
}

func TestOrderHandler_GetByID_AllowsOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	owner := uuid.New()
	orderID := uuid.New()
	svc := &fakeOrderService{order: &model.Order{ID: orderID, UserID: owner}}
	h := NewOrderHandler(svc)

	r := orderTestRouter()
	r.GET("/orders/:id", func(c *gin.Context) {
		c.Set("userId", owner.String())
		h.GetByID(c)
	})

	req := httptest.NewRequest(http.MethodGet, "/orders/"+orderID.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for owner, got %d", w.Code)
	}
}
