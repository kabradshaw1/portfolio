package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/kabradshaw1/portfolio/go/order-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

type fakeProductService struct {
	gotParams model.ProductListParams
}

func (f *fakeProductService) List(ctx context.Context, params model.ProductListParams) ([]model.Product, int, error) {
	f.gotParams = params
	return nil, 0, nil
}
func (f *fakeProductService) GetByID(ctx context.Context, id uuid.UUID) (*model.Product, error) {
	return nil, nil
}
func (f *fakeProductService) Categories(ctx context.Context) ([]string, error) {
	return nil, nil
}

func productTestRouter() *gin.Engine {
	r := gin.New()
	r.Use(apperror.ErrorHandler())
	return r
}

func TestProductHandler_List_ForwardsQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &fakeProductService{}
	h := NewProductHandler(svc)

	r := productTestRouter()
	r.GET("/products", h.List)

	req := httptest.NewRequest(http.MethodGet, "/products?q=jacket&limit=5", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	if svc.gotParams.Query != "jacket" {
		t.Errorf("expected Query=jacket, got %q", svc.gotParams.Query)
	}
	if svc.gotParams.Limit != 5 {
		t.Errorf("expected Limit=5, got %d", svc.gotParams.Limit)
	}

	// Also sanity check the response JSON shape
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if _, ok := body["products"]; !ok {
		t.Errorf("expected products key in body")
	}
}
