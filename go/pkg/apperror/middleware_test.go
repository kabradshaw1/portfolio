package apperror

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestErrorHandler_AppError(t *testing.T) {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("requestId", "req-123")
		c.Next()
	})
	r.Use(ErrorHandler())
	r.GET("/test", func(c *gin.Context) {
		_ = c.Error(NotFound("PRODUCT_NOT_FOUND", "product not found"))
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}

	var resp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error.Code != "PRODUCT_NOT_FOUND" {
		t.Errorf("code = %q", resp.Error.Code)
	}
	if resp.Error.Message != "product not found" {
		t.Errorf("message = %q", resp.Error.Message)
	}
	if resp.Error.RequestID != "req-123" {
		t.Errorf("request_id = %q", resp.Error.RequestID)
	}
}

func TestErrorHandler_UnknownError(t *testing.T) {
	r := gin.New()
	r.Use(ErrorHandler())
	r.GET("/test", func(c *gin.Context) {
		_ = c.Error(errors.New("db connection failed"))
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}

	var resp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error.Code != "INTERNAL_ERROR" {
		t.Errorf("code = %q", resp.Error.Code)
	}
	if resp.Error.Message != "an unexpected error occurred" {
		t.Errorf("message = %q, want hidden", resp.Error.Message)
	}
}

func TestErrorHandler_NoError(t *testing.T) {
	r := gin.New()
	r.Use(ErrorHandler())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestErrorHandler_ValidationError(t *testing.T) {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("requestId", "req-val")
		c.Next()
	})
	r.Use(ErrorHandler())
	r.POST("/test", func(c *gin.Context) {
		_ = c.Error(Validation([]FieldError{
			{Field: "quantity", Message: "must be between 1 and 99"},
		}))
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", w.Code)
	}

	var resp ValidationErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error.Code != "VALIDATION_ERROR" {
		t.Errorf("code = %q, want VALIDATION_ERROR", resp.Error.Code)
	}
	if resp.Error.Message != "validation failed" {
		t.Errorf("message = %q, want validation failed", resp.Error.Message)
	}
	if resp.Error.RequestID != "req-val" {
		t.Errorf("request_id = %q, want req-val", resp.Error.RequestID)
	}
	if len(resp.Error.Fields) != 1 {
		t.Fatalf("len(fields) = %d, want 1", len(resp.Error.Fields))
	}
	if resp.Error.Fields[0].Field != "quantity" {
		t.Errorf("fields[0].field = %q, want quantity", resp.Error.Fields[0].Field)
	}
	if resp.Error.Fields[0].Message != "must be between 1 and 99" {
		t.Errorf("fields[0].message = %q", resp.Error.Fields[0].Message)
	}
}

func TestErrorHandler_MissingRequestID(t *testing.T) {
	r := gin.New()
	r.Use(ErrorHandler())
	r.GET("/test", func(c *gin.Context) {
		_ = c.Error(BadRequest("VALIDATION_ERROR", "bad input"))
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}

	var resp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error.RequestID != "" {
		t.Errorf("request_id = %q, want empty", resp.Error.RequestID)
	}
}
