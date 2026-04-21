package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/kabradshaw1/portfolio/go/order-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/service"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

type fakeReturnService struct {
	gotUser uuid.UUID
	gotOrd  uuid.UUID
	gotIDs  []string
	gotRsn  string
	ret     *model.Return
	err     error
}

func (f *fakeReturnService) Initiate(ctx context.Context, userID, orderID uuid.UUID, itemIDs []string, reason string) (*model.Return, error) {
	f.gotUser = userID
	f.gotOrd = orderID
	f.gotIDs = itemIDs
	f.gotRsn = reason
	return f.ret, f.err
}

func returnTestRouter() *gin.Engine {
	r := gin.New()
	r.Use(apperror.ErrorHandler())
	return r
}

func TestReturnHandler_Initiate_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	user := uuid.New()
	order := uuid.New()
	svc := &fakeReturnService{ret: &model.Return{ID: uuid.New(), OrderID: order, UserID: user, Status: "requested", Reason: "doesn't fit"}}
	h := NewReturnHandler(svc)

	r := returnTestRouter()
	r.POST("/orders/:id/returns", func(c *gin.Context) {
		c.Set("userId", user.String())
		h.Initiate(c)
	})

	body := strings.NewReader(`{"itemIds":["i1"],"reason":"doesn't fit"}`)
	req := httptest.NewRequest(http.MethodPost, "/orders/"+order.String()+"/returns", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if svc.gotUser != user || svc.gotOrd != order {
		t.Errorf("ids = %v %v", svc.gotUser, svc.gotOrd)
	}
	if svc.gotRsn != "doesn't fit" || len(svc.gotIDs) != 1 {
		t.Errorf("svc args = %+v", svc)
	}
}

func TestReturnHandler_Initiate_NotOwned(t *testing.T) {
	gin.SetMode(gin.TestMode)
	user := uuid.New()
	order := uuid.New()
	svc := &fakeReturnService{err: service.ErrOrderNotOwned}
	h := NewReturnHandler(svc)

	r := returnTestRouter()
	r.POST("/orders/:id/returns", func(c *gin.Context) {
		c.Set("userId", user.String())
		h.Initiate(c)
	})

	body := strings.NewReader(`{"itemIds":["i1"],"reason":"r"}`)
	req := httptest.NewRequest(http.MethodPost, "/orders/"+order.String()+"/returns", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestReturnHandler_Initiate_BadBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &fakeReturnService{}
	h := NewReturnHandler(svc)

	r := returnTestRouter()
	user := uuid.New()
	order := uuid.New()
	r.POST("/orders/:id/returns", func(c *gin.Context) {
		c.Set("userId", user.String())
		h.Initiate(c)
	})

	body := strings.NewReader(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/orders/"+order.String()+"/returns", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
