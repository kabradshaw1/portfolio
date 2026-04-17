# Ecommerce Service Production Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Harden the Go ecommerce service with input validation, idempotency keys, cursor-based pagination, and integration tests.

**Architecture:** Extend the existing handler → service → repo layered architecture. Add a `validate` package for structured input validation, `pagination` package for cursor encoding, and `idempotency` middleware backed by Redis. Integration tests use testcontainers-go for real Postgres/Redis/RabbitMQ.

**Tech Stack:** Go, Gin, pgx, go-redis, amqp091-go, testcontainers-go, existing `go/pkg/` (apperror, resilience, tracing)

**Spec:** `docs/superpowers/specs/2026-04-16-ecommerce-production-hardening-design.md`

---

## File Map

### New Files
| File | Responsibility |
|------|---------------|
| `go/pkg/apperror/validation.go` | `FieldError` type, `Validation()` constructor |
| `go/pkg/apperror/validation_test.go` | Unit tests for validation error type |
| `go/ecommerce-service/internal/validate/validate.go` | Per-request validators |
| `go/ecommerce-service/internal/validate/validate_test.go` | Validator unit tests |
| `go/ecommerce-service/internal/pagination/cursor.go` | Cursor encode/decode |
| `go/ecommerce-service/internal/pagination/cursor_test.go` | Cursor unit tests |
| `go/ecommerce-service/internal/middleware/idempotency.go` | Idempotency key middleware |
| `go/ecommerce-service/internal/middleware/idempotency_test.go` | Idempotency unit tests |
| `go/ecommerce-service/migrations/005_add_pagination_indexes.up.sql` | Composite index `(price, id)` |
| `go/ecommerce-service/migrations/005_add_pagination_indexes.down.sql` | Drop composite index |
| `go/ecommerce-service/internal/integration/testutil/containers.go` | Testcontainer setup |
| `go/ecommerce-service/internal/integration/testutil/db.go` | Migration runner + seeder |
| `go/ecommerce-service/internal/integration/testutil/helpers.go` | HTTP client + assertions |
| `go/ecommerce-service/internal/integration/repository_test.go` | Repo integration tests |
| `go/ecommerce-service/internal/integration/checkout_test.go` | E2E checkout flow |
| `go/ecommerce-service/internal/integration/idempotency_test.go` | Idempotency integration tests |
| `go/ecommerce-service/internal/integration/pagination_test.go` | Pagination integration tests |
| `go/ecommerce-service/internal/integration/ratelimit_test.go` | Rate limiter integration tests |

### Modified Files
| File | Changes |
|------|---------|
| `go/pkg/apperror/middleware.go` | Handle `ValidationError` with `fields` array in JSON |
| `go/ecommerce-service/internal/handler/cart.go` | Call validators before processing |
| `go/ecommerce-service/internal/handler/product.go` | Call validators, parse cursor param |
| `go/ecommerce-service/internal/handler/order.go` | Parse cursor/limit params |
| `go/ecommerce-service/internal/handler/return.go` | Call validators |
| `go/ecommerce-service/internal/model/product.go` | Add cursor fields to params and response |
| `go/ecommerce-service/internal/model/order.go` | Add `OrderListParams`, `OrderListResponse` |
| `go/ecommerce-service/internal/repository/product.go` | Add cursor-mode query branch |
| `go/ecommerce-service/internal/repository/order.go` | Add pagination to `ListByUser` |
| `go/ecommerce-service/internal/service/product.go` | Pass through cursor params, update cache key |
| `go/ecommerce-service/internal/service/order.go` | Accept + pass pagination params |
| `go/ecommerce-service/cmd/server/main.go` | Wire idempotency middleware to routes |
| `go/ecommerce-service/internal/metrics/metrics.go` | Add idempotency metrics |
| `Makefile` | Add `preflight-go-integration` target |

---

## Task 1: Validation Error Type in apperror

**Files:**
- Create: `go/pkg/apperror/validation.go`
- Create: `go/pkg/apperror/validation_test.go`
- Modify: `go/pkg/apperror/middleware.go:29-38`

- [ ] **Step 1: Write the failing test for FieldError and Validation constructor**

Create `go/pkg/apperror/validation_test.go`:

```go
package apperror_test

import (
	"net/http"
	"testing"

	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

func TestValidation_ReturnsCorrectStatus(t *testing.T) {
	fields := []apperror.FieldError{
		{Field: "quantity", Message: "must be between 1 and 99"},
	}
	err := apperror.Validation(fields)

	if err.HTTPStatus != http.StatusUnprocessableEntity {
		t.Errorf("expected status 422, got %d", err.HTTPStatus)
	}
	if err.Code != "VALIDATION_ERROR" {
		t.Errorf("expected code VALIDATION_ERROR, got %s", err.Code)
	}
}

func TestValidation_StoresFields(t *testing.T) {
	fields := []apperror.FieldError{
		{Field: "quantity", Message: "must be between 1 and 99"},
		{Field: "productId", Message: "required"},
	}
	err := apperror.Validation(fields)

	if len(err.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(err.Fields))
	}
	if err.Fields[0].Field != "quantity" {
		t.Errorf("expected field 'quantity', got '%s'", err.Fields[0].Field)
	}
	if err.Fields[1].Field != "productId" {
		t.Errorf("expected field 'productId', got '%s'", err.Fields[1].Field)
	}
}

func TestValidation_ImplementsError(t *testing.T) {
	fields := []apperror.FieldError{
		{Field: "name", Message: "required"},
	}
	err := apperror.Validation(fields)

	if err.Error() != "validation failed" {
		t.Errorf("expected 'validation failed', got '%s'", err.Error())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/go/pkg && go test ./apperror/ -run TestValidation -v`
Expected: FAIL — `apperror.FieldError` and `apperror.Validation` undefined

- [ ] **Step 3: Implement FieldError and Validation constructor**

Create `go/pkg/apperror/validation.go`:

```go
package apperror

import "net/http"

// FieldError represents a validation failure on a specific request field.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Validation creates an AppError for input validation failures.
// The Fields slice is serialized into the error response by ErrorHandler.
func Validation(fields []FieldError) *AppError {
	return &AppError{
		Code:       "VALIDATION_ERROR",
		Message:    "validation failed",
		HTTPStatus: http.StatusUnprocessableEntity,
		Fields:     fields,
	}
}
```

Add the `Fields` field to `AppError` in `go/pkg/apperror/apperror.go`:

```go
// In AppError struct, add after Err field:
Fields []FieldError `json:"-"`
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/go/pkg && go test ./apperror/ -run TestValidation -v`
Expected: PASS

- [ ] **Step 5: Update ErrorHandler to serialize Fields**

Modify `go/pkg/apperror/middleware.go` — the `ErrorResponse` and `ErrorBody` types need a `Fields` array. Add a `ValidationErrorResponse` type and update the middleware to use it when `Fields` is non-empty.

In `go/pkg/apperror/apperror.go`, add after `ErrorResponse`:

```go
// ValidationErrorBody extends ErrorBody with field-level errors.
type ValidationErrorBody struct {
	Code      string       `json:"code"`
	Message   string       `json:"message"`
	RequestID string       `json:"request_id,omitempty"`
	Fields    []FieldError `json:"fields"`
}

// ValidationErrorResponse is the top-level JSON envelope for validation errors.
type ValidationErrorResponse struct {
	Error ValidationErrorBody `json:"error"`
}
```

In `go/pkg/apperror/middleware.go`, replace the `AppError` branch (lines 31-38):

Old:
```go
		var ae *AppError
		if errors.As(err, &ae) {
			c.JSON(ae.HTTPStatus, ErrorResponse{
				Error: ErrorBody{
					Code:      ae.Code,
					Message:   ae.Message,
					RequestID: rid,
				},
			})
			return
		}
```

New:
```go
		var ae *AppError
		if errors.As(err, &ae) {
			if len(ae.Fields) > 0 {
				c.JSON(ae.HTTPStatus, ValidationErrorResponse{
					Error: ValidationErrorBody{
						Code:      ae.Code,
						Message:   ae.Message,
						RequestID: rid,
						Fields:    ae.Fields,
					},
				})
				return
			}
			c.JSON(ae.HTTPStatus, ErrorResponse{
				Error: ErrorBody{
					Code:      ae.Code,
					Message:   ae.Message,
					RequestID: rid,
				},
			})
			return
		}
```

- [ ] **Step 6: Run all apperror tests**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/go/pkg && go test ./apperror/ -v`
Expected: All PASS (existing middleware tests + new validation tests)

- [ ] **Step 7: Run go mod tidy in pkg and all services**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/go/pkg && go mod tidy && cd ../ecommerce-service && go mod tidy && cd ../auth-service && go mod tidy && cd ../ai-service && go mod tidy`

- [ ] **Step 8: Commit**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer
git add go/pkg/apperror/validation.go go/pkg/apperror/validation_test.go go/pkg/apperror/apperror.go go/pkg/apperror/middleware.go
git commit -m "feat(apperror): add FieldError type and Validation constructor with 422 responses"
```

---

## Task 2: Input Validators

**Files:**
- Create: `go/ecommerce-service/internal/validate/validate.go`
- Create: `go/ecommerce-service/internal/validate/validate_test.go`

- [ ] **Step 1: Write failing tests for all validators**

Create `go/ecommerce-service/internal/validate/validate_test.go`:

```go
package validate_test

import (
	"testing"

	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/validate"
)

func TestValidateAddToCart_ValidInput(t *testing.T) {
	errs := validate.AddToCart("550e8400-e29b-41d4-a716-446655440000", 1)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestValidateAddToCart_EmptyProductID(t *testing.T) {
	errs := validate.AddToCart("", 1)
	if len(errs) != 1 || errs[0].Field != "productId" {
		t.Errorf("expected productId error, got %v", errs)
	}
}

func TestValidateAddToCart_InvalidUUID(t *testing.T) {
	errs := validate.AddToCart("not-a-uuid", 1)
	if len(errs) != 1 || errs[0].Field != "productId" {
		t.Errorf("expected productId error, got %v", errs)
	}
}

func TestValidateAddToCart_QuantityZero(t *testing.T) {
	errs := validate.AddToCart("550e8400-e29b-41d4-a716-446655440000", 0)
	if len(errs) != 1 || errs[0].Field != "quantity" {
		t.Errorf("expected quantity error, got %v", errs)
	}
}

func TestValidateAddToCart_QuantityTooHigh(t *testing.T) {
	errs := validate.AddToCart("550e8400-e29b-41d4-a716-446655440000", 100)
	if len(errs) != 1 || errs[0].Field != "quantity" {
		t.Errorf("expected quantity error, got %v", errs)
	}
}

func TestValidateAddToCart_MultipleErrors(t *testing.T) {
	errs := validate.AddToCart("", 0)
	if len(errs) != 2 {
		t.Errorf("expected 2 errors, got %d: %v", len(errs), errs)
	}
}

func TestValidateUpdateCart_ValidInput(t *testing.T) {
	errs := validate.UpdateCart(5)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestValidateUpdateCart_QuantityZero(t *testing.T) {
	errs := validate.UpdateCart(0)
	if len(errs) != 1 || errs[0].Field != "quantity" {
		t.Errorf("expected quantity error, got %v", errs)
	}
}

func TestValidateUpdateCart_QuantityTooHigh(t *testing.T) {
	errs := validate.UpdateCart(100)
	if len(errs) != 1 || errs[0].Field != "quantity" {
		t.Errorf("expected quantity error, got %v", errs)
	}
}

func TestValidateProductListParams_ValidDefaults(t *testing.T) {
	errs := validate.ProductListParams("created_at_desc", 1, 20)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestValidateProductListParams_InvalidSort(t *testing.T) {
	errs := validate.ProductListParams("invalid_sort", 1, 20)
	if len(errs) != 1 || errs[0].Field != "sort" {
		t.Errorf("expected sort error, got %v", errs)
	}
}

func TestValidateProductListParams_AllValidSorts(t *testing.T) {
	validSorts := []string{"created_at_desc", "price_asc", "price_desc", "name_asc"}
	for _, sort := range validSorts {
		errs := validate.ProductListParams(sort, 1, 20)
		if len(errs) != 0 {
			t.Errorf("sort %q should be valid, got errors: %v", sort, errs)
		}
	}
}

func TestValidateProductListParams_LimitTooHigh(t *testing.T) {
	errs := validate.ProductListParams("created_at_desc", 1, 101)
	if len(errs) != 1 || errs[0].Field != "limit" {
		t.Errorf("expected limit error, got %v", errs)
	}
}

func TestValidateProductListParams_PageZero(t *testing.T) {
	errs := validate.ProductListParams("created_at_desc", 0, 20)
	if len(errs) != 1 || errs[0].Field != "page" {
		t.Errorf("expected page error, got %v", errs)
	}
}

func TestValidateInitiateReturn_ValidInput(t *testing.T) {
	errs := validate.InitiateReturn([]string{"item-1"}, "Defective product")
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestValidateInitiateReturn_EmptyItemIDs(t *testing.T) {
	errs := validate.InitiateReturn([]string{}, "Defective")
	if len(errs) != 1 || errs[0].Field != "itemIds" {
		t.Errorf("expected itemIds error, got %v", errs)
	}
}

func TestValidateInitiateReturn_EmptyReason(t *testing.T) {
	errs := validate.InitiateReturn([]string{"item-1"}, "")
	if len(errs) != 1 || errs[0].Field != "reason" {
		t.Errorf("expected reason error, got %v", errs)
	}
}

func TestValidateInitiateReturn_ReasonTooLong(t *testing.T) {
	longReason := make([]byte, 501)
	for i := range longReason {
		longReason[i] = 'a'
	}
	errs := validate.InitiateReturn([]string{"item-1"}, string(longReason))
	if len(errs) != 1 || errs[0].Field != "reason" {
		t.Errorf("expected reason error, got %v", errs)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/go/ecommerce-service && go test ./internal/validate/ -v`
Expected: FAIL — package not found

- [ ] **Step 3: Implement all validators**

Create `go/ecommerce-service/internal/validate/validate.go`:

```go
package validate

import (
	"github.com/google/uuid"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

var validSorts = map[string]bool{
	"created_at_desc": true,
	"price_asc":       true,
	"price_desc":      true,
	"name_asc":        true,
}

const (
	minQuantity  = 1
	maxQuantity  = 99
	maxReasonLen = 500
)

// AddToCart validates the add-to-cart request fields.
func AddToCart(productID string, quantity int) []apperror.FieldError {
	var errs []apperror.FieldError

	if productID == "" {
		errs = append(errs, apperror.FieldError{Field: "productId", Message: "required"})
	} else if _, err := uuid.Parse(productID); err != nil {
		errs = append(errs, apperror.FieldError{Field: "productId", Message: "must be a valid UUID"})
	}

	if quantity < minQuantity || quantity > maxQuantity {
		errs = append(errs, apperror.FieldError{
			Field:   "quantity",
			Message: "must be between 1 and 99",
		})
	}

	return errs
}

// UpdateCart validates the update-cart request fields.
func UpdateCart(quantity int) []apperror.FieldError {
	var errs []apperror.FieldError

	if quantity < minQuantity || quantity > maxQuantity {
		errs = append(errs, apperror.FieldError{
			Field:   "quantity",
			Message: "must be between 1 and 99",
		})
	}

	return errs
}

// ProductListParams validates product listing query parameters.
func ProductListParams(sort string, page, limit int) []apperror.FieldError {
	var errs []apperror.FieldError

	if !validSorts[sort] {
		errs = append(errs, apperror.FieldError{
			Field:   "sort",
			Message: "must be one of: created_at_desc, price_asc, price_desc, name_asc",
		})
	}

	if page < 1 {
		errs = append(errs, apperror.FieldError{Field: "page", Message: "must be at least 1"})
	}

	if limit < 1 || limit > 100 {
		errs = append(errs, apperror.FieldError{Field: "limit", Message: "must be between 1 and 100"})
	}

	return errs
}

// InitiateReturn validates the return initiation request fields.
func InitiateReturn(itemIDs []string, reason string) []apperror.FieldError {
	var errs []apperror.FieldError

	if len(itemIDs) == 0 {
		errs = append(errs, apperror.FieldError{Field: "itemIds", Message: "must contain at least one item"})
	}

	if reason == "" {
		errs = append(errs, apperror.FieldError{Field: "reason", Message: "required"})
	} else if len(reason) > maxReasonLen {
		errs = append(errs, apperror.FieldError{Field: "reason", Message: "must be 500 characters or less"})
	}

	return errs
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/go/ecommerce-service && go test ./internal/validate/ -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer
git add go/ecommerce-service/internal/validate/
git commit -m "feat(ecommerce): add structured input validators with field-level errors"
```

---

## Task 3: Wire Validators Into Handlers

**Files:**
- Modify: `go/ecommerce-service/internal/handler/cart.go:54-65` (AddItem), `83-100` (UpdateQuantity)
- Modify: `go/ecommerce-service/internal/handler/product.go:30-36` (List)
- Modify: `go/ecommerce-service/internal/handler/return.go:37-42` (Initiate)

- [ ] **Step 1: Update cart handler AddItem to use validator**

In `go/ecommerce-service/internal/handler/cart.go`, add import for validate package:

```go
import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/metrics"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/validate"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)
```

Replace the body of `AddItem` (lines 55-81) with:

```go
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

	productID, _ := uuid.Parse(req.ProductID) // already validated

	item, err := h.svc.AddItem(c.Request.Context(), userID, productID, req.Quantity)
	if err != nil {
		_ = c.Error(err)
		return
	}

	metrics.CartItemsAdded.Inc()
	c.JSON(http.StatusCreated, item)
}
```

- [ ] **Step 2: Update cart handler UpdateQuantity to use validator**

Replace the body of `UpdateQuantity` (lines 83-108) with:

```go
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
```

- [ ] **Step 3: Update product handler List to use validator**

In `go/ecommerce-service/internal/handler/product.go`, add import for validate:

```go
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
```

Replace the `List` method body (lines 31-64) with:

```go
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
```

- [ ] **Step 4: Update return handler to use validator**

In `go/ecommerce-service/internal/handler/return.go`, add import for validate:

```go
import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/validate"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)
```

Replace the `Initiate` method body (lines 27-49) with:

```go
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
```

- [ ] **Step 5: Run all existing unit tests**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/go/ecommerce-service && go test ./... -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer
git add go/ecommerce-service/internal/handler/cart.go go/ecommerce-service/internal/handler/product.go go/ecommerce-service/internal/handler/return.go
git commit -m "feat(ecommerce): wire structured validators into handlers"
```

---

## Task 4: Cursor Encoding/Decoding

**Files:**
- Create: `go/ecommerce-service/internal/pagination/cursor.go`
- Create: `go/ecommerce-service/internal/pagination/cursor_test.go`

- [ ] **Step 1: Write failing tests for cursor encode/decode**

Create `go/ecommerce-service/internal/pagination/cursor_test.go`:

```go
package pagination_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/pagination"
)

func TestEncodeDecode_RoundTrip(t *testing.T) {
	id := uuid.New()
	sortVal := "2026-04-16T10:30:00Z"

	cursor := pagination.EncodeCursor(sortVal, id)
	if cursor == "" {
		t.Fatal("expected non-empty cursor")
	}

	gotVal, gotID, err := pagination.DecodeCursor(cursor)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotVal != sortVal {
		t.Errorf("expected sort value %q, got %q", sortVal, gotVal)
	}
	if gotID != id {
		t.Errorf("expected id %s, got %s", id, gotID)
	}
}

func TestEncodeDecode_PriceValue(t *testing.T) {
	id := uuid.New()
	sortVal := "1999" // price in cents

	cursor := pagination.EncodeCursor(sortVal, id)
	gotVal, gotID, err := pagination.DecodeCursor(cursor)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotVal != sortVal {
		t.Errorf("expected %q, got %q", sortVal, gotVal)
	}
	if gotID != id {
		t.Errorf("expected %s, got %s", id, gotID)
	}
}

func TestDecodeCursor_InvalidBase64(t *testing.T) {
	_, _, err := pagination.DecodeCursor("not-valid-base64!!!")
	if err == nil {
		t.Error("expected error for invalid base64")
	}
}

func TestDecodeCursor_InvalidJSON(t *testing.T) {
	// Valid base64 but not valid JSON
	_, _, err := pagination.DecodeCursor("bm90LWpzb24=") // "not-json"
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestDecodeCursor_EmptyString(t *testing.T) {
	_, _, err := pagination.DecodeCursor("")
	if err == nil {
		t.Error("expected error for empty cursor")
	}
}

func TestDecodeCursor_InvalidUUID(t *testing.T) {
	// base64 of {"v":"foo","id":"not-a-uuid"}
	_, _, err := pagination.DecodeCursor("eyJ2IjoiZm9vIiwiaWQiOiJub3QtYS11dWlkIn0=")
	if err == nil {
		t.Error("expected error for invalid UUID in cursor")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/go/ecommerce-service && go test ./internal/pagination/ -v`
Expected: FAIL — package not found

- [ ] **Step 3: Implement cursor encode/decode**

Create `go/ecommerce-service/internal/pagination/cursor.go`:

```go
package pagination

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

type cursorPayload struct {
	Value string `json:"v"`
	ID    string `json:"id"`
}

// EncodeCursor encodes a sort value and ID into an opaque cursor string.
func EncodeCursor(sortValue string, id uuid.UUID) string {
	payload := cursorPayload{Value: sortValue, ID: id.String()}
	data, _ := json.Marshal(payload) // struct marshalling cannot fail
	return base64.URLEncoding.EncodeToString(data)
}

// DecodeCursor decodes an opaque cursor string into its sort value and ID.
func DecodeCursor(cursor string) (string, uuid.UUID, error) {
	if cursor == "" {
		return "", uuid.Nil, fmt.Errorf("cursor is empty")
	}

	data, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return "", uuid.Nil, fmt.Errorf("invalid cursor encoding: %w", err)
	}

	var payload cursorPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", uuid.Nil, fmt.Errorf("invalid cursor format: %w", err)
	}

	id, err := uuid.Parse(payload.ID)
	if err != nil {
		return "", uuid.Nil, fmt.Errorf("invalid cursor ID: %w", err)
	}

	return payload.Value, id, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/go/ecommerce-service && go test ./internal/pagination/ -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer
git add go/ecommerce-service/internal/pagination/
git commit -m "feat(ecommerce): add cursor-based pagination encode/decode"
```

---

## Task 5: Cursor Pagination for Products

**Files:**
- Modify: `go/ecommerce-service/internal/model/product.go` — add cursor fields
- Modify: `go/ecommerce-service/internal/repository/product.go:33-111` — add cursor query branch
- Modify: `go/ecommerce-service/internal/service/product.go:32-61` — pass cursor through, update cache key
- Modify: `go/ecommerce-service/internal/handler/product.go:31-64` — parse cursor param
- Create: `go/ecommerce-service/migrations/005_add_pagination_indexes.up.sql`
- Create: `go/ecommerce-service/migrations/005_add_pagination_indexes.down.sql`

- [ ] **Step 1: Update product model with cursor fields**

In `go/ecommerce-service/internal/model/product.go`, replace the entire file:

```go
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
	Cursor   string // opaque cursor for cursor-based pagination
}

type ProductListResponse struct {
	Products   []Product `json:"products"`
	Total      int       `json:"total,omitempty"`  // only set in offset mode
	Page       int       `json:"page,omitempty"`   // only set in offset mode
	Limit      int       `json:"limit"`
	NextCursor string    `json:"nextCursor,omitempty"`
	HasMore    bool      `json:"hasMore"`
}
```

- [ ] **Step 2: Add cursor-mode query to product repository**

In `go/ecommerce-service/internal/repository/product.go`, add import for `pagination` and `time`:

```go
import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/pagination"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
	gobreaker "github.com/sony/gobreaker/v2"
)
```

Replace the `List` method (lines 33-111) with:

```go
func (r *ProductRepository) List(ctx context.Context, params model.ProductListParams) ([]model.Product, int, error) {
	if params.Cursor != "" {
		return r.listByCursor(ctx, params)
	}
	return r.listByOffset(ctx, params)
}

func (r *ProductRepository) listByCursor(ctx context.Context, params model.ProductListParams) ([]model.Product, int, error) {
	type result struct {
		products []model.Product
	}
	res, err := resilience.Call(ctx, r.breaker, r.retryCfg, func(ctx context.Context) (result, error) {
		cursorVal, cursorID, err := pagination.DecodeCursor(params.Cursor)
		if err != nil {
			return result{}, fmt.Errorf("invalid cursor: %w", err)
		}

		limit := params.Limit
		if limit <= 0 {
			limit = 20
		}

		// Determine sort column and direction
		sortCol, descending := "created_at", true
		switch params.Sort {
		case "price_asc":
			sortCol, descending = "price", false
		case "price_desc":
			sortCol, descending = "price", true
		case "name_asc":
			sortCol, descending = "name", false
		}

		var args []any
		argIdx := 1

		// Build WHERE clause for category/query filters
		var whereParts []string
		if params.Category != "" {
			whereParts = append(whereParts, fmt.Sprintf("category = $%d", argIdx))
			args = append(args, params.Category)
			argIdx++
		}
		if params.Query != "" {
			whereParts = append(whereParts, fmt.Sprintf("name ILIKE '%%' || $%d || '%%'", argIdx))
			args = append(args, params.Query)
			argIdx++
		}

		// Cursor condition: use < for DESC, > for ASC
		op := ">"
		if descending {
			op = "<"
		}

		// Parse cursor value based on sort column
		var cursorArg any
		switch sortCol {
		case "created_at":
			t, err := time.Parse(time.RFC3339Nano, cursorVal)
			if err != nil {
				return result{}, fmt.Errorf("invalid cursor timestamp: %w", err)
			}
			cursorArg = t
		case "price":
			// price stored as int
			var price int
			if _, err := fmt.Sscanf(cursorVal, "%d", &price); err != nil {
				return result{}, fmt.Errorf("invalid cursor price: %w", err)
			}
			cursorArg = price
		case "name":
			cursorArg = cursorVal
		}

		whereParts = append(whereParts, fmt.Sprintf("(%s, id) %s ($%d, $%d)", sortCol, op, argIdx, argIdx+1))
		args = append(args, cursorArg, cursorID)
		argIdx += 2

		whereClause := " WHERE " + strings.Join(whereParts, " AND ")

		dir := "ASC"
		if descending {
			dir = "DESC"
		}
		orderClause := fmt.Sprintf(" ORDER BY %s %s, id %s", sortCol, dir, dir)

		query := fmt.Sprintf(
			"SELECT id, name, description, price, category, image_url, stock, created_at, updated_at FROM products%s%s LIMIT $%d",
			whereClause, orderClause, argIdx,
		)
		args = append(args, limit+1) // fetch one extra to determine hasMore

		rows, err := r.pool.Query(ctx, query, args...)
		if err != nil {
			return result{}, fmt.Errorf("list products (cursor): %w", err)
		}
		defer rows.Close()

		var products []model.Product
		for rows.Next() {
			var p model.Product
			if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Price, &p.Category, &p.ImageURL, &p.Stock, &p.CreatedAt, &p.UpdatedAt); err != nil {
				return result{}, fmt.Errorf("scan product: %w", err)
			}
			products = append(products, p)
		}

		return result{products: products}, nil
	})
	if err != nil {
		return nil, 0, err
	}
	return res.products, 0, nil // total=0 in cursor mode
}

func (r *ProductRepository) listByOffset(ctx context.Context, params model.ProductListParams) ([]model.Product, int, error) {
	type result struct {
		products []model.Product
		total    int
	}
	res, err := resilience.Call(ctx, r.breaker, r.retryCfg, func(ctx context.Context) (result, error) {
		var args []any
		argIdx := 1

		var whereParts []string
		if params.Category != "" {
			whereParts = append(whereParts, fmt.Sprintf("category = $%d", argIdx))
			args = append(args, params.Category)
			argIdx++
		}
		if params.Query != "" {
			whereParts = append(whereParts, fmt.Sprintf("name ILIKE '%%' || $%d || '%%'", argIdx))
			args = append(args, params.Query)
			argIdx++
		}
		whereClause := ""
		if len(whereParts) > 0 {
			whereClause = " WHERE " + strings.Join(whereParts, " AND ")
		}

		countQuery := "SELECT COUNT(*) FROM products" + whereClause
		var total int
		if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
			return result{}, fmt.Errorf("count products: %w", err)
		}

		orderClause := " ORDER BY created_at DESC"
		switch params.Sort {
		case "price_asc":
			orderClause = " ORDER BY price ASC"
		case "price_desc":
			orderClause = " ORDER BY price DESC"
		case "name_asc":
			orderClause = " ORDER BY name ASC"
		}

		limit := params.Limit
		if limit <= 0 {
			limit = 20
		}
		page := params.Page
		if page <= 0 {
			page = 1
		}
		offset := (page - 1) * limit

		query := fmt.Sprintf(
			"SELECT id, name, description, price, category, image_url, stock, created_at, updated_at FROM products%s%s LIMIT $%d OFFSET $%d",
			whereClause, orderClause, argIdx, argIdx+1,
		)
		args = append(args, limit, offset)

		rows, err := r.pool.Query(ctx, query, args...)
		if err != nil {
			return result{}, fmt.Errorf("list products: %w", err)
		}
		defer rows.Close()

		var products []model.Product
		for rows.Next() {
			var p model.Product
			if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Price, &p.Category, &p.ImageURL, &p.Stock, &p.CreatedAt, &p.UpdatedAt); err != nil {
				return result{}, fmt.Errorf("scan product: %w", err)
			}
			products = append(products, p)
		}

		return result{products: products, total: total}, nil
	})
	if err != nil {
		return nil, 0, err
	}
	return res.products, res.total, nil
}
```

- [ ] **Step 3: Update product service to handle cursor mode**

In `go/ecommerce-service/internal/service/product.go`, replace the `List` method (lines 32-61):

```go
func (s *ProductService) List(ctx context.Context, params model.ProductListParams) ([]model.Product, int, error) {
	// Skip cache for cursor-based pagination (cursor values are unique per request)
	if params.Cursor != "" {
		return s.repo.List(ctx, params)
	}

	cacheKey := fmt.Sprintf("ecom:products:list:%s:%s:%d:%d", params.Category, params.Sort, params.Page, params.Limit)

	if s.redis != nil {
		cached, err := s.redis.Get(ctx, cacheKey).Result()
		if err == nil {
			var resp model.ProductListResponse
			if json.Unmarshal([]byte(cached), &resp) == nil {
				metrics.CacheOps.WithLabelValues("get", "hit").Inc()
				return resp.Products, resp.Total, nil
			}
		}
		metrics.CacheOps.WithLabelValues("get", "miss").Inc()
	}

	products, total, err := s.repo.List(ctx, params)
	if err != nil {
		return nil, 0, err
	}

	if s.redis != nil {
		resp := model.ProductListResponse{Products: products, Total: total, Page: params.Page, Limit: params.Limit}
		if data, err := json.Marshal(resp); err == nil {
			s.redis.Set(ctx, cacheKey, data, 5*time.Minute)
			metrics.CacheOps.WithLabelValues("set", "success").Inc()
		}
	}

	return products, total, nil
}
```

- [ ] **Step 4: Update product handler to parse cursor and build response**

Replace the `List` method in `go/ecommerce-service/internal/handler/product.go` with:

```go
func (h *ProductHandler) List(c *gin.Context) {
	cursor := c.Query("cursor")
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
		Cursor:   cursor,
	}

	products, total, err := h.svc.List(c.Request.Context(), params)
	if err != nil {
		_ = c.Error(err)
		return
	}

	resp := model.ProductListResponse{
		Limit: limit,
	}

	if cursor != "" {
		// Cursor mode: check if we got limit+1 results (hasMore)
		if len(products) > limit {
			resp.HasMore = true
			products = products[:limit]
			last := products[len(products)-1]
			resp.NextCursor = buildProductCursor(last, sort)
		}
		resp.Products = products
	} else {
		// Offset mode: backwards compatible
		resp.Products = products
		resp.Total = total
		resp.Page = page
		resp.HasMore = page*limit < total
		if resp.HasMore && len(products) > 0 {
			last := products[len(products)-1]
			resp.NextCursor = buildProductCursor(last, sort)
		}
	}

	c.JSON(http.StatusOK, resp)
}
```

Add a helper function at the bottom of the same file, and add import for `pagination`, `fmt`:

```go
import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/metrics"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/pagination"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/validate"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)
```

```go
func buildProductCursor(p model.Product, sort string) string {
	switch sort {
	case "price_asc", "price_desc":
		return pagination.EncodeCursor(fmt.Sprintf("%d", p.Price), p.ID)
	case "name_asc":
		return pagination.EncodeCursor(p.Name, p.ID)
	default: // created_at_desc
		return pagination.EncodeCursor(p.CreatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"), p.ID)
	}
}
```

- [ ] **Step 5: Create pagination index migration**

Create `go/ecommerce-service/migrations/005_add_pagination_indexes.up.sql`:

```sql
CREATE INDEX IF NOT EXISTS idx_products_price_id ON products (price, id);
CREATE INDEX IF NOT EXISTS idx_products_name_id ON products (name, id);
CREATE INDEX IF NOT EXISTS idx_products_created_at_id ON products (created_at DESC, id DESC);
```

Create `go/ecommerce-service/migrations/005_add_pagination_indexes.down.sql`:

```sql
DROP INDEX IF EXISTS idx_products_price_id;
DROP INDEX IF EXISTS idx_products_name_id;
DROP INDEX IF EXISTS idx_products_created_at_id;
```

- [ ] **Step 6: Run all existing unit tests**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/go/ecommerce-service && go test ./... -v`
Expected: All PASS

- [ ] **Step 7: Commit**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer
git add go/ecommerce-service/internal/model/product.go go/ecommerce-service/internal/repository/product.go go/ecommerce-service/internal/service/product.go go/ecommerce-service/internal/handler/product.go go/ecommerce-service/migrations/005_add_pagination_indexes.up.sql go/ecommerce-service/migrations/005_add_pagination_indexes.down.sql
git commit -m "feat(ecommerce): add cursor-based pagination for products"
```

---

## Task 6: Cursor Pagination for Orders

**Files:**
- Modify: `go/ecommerce-service/internal/model/order.go` — add pagination types
- Modify: `go/ecommerce-service/internal/repository/order.go:108-131` — add cursor + limit
- Modify: `go/ecommerce-service/internal/service/order.go:86-88` — accept params
- Modify: `go/ecommerce-service/internal/handler/order.go:44-58` — parse cursor/limit

- [ ] **Step 1: Update order model with pagination types**

In `go/ecommerce-service/internal/model/order.go`, add at the bottom:

```go
type OrderListParams struct {
	Cursor string
	Limit  int
}

type OrderListResponse struct {
	Orders     []Order `json:"orders"`
	NextCursor string  `json:"nextCursor,omitempty"`
	HasMore    bool    `json:"hasMore"`
}
```

- [ ] **Step 2: Update order repository ListByUser with cursor support**

Replace `ListByUser` in `go/ecommerce-service/internal/repository/order.go` (lines 108-131):

```go
func (r *OrderRepository) ListByUser(ctx context.Context, userID uuid.UUID, params model.OrderListParams) ([]model.Order, error) {
	return resilience.Call(ctx, r.breaker, r.retryCfg, func(ctx context.Context) ([]model.Order, error) {
		limit := params.Limit
		if limit <= 0 {
			limit = 20
		}

		var args []any
		argIdx := 1
		whereParts := []string{fmt.Sprintf("user_id = $%d", argIdx)}
		args = append(args, userID)
		argIdx++

		if params.Cursor != "" {
			cursorVal, cursorID, err := pagination.DecodeCursor(params.Cursor)
			if err != nil {
				return nil, fmt.Errorf("invalid cursor: %w", err)
			}
			cursorTime, err := time.Parse(time.RFC3339Nano, cursorVal)
			if err != nil {
				return nil, fmt.Errorf("invalid cursor timestamp: %w", err)
			}
			whereParts = append(whereParts, fmt.Sprintf("(created_at, id) < ($%d, $%d)", argIdx, argIdx+1))
			args = append(args, cursorTime, cursorID)
			argIdx += 2
		}

		query := fmt.Sprintf(
			"SELECT id, user_id, status, total, created_at, updated_at FROM orders WHERE %s ORDER BY created_at DESC, id DESC LIMIT $%d",
			strings.Join(whereParts, " AND "), argIdx,
		)
		args = append(args, limit+1) // fetch one extra for hasMore

		rows, err := r.pool.Query(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("list orders: %w", err)
		}
		defer rows.Close()

		var orders []model.Order
		for rows.Next() {
			var o model.Order
			if err := rows.Scan(&o.ID, &o.UserID, &o.Status, &o.Total, &o.CreatedAt, &o.UpdatedAt); err != nil {
				return nil, fmt.Errorf("scan order: %w", err)
			}
			orders = append(orders, o)
		}
		return orders, nil
	})
}
```

Add imports for `pagination` and `time` to order repository:

```go
import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/pagination"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
	gobreaker "github.com/sony/gobreaker/v2"
)
```

- [ ] **Step 3: Update OrderRepo interface and OrderService**

In `go/ecommerce-service/internal/service/order.go`, update the `OrderRepo` interface:

```go
type OrderRepo interface {
	Create(ctx context.Context, userID uuid.UUID, total int, items []model.OrderItem) (*model.Order, error)
	FindByID(ctx context.Context, id uuid.UUID) (*model.Order, error)
	ListByUser(ctx context.Context, userID uuid.UUID, params model.OrderListParams) ([]model.Order, error)
	UpdateStatus(ctx context.Context, orderID uuid.UUID, status model.OrderStatus) error
}
```

Replace `ListOrders` method:

```go
func (s *OrderService) ListOrders(ctx context.Context, userID uuid.UUID, params model.OrderListParams) ([]model.Order, error) {
	return s.orderRepo.ListByUser(ctx, userID, params)
}
```

- [ ] **Step 4: Update OrderServiceInterface and handler**

In `go/ecommerce-service/internal/handler/order.go`, update the interface:

```go
type OrderServiceInterface interface {
	Checkout(ctx context.Context, userID uuid.UUID) (*model.Order, error)
	GetOrder(ctx context.Context, orderID uuid.UUID) (*model.Order, error)
	ListOrders(ctx context.Context, userID uuid.UUID, params model.OrderListParams) ([]model.Order, error)
}
```

Add imports for `pagination`, `strconv`, `fmt`:

```go
import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/pagination"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)
```

Replace the `List` method (lines 44-58):

```go
func (h *OrderHandler) List(c *gin.Context) {
	userID, err := uuid.Parse(c.GetString("userId"))
	if err != nil {
		_ = c.Error(apperror.BadRequest("INVALID_ID", "invalid user ID"))
		return
	}

	cursor := c.Query("cursor")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit < 1 || limit > 50 {
		limit = 20
	}

	params := model.OrderListParams{
		Cursor: cursor,
		Limit:  limit,
	}

	orders, err := h.svc.ListOrders(c.Request.Context(), userID, params)
	if err != nil {
		_ = c.Error(err)
		return
	}

	resp := model.OrderListResponse{HasMore: false}
	if len(orders) > limit {
		resp.HasMore = true
		orders = orders[:limit]
		last := orders[len(orders)-1]
		resp.NextCursor = pagination.EncodeCursor(
			last.CreatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"),
			last.ID,
		)
	}
	resp.Orders = orders

	c.JSON(http.StatusOK, resp)
}
```

- [ ] **Step 5: Update the worker's order repo usage (if it calls ListByUser)**

The worker (`order_processor.go`) uses `OrderRepo` but only calls `UpdateStatus`. Check if it also references `ListByUser` — it doesn't, but the worker test's mock must match the interface.

Check and update any mocks in test files that implement `OrderRepo` to include the new `ListByUser` signature.

In `go/ecommerce-service/internal/service/order_test.go`, update the mock's `ListByUser`:

```go
func (m *mockOrderRepo) ListByUser(ctx context.Context, userID uuid.UUID, params model.OrderListParams) ([]model.Order, error) {
	// existing mock implementation, add params argument
	return nil, nil
}
```

Similarly update any mock in `go/ecommerce-service/internal/worker/order_processor_test.go` if it implements `OrderRepo`.

- [ ] **Step 6: Run all unit tests**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/go/ecommerce-service && go test ./... -v`
Expected: All PASS

- [ ] **Step 7: Commit**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer
git add go/ecommerce-service/internal/model/order.go go/ecommerce-service/internal/repository/order.go go/ecommerce-service/internal/service/order.go go/ecommerce-service/internal/handler/order.go
git add -u go/ecommerce-service/internal/service/ go/ecommerce-service/internal/worker/
git commit -m "feat(ecommerce): add cursor-based pagination for orders"
```

---

## Task 7: Idempotency Middleware

**Files:**
- Create: `go/ecommerce-service/internal/middleware/idempotency.go`
- Create: `go/ecommerce-service/internal/middleware/idempotency_test.go`
- Modify: `go/ecommerce-service/internal/metrics/metrics.go` — add idempotency metrics

- [ ] **Step 1: Add idempotency metrics**

In `go/ecommerce-service/internal/metrics/metrics.go`, add at the end of the `var` block:

```go
	IdempotencyOps = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ecommerce_idempotency_operations_total",
		Help: "Idempotency key operations.",
	}, []string{"result"}) // hit, miss, conflict, error
```

- [ ] **Step 2: Write failing tests for idempotency middleware**

Create `go/ecommerce-service/internal/middleware/idempotency_test.go`:

```go
package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/middleware"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

func setupIdempotencyRouter(mw gin.HandlerFunc, required bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(apperror.ErrorHandler())
	r.POST("/test", func(c *gin.Context) {
		c.Set("userId", uuid.New().String())
		c.Next()
	}, mw, func(c *gin.Context) {
		c.JSON(http.StatusCreated, gin.H{"id": "order-123"})
	})
	return r
}

func TestIdempotency_NilRedis_PassesThrough(t *testing.T) {
	mw := middleware.Idempotency(nil, true)
	router := setupIdempotencyRouter(mw, true)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/test", strings.NewReader(`{}`))
	req.Header.Set("Idempotency-Key", uuid.New().String())
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
}

func TestIdempotency_RequiredButMissing_Returns400(t *testing.T) {
	mw := middleware.Idempotency(nil, true)
	router := setupIdempotencyRouter(mw, true)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/test", strings.NewReader(`{}`))
	// No Idempotency-Key header
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestIdempotency_OptionalAndMissing_PassesThrough(t *testing.T) {
	mw := middleware.Idempotency(nil, false)
	router := setupIdempotencyRouter(mw, false)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/test", strings.NewReader(`{}`))
	// No Idempotency-Key header
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
}

func TestIdempotency_InvalidUUID_Returns400(t *testing.T) {
	mw := middleware.Idempotency(nil, true)
	router := setupIdempotencyRouter(mw, true)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/test", strings.NewReader(`{}`))
	req.Header.Set("Idempotency-Key", "not-a-uuid")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/go/ecommerce-service && go test ./internal/middleware/ -run TestIdempotency -v`
Expected: FAIL — `middleware.Idempotency` undefined

- [ ] **Step 4: Implement idempotency middleware**

Create `go/ecommerce-service/internal/middleware/idempotency.go`:

```go
package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/metrics"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

const (
	idempotencyHeader     = "Idempotency-Key"
	idempotencyPrefix     = "idempotency:"
	processingTTL         = 30 * time.Second
	completedTTL          = 24 * time.Hour
)

type idempotencyEntry struct {
	Status     string `json:"status"`
	StatusCode int    `json:"status_code,omitempty"`
	Body       string `json:"body,omitempty"`
}

// Idempotency returns middleware that prevents duplicate requests using Redis.
// If required is true, requests without an Idempotency-Key header are rejected.
// If redisClient is nil, the middleware passes through (fail open).
func Idempotency(redisClient *redis.Client, required bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.GetHeader(idempotencyHeader)
		if key == "" {
			if required {
				_ = c.Error(apperror.BadRequest("MISSING_IDEMPOTENCY_KEY", "Idempotency-Key header is required"))
				c.Abort()
				return
			}
			c.Next()
			return
		}

		// Validate key is a UUID
		if _, err := uuid.Parse(key); err != nil {
			_ = c.Error(apperror.BadRequest("INVALID_IDEMPOTENCY_KEY", "Idempotency-Key must be a valid UUID"))
			c.Abort()
			return
		}

		// No Redis — fail open
		if redisClient == nil {
			c.Next()
			return
		}

		userID := c.GetString("userId")
		redisKey := idempotencyPrefix + userID + ":" + key
		ctx := c.Request.Context()

		// Check if key already exists
		existing, err := redisClient.Get(ctx, redisKey).Result()
		if err == nil {
			var entry idempotencyEntry
			if json.Unmarshal([]byte(existing), &entry) == nil {
				if entry.Status == "processing" {
					metrics.IdempotencyOps.WithLabelValues("conflict").Inc()
					_ = c.Error(apperror.Conflict("DUPLICATE_REQUEST", "request is already being processed"))
					c.Abort()
					return
				}
				// Replay cached response
				metrics.IdempotencyOps.WithLabelValues("hit").Inc()
				c.Data(entry.StatusCode, "application/json", []byte(entry.Body))
				c.Abort()
				return
			}
		} else if err != redis.Nil {
			// Redis error — fail open
			slog.Warn("idempotency redis error", "error", err, "key", redisKey)
			metrics.IdempotencyOps.WithLabelValues("error").Inc()
			c.Next()
			return
		}

		// Set processing marker
		processingEntry, _ := json.Marshal(idempotencyEntry{Status: "processing"})
		set, err := redisClient.SetNX(ctx, redisKey, string(processingEntry), processingTTL).Result()
		if err != nil {
			slog.Warn("idempotency redis setnx error", "error", err)
			metrics.IdempotencyOps.WithLabelValues("error").Inc()
			c.Next()
			return
		}
		if !set {
			// Another request set it between our GET and SETNX
			metrics.IdempotencyOps.WithLabelValues("conflict").Inc()
			_ = c.Error(apperror.Conflict("DUPLICATE_REQUEST", "request is already being processed"))
			c.Abort()
			return
		}

		metrics.IdempotencyOps.WithLabelValues("miss").Inc()

		// Wrap response writer to capture output
		w := &responseCapture{ResponseWriter: c.Writer, body: &bytes.Buffer{}}
		c.Writer = w

		c.Next()

		// Cache the response
		completedEntry, _ := json.Marshal(idempotencyEntry{
			Status:     "done",
			StatusCode: w.Status(),
			Body:       w.body.String(),
		})
		if err := redisClient.Set(ctx, redisKey, string(completedEntry), completedTTL).Err(); err != nil {
			slog.Warn("idempotency redis set error", "error", err)
		}
	}
}

// responseCapture wraps gin.ResponseWriter to capture the response body.
type responseCapture struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *responseCapture) Write(data []byte) (int, error) {
	w.body.Write(data)
	return w.ResponseWriter.Write(data)
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/go/ecommerce-service && go test ./internal/middleware/ -run TestIdempotency -v`
Expected: All PASS

- [ ] **Step 6: Run all unit tests**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/go/ecommerce-service && go test ./... -v`
Expected: All PASS

- [ ] **Step 7: Commit**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer
git add go/ecommerce-service/internal/middleware/idempotency.go go/ecommerce-service/internal/middleware/idempotency_test.go go/ecommerce-service/internal/metrics/metrics.go
git commit -m "feat(ecommerce): add Redis-backed idempotency key middleware"
```

---

## Task 8: Wire Idempotency Middleware Into Routes

**Files:**
- Modify: `go/ecommerce-service/cmd/server/main.go:210-224`

- [ ] **Step 1: Apply idempotency middleware to checkout and cart-add routes**

In `go/ecommerce-service/cmd/server/main.go`, replace the authenticated routes block (lines 211-224):

```go
	// Authenticated routes
	auth := router.Group("/")
	auth.Use(middleware.Auth(jwtSecret))
	auth.Use(ecomLimiter.Middleware())
	{
		auth.GET("/cart", cartHandler.GetCart)
		auth.POST("/cart", middleware.Idempotency(redisClient, false), cartHandler.AddItem)
		auth.PUT("/cart/:itemId", cartHandler.UpdateQuantity)
		auth.DELETE("/cart/:itemId", cartHandler.RemoveItem)

		auth.POST("/orders", middleware.Idempotency(redisClient, true), orderHandler.Checkout)
		auth.GET("/orders", orderHandler.List)
		auth.GET("/orders/:id", orderHandler.GetByID)
		auth.POST("/orders/:id/returns", returnHandler.Initiate)
	}
```

- [ ] **Step 2: Run all unit tests**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/go/ecommerce-service && go test ./... -v`
Expected: All PASS

- [ ] **Step 3: Commit**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer
git add go/ecommerce-service/cmd/server/main.go
git commit -m "feat(ecommerce): wire idempotency middleware to checkout and cart-add routes"
```

---

## Task 9: Integration Test Infrastructure

**Files:**
- Create: `go/ecommerce-service/internal/integration/testutil/containers.go`
- Create: `go/ecommerce-service/internal/integration/testutil/db.go`
- Create: `go/ecommerce-service/internal/integration/testutil/helpers.go`

- [ ] **Step 1: Add testcontainers-go dependency**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/go/ecommerce-service && go get github.com/testcontainers/testcontainers-go && go get github.com/testcontainers/testcontainers-go/modules/postgres && go get github.com/testcontainers/testcontainers-go/modules/redis && go get github.com/testcontainers/testcontainers-go/modules/rabbitmq && go mod tidy`

- [ ] **Step 2: Create container setup utilities**

Create `go/ecommerce-service/internal/integration/testutil/containers.go`:

```go
//go:build integration

package testutil

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/modules/rabbitmq"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Infra holds shared test infrastructure.
type Infra struct {
	Pool        *pgxpool.Pool
	RedisClient *redis.Client
	RabbitConn  *amqp.Connection
	RabbitCh    *amqp.Channel

	pgContainer     testcontainers.Container
	redisContainer  testcontainers.Container
	rabbitContainer testcontainers.Container
}

// SetupInfra starts Postgres, Redis, and RabbitMQ containers.
func SetupInfra(ctx context.Context, t *testing.T) *Infra {
	t.Helper()
	infra := &Infra{}

	// Postgres
	pgContainer, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("ecommercedb_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	infra.pgContainer = pgContainer

	pgConnStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("postgres connection string: %v", err)
	}

	pool, err := pgxpool.New(ctx, pgConnStr)
	if err != nil {
		t.Fatalf("connect to postgres: %v", err)
	}
	infra.Pool = pool

	// Redis
	redisContainer, err := tcredis.Run(ctx, "redis:7-alpine")
	if err != nil {
		t.Fatalf("start redis: %v", err)
	}
	infra.redisContainer = redisContainer

	redisEndpoint, err := redisContainer.Endpoint(ctx, "")
	if err != nil {
		t.Fatalf("redis endpoint: %v", err)
	}

	infra.RedisClient = redis.NewClient(&redis.Options{Addr: redisEndpoint})

	// RabbitMQ
	rabbitContainer, err := rabbitmq.Run(ctx, "rabbitmq:3-alpine")
	if err != nil {
		t.Fatalf("start rabbitmq: %v", err)
	}
	infra.rabbitContainer = rabbitContainer

	rabbitEndpoint, err := rabbitContainer.AmqpURL(ctx)
	if err != nil {
		t.Fatalf("rabbitmq endpoint: %v", err)
	}

	conn, err := amqp.Dial(rabbitEndpoint)
	if err != nil {
		t.Fatalf("connect to rabbitmq: %v", err)
	}
	infra.RabbitConn = conn

	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("rabbitmq channel: %v", err)
	}
	infra.RabbitCh = ch

	if err := ch.ExchangeDeclare("ecommerce", "topic", true, false, false, false, nil); err != nil {
		t.Fatalf("declare exchange: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()
		infra.RedisClient.Close()
		ch.Close()
		conn.Close()
		pgContainer.Terminate(ctx)
		redisContainer.Terminate(ctx)
		rabbitContainer.Terminate(ctx)
	})

	return infra
}

// DatabaseURL returns the connection string for test Postgres.
func (i *Infra) DatabaseURL(ctx context.Context) string {
	connStr, _ := i.pgContainer.(interface {
		ConnectionString(context.Context, ...string) (string, error)
	}).ConnectionString(ctx, "sslmode=disable")
	return connStr
}

func (i *Infra) RedisURL(ctx context.Context) string {
	endpoint, _ := i.redisContainer.Endpoint(ctx, "")
	return fmt.Sprintf("redis://%s", endpoint)
}
```

- [ ] **Step 3: Create database migration runner and seeder**

Create `go/ecommerce-service/internal/integration/testutil/db.go`:

```go
//go:build integration

package testutil

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunMigrations applies all .up.sql migrations in order.
func RunMigrations(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	migrationsDir := findMigrationsDir()
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}

	var upFiles []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".up.sql") {
			upFiles = append(upFiles, e.Name())
		}
	}
	sort.Strings(upFiles)

	// Ensure pgcrypto extension exists
	if _, err := pool.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS pgcrypto"); err != nil {
		t.Fatalf("create pgcrypto: %v", err)
	}

	for _, f := range upFiles {
		sql, err := os.ReadFile(filepath.Join(migrationsDir, f))
		if err != nil {
			t.Fatalf("read migration %s: %v", f, err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("run migration %s: %v", f, err)
		}
	}
}

// TruncateAll truncates all application tables for test isolation.
func TruncateAll(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	tables := []string{"returns", "order_items", "orders", "cart_items", "products"}
	for _, table := range tables {
		if _, err := pool.Exec(ctx, "TRUNCATE "+table+" CASCADE"); err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}
}

// SeedProducts inserts n test products and returns their IDs.
func SeedProducts(ctx context.Context, t *testing.T, pool *pgxpool.Pool, n int) []string {
	t.Helper()
	var ids []string
	for i := 0; i < n; i++ {
		var id string
		err := pool.QueryRow(ctx,
			`INSERT INTO products (id, name, description, price, category, image_url, stock, created_at, updated_at)
			 VALUES (gen_random_uuid(), $1, $2, $3, $4, '', 100, NOW() + ($5 || ' seconds')::interval, NOW())
			 RETURNING id::text`,
			"Product "+strings.Repeat("A", i%26+1), // varied names for sort testing
			"Test product",
			(i+1)*100, // varied prices: 100, 200, ...
			[]string{"Electronics", "Clothing", "Home", "Books", "Sports"}[i%5],
			i, // stagger created_at by 1 second each
		).Scan(&id)
		if err != nil {
			t.Fatalf("seed product %d: %v", i, err)
		}
		ids = append(ids, id)
	}
	return ids
}

func findMigrationsDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "..", "..", "migrations")
}
```

- [ ] **Step 4: Create HTTP test helpers**

Create `go/ecommerce-service/internal/integration/testutil/helpers.go`:

```go
//go:build integration

package testutil

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// DoRequest performs an HTTP request against a Gin engine and returns the response.
func DoRequest(t *testing.T, router *gin.Engine, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}

	req, err := http.NewRequest(method, path, bodyReader)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// ParseJSON unmarshals response body into target.
func ParseJSON(t *testing.T, w *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), target); err != nil {
		t.Fatalf("parse response JSON: %v\nbody: %s", err, w.Body.String())
	}
}
```

- [ ] **Step 5: Commit**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer
git add go/ecommerce-service/internal/integration/ go/ecommerce-service/go.mod go/ecommerce-service/go.sum
git commit -m "feat(ecommerce): add testcontainers-based integration test infrastructure"
```

---

## Task 10: Repository Integration Tests

**Files:**
- Create: `go/ecommerce-service/internal/integration/repository_test.go`

- [ ] **Step 1: Write repository integration tests**

Create `go/ecommerce-service/internal/integration/repository_test.go`:

```go
//go:build integration

package integration

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/integration/testutil"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/repository"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
)

var infra *testutil.Infra

func TestMain(m *testing.M) {
	ctx := context.Background()
	// Use a temporary testing.T for setup
	t := &testing.T{}
	infra = testutil.SetupInfra(ctx, t)
	testutil.RunMigrations(ctx, t, infra.Pool)
	os.Exit(m.Run())
}

// Helper — resilience.NewBreaker returns *gobreaker.CircuitBreaker[any]
// Repositories accept this type in their constructors.

func TestProductRepository_ListAndFindByID(t *testing.T) {
	ctx := context.Background()
	testutil.TruncateAll(ctx, t, infra.Pool)
	ids := testutil.SeedProducts(ctx, t, infra.Pool, 5)

	breaker := resilience.NewBreaker(resilience.BreakerConfig{Name: "test-product"})
	repo := repository.NewProductRepository(infra.Pool, breaker)

	// List all
	products, total, err := repo.List(ctx, model.ProductListParams{
		Sort:  "created_at_desc",
		Page:  1,
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("list products: %v", err)
	}
	if total != 5 {
		t.Errorf("expected total 5, got %d", total)
	}
	if len(products) != 5 {
		t.Errorf("expected 5 products, got %d", len(products))
	}

	// FindByID
	id, _ := uuid.Parse(ids[0])
	product, err := repo.FindByID(ctx, id)
	if err != nil {
		t.Fatalf("find product: %v", err)
	}
	if product.ID != id {
		t.Errorf("expected id %s, got %s", id, product.ID)
	}
}

func TestCartRepository_CRUD(t *testing.T) {
	ctx := context.Background()
	testutil.TruncateAll(ctx, t, infra.Pool)
	productIDs := testutil.SeedProducts(ctx, t, infra.Pool, 2)

	breaker := resilience.NewBreaker(resilience.BreakerConfig{Name: "test-cart"})
	repo := repository.NewCartRepository(infra.Pool, breaker)
	userID := uuid.New()
	productID, _ := uuid.Parse(productIDs[0])

	// Add item
	item, err := repo.AddItem(ctx, userID, productID, 3)
	if err != nil {
		t.Fatalf("add item: %v", err)
	}
	if item.Quantity != 3 {
		t.Errorf("expected quantity 3, got %d", item.Quantity)
	}

	// Add same item again (upsert)
	item, err = repo.AddItem(ctx, userID, productID, 2)
	if err != nil {
		t.Fatalf("upsert item: %v", err)
	}
	if item.Quantity != 5 {
		t.Errorf("expected quantity 5 after upsert, got %d", item.Quantity)
	}

	// Get by user
	items, err := repo.GetByUser(ctx, userID)
	if err != nil {
		t.Fatalf("get cart: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].ProductName == "" {
		t.Error("expected product name from JOIN, got empty")
	}

	// Update quantity
	err = repo.UpdateQuantity(ctx, item.ID, userID, 10)
	if err != nil {
		t.Fatalf("update quantity: %v", err)
	}

	// Remove item
	err = repo.RemoveItem(ctx, item.ID, userID)
	if err != nil {
		t.Fatalf("remove item: %v", err)
	}

	items, err = repo.GetByUser(ctx, userID)
	if err != nil {
		t.Fatalf("get cart after remove: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected empty cart, got %d items", len(items))
	}
}

func TestOrderRepository_CreateAndFind(t *testing.T) {
	ctx := context.Background()
	testutil.TruncateAll(ctx, t, infra.Pool)
	productIDs := testutil.SeedProducts(ctx, t, infra.Pool, 1)

	breaker := resilience.NewBreaker(resilience.BreakerConfig{Name: "test-order"})
	repo := repository.NewOrderRepository(infra.Pool, breaker)
	userID := uuid.New()
	productID, _ := uuid.Parse(productIDs[0])

	items := []model.OrderItem{
		{ProductID: productID, Quantity: 2, PriceAtPurchase: 100},
	}

	order, err := repo.Create(ctx, userID, 200, items)
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if order.Status != model.OrderStatusPending {
		t.Errorf("expected status pending, got %s", order.Status)
	}
	if order.Total != 200 {
		t.Errorf("expected total 200, got %d", order.Total)
	}

	// Find by ID
	found, err := repo.FindByID(ctx, order.ID)
	if err != nil {
		t.Fatalf("find order: %v", err)
	}
	if len(found.Items) != 1 {
		t.Errorf("expected 1 item, got %d", len(found.Items))
	}

	// Update status
	err = repo.UpdateStatus(ctx, order.ID, model.OrderStatusCompleted)
	if err != nil {
		t.Fatalf("update status: %v", err)
	}

	found, err = repo.FindByID(ctx, order.ID)
	if err != nil {
		t.Fatalf("find after update: %v", err)
	}
	if found.Status != model.OrderStatusCompleted {
		t.Errorf("expected completed, got %s", found.Status)
	}
}
```

Note: The `TestMain` function here uses a non-standard pattern since `testutil.SetupInfra` requires a `*testing.T`. This will need adjustment — `TestMain` receives `*testing.M`, not `*testing.T`. The proper pattern is to use a package-level setup function called from the first test or use a `sync.Once`. Let me fix this:

Replace the `TestMain` and `infra` with:

```go
//go:build integration

package integration

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/integration/testutil"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/repository"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
)

var (
	infra     *testutil.Infra
	infraOnce sync.Once
)

func setupInfra(t *testing.T) *testutil.Infra {
	t.Helper()
	infraOnce.Do(func() {
		infra = testutil.SetupInfra(context.Background(), t)
		testutil.RunMigrations(context.Background(), t, infra.Pool)
	})
	return infra
}
```

Then each test starts with `infra := setupInfra(t)`.

- [ ] **Step 2: Run integration tests**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/go/ecommerce-service && go test -tags=integration -race ./internal/integration/ -run TestProduct -v -timeout 120s`
Expected: PASS (may take 30-60s for container startup)

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/go/ecommerce-service && go test -tags=integration -race ./internal/integration/ -run TestCart -v -timeout 120s`
Expected: PASS

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/go/ecommerce-service && go test -tags=integration -race ./internal/integration/ -run TestOrder -v -timeout 120s`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer
git add go/ecommerce-service/internal/integration/repository_test.go
git commit -m "test(ecommerce): add repository integration tests with testcontainers"
```

---

## Task 11: Checkout Flow Integration Test

**Files:**
- Create: `go/ecommerce-service/internal/integration/checkout_test.go`

- [ ] **Step 1: Write checkout flow integration test**

Create `go/ecommerce-service/internal/integration/checkout_test.go`:

```go
//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/integration/testutil"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/repository"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/service"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
)

type testPublisher struct {
	ch       *amqp.Channel
	messages []string
}

func (p *testPublisher) PublishOrderCreated(orderID string) error {
	p.messages = append(p.messages, orderID)
	body, _ := json.Marshal(model.OrderMessage{OrderID: orderID})
	return p.ch.Publish("ecommerce", "order.created", false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        body,
	})
}

func TestCheckoutFlow_EndToEnd(t *testing.T) {
	infra := setupInfra(t)
	ctx := context.Background()
	testutil.TruncateAll(ctx, t, infra.Pool)
	productIDs := testutil.SeedProducts(ctx, t, infra.Pool, 3)

	breaker := resilience.NewBreaker(resilience.BreakerConfig{Name: "test-checkout"})
	cartRepo := repository.NewCartRepository(infra.Pool, breaker)
	orderRepo := repository.NewOrderRepository(infra.Pool, breaker)
	publisher := &testPublisher{ch: infra.RabbitCh}
	orderSvc := service.NewOrderService(orderRepo, cartRepo, publisher)

	userID := uuid.New()
	pid1, _ := uuid.Parse(productIDs[0])
	pid2, _ := uuid.Parse(productIDs[1])

	// Add items to cart
	_, err := cartRepo.AddItem(ctx, userID, pid1, 2)
	if err != nil {
		t.Fatalf("add item 1: %v", err)
	}
	_, err = cartRepo.AddItem(ctx, userID, pid2, 1)
	if err != nil {
		t.Fatalf("add item 2: %v", err)
	}

	// Checkout
	order, err := orderSvc.Checkout(ctx, userID)
	if err != nil {
		t.Fatalf("checkout: %v", err)
	}

	// Verify order created
	if order.Status != model.OrderStatusPending {
		t.Errorf("expected pending, got %s", order.Status)
	}
	if len(order.Items) != 2 {
		t.Errorf("expected 2 order items, got %d", len(order.Items))
	}

	// Verify cart is cleared
	cartItems, err := cartRepo.GetByUser(ctx, userID)
	if err != nil {
		t.Fatalf("get cart after checkout: %v", err)
	}
	if len(cartItems) != 0 {
		t.Errorf("expected empty cart after checkout, got %d items", len(cartItems))
	}

	// Verify RabbitMQ message was published
	if len(publisher.messages) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(publisher.messages))
	}
	if publisher.messages[0] != order.ID.String() {
		t.Errorf("expected order ID %s in message, got %s", order.ID, publisher.messages[0])
	}

	// Verify order exists in DB
	found, err := orderRepo.FindByID(ctx, order.ID)
	if err != nil {
		t.Fatalf("find order: %v", err)
	}
	if found.Total != order.Total {
		t.Errorf("expected total %d, got %d", order.Total, found.Total)
	}
}

func TestCheckoutFlow_EmptyCart(t *testing.T) {
	infra := setupInfra(t)
	ctx := context.Background()
	testutil.TruncateAll(ctx, t, infra.Pool)

	breaker := resilience.NewBreaker(resilience.BreakerConfig{Name: "test-checkout-empty"})
	cartRepo := repository.NewCartRepository(infra.Pool, breaker)
	orderRepo := repository.NewOrderRepository(infra.Pool, breaker)
	publisher := &testPublisher{ch: infra.RabbitCh}
	orderSvc := service.NewOrderService(orderRepo, cartRepo, publisher)

	userID := uuid.New()
	_, err := orderSvc.Checkout(ctx, userID)
	if err == nil {
		t.Fatal("expected error for empty cart checkout")
	}

	// Allow a moment for any async operations
	time.Sleep(10 * time.Millisecond)
}
```

- [ ] **Step 2: Run the test**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/go/ecommerce-service && go test -tags=integration -race ./internal/integration/ -run TestCheckout -v -timeout 120s`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer
git add go/ecommerce-service/internal/integration/checkout_test.go
git commit -m "test(ecommerce): add end-to-end checkout flow integration test"
```

---

## Task 12: Pagination Integration Tests

**Files:**
- Create: `go/ecommerce-service/internal/integration/pagination_test.go`

- [ ] **Step 1: Write pagination integration tests**

Create `go/ecommerce-service/internal/integration/pagination_test.go`:

```go
//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/integration/testutil"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/pagination"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/repository"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
)

func TestPagination_CursorThroughAllProducts(t *testing.T) {
	infra := setupInfra(t)
	ctx := context.Background()
	testutil.TruncateAll(ctx, t, infra.Pool)
	testutil.SeedProducts(ctx, t, infra.Pool, 25)

	breaker := resilience.NewBreaker(resilience.BreakerConfig{Name: "test-pagination"})
	repo := repository.NewProductRepository(infra.Pool, breaker)

	pageSize := 5
	var allProducts []model.Product
	cursor := ""

	for i := 0; i < 10; i++ { // safety limit
		products, _, err := repo.List(ctx, model.ProductListParams{
			Sort:   "created_at_desc",
			Limit:  pageSize,
			Cursor: cursor,
		})
		if err != nil {
			t.Fatalf("page %d: %v", i, err)
		}

		// In cursor mode, repo returns limit+1 items if there are more
		hasMore := len(products) > pageSize
		if hasMore {
			products = products[:pageSize]
		}

		allProducts = append(allProducts, products...)

		if !hasMore {
			break
		}

		last := products[len(products)-1]
		cursor = pagination.EncodeCursor(
			last.CreatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"),
			last.ID,
		)
	}

	if len(allProducts) != 25 {
		t.Errorf("expected 25 products total, got %d", len(allProducts))
	}

	// Verify no duplicates
	seen := make(map[string]bool)
	for _, p := range allProducts {
		key := p.ID.String()
		if seen[key] {
			t.Errorf("duplicate product: %s", key)
		}
		seen[key] = true
	}

	// Verify descending order
	for i := 1; i < len(allProducts); i++ {
		if allProducts[i].CreatedAt.After(allProducts[i-1].CreatedAt) {
			t.Errorf("products not in descending created_at order at index %d", i)
		}
	}
}

func TestPagination_OffsetStillWorks(t *testing.T) {
	infra := setupInfra(t)
	ctx := context.Background()
	testutil.TruncateAll(ctx, t, infra.Pool)
	testutil.SeedProducts(ctx, t, infra.Pool, 15)

	breaker := resilience.NewBreaker(resilience.BreakerConfig{Name: "test-offset"})
	repo := repository.NewProductRepository(infra.Pool, breaker)

	products, total, err := repo.List(ctx, model.ProductListParams{
		Sort:  "created_at_desc",
		Page:  2,
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("list page 2: %v", err)
	}
	if total != 15 {
		t.Errorf("expected total 15, got %d", total)
	}
	if len(products) != 5 {
		t.Errorf("expected 5 products on page 2, got %d", len(products))
	}
}

func TestPagination_PriceSortCursor(t *testing.T) {
	infra := setupInfra(t)
	ctx := context.Background()
	testutil.TruncateAll(ctx, t, infra.Pool)
	testutil.SeedProducts(ctx, t, infra.Pool, 10)

	breaker := resilience.NewBreaker(resilience.BreakerConfig{Name: "test-price-cursor"})
	repo := repository.NewProductRepository(infra.Pool, breaker)

	// First page, price ascending
	products, _, err := repo.List(ctx, model.ProductListParams{
		Sort:  "price_asc",
		Limit: 3,
	})
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	hasMore := len(products) > 3
	if !hasMore {
		t.Fatal("expected more products")
	}
	products = products[:3]

	// Verify ascending price order
	for i := 1; i < len(products); i++ {
		if products[i].Price < products[i-1].Price {
			t.Errorf("prices not ascending at index %d: %d < %d", i, products[i].Price, products[i-1].Price)
		}
	}
}
```

- [ ] **Step 2: Run the tests**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/go/ecommerce-service && go test -tags=integration -race ./internal/integration/ -run TestPagination -v -timeout 120s`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer
git add go/ecommerce-service/internal/integration/pagination_test.go
git commit -m "test(ecommerce): add cursor pagination integration tests"
```

---

## Task 13: Idempotency and Rate Limit Integration Tests

**Files:**
- Create: `go/ecommerce-service/internal/integration/idempotency_test.go`
- Create: `go/ecommerce-service/internal/integration/ratelimit_test.go`

- [ ] **Step 1: Write idempotency integration tests**

Create `go/ecommerce-service/internal/integration/idempotency_test.go`:

```go
//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/integration/testutil"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/middleware"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
	"github.com/gin-gonic/gin"
)

func TestIdempotency_SameKeyReturnsCachedResponse(t *testing.T) {
	infra := setupInfra(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(apperror.ErrorHandler())

	callCount := 0
	router.POST("/test",
		func(c *gin.Context) { c.Set("userId", uuid.New().String()); c.Next() },
		middleware.Idempotency(infra.RedisClient, true),
		func(c *gin.Context) {
			callCount++
			c.JSON(http.StatusCreated, gin.H{"id": "order-" + uuid.New().String()})
		},
	)

	idempotencyKey := uuid.New().String()
	headers := map[string]string{"Idempotency-Key": idempotencyKey}

	// First request
	w1 := testutil.DoRequest(t, router, "POST", "/test", `{}`, headers)
	if w1.Code != http.StatusCreated {
		t.Fatalf("first request: expected 201, got %d: %s", w1.Code, w1.Body.String())
	}

	// Second request with same key
	w2 := testutil.DoRequest(t, router, "POST", "/test", `{}`, headers)
	if w2.Code != http.StatusCreated {
		t.Fatalf("second request: expected 201 (cached), got %d: %s", w2.Code, w2.Body.String())
	}

	// Handler should only have been called once
	if callCount != 1 {
		t.Errorf("expected handler called once, got %d", callCount)
	}

	// Both responses should be identical
	if w1.Body.String() != w2.Body.String() {
		t.Errorf("responses differ:\n  first:  %s\n  second: %s", w1.Body.String(), w2.Body.String())
	}
}

func TestIdempotency_DifferentKeyCreatesSeparateResource(t *testing.T) {
	infra := setupInfra(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(apperror.ErrorHandler())

	callCount := 0
	router.POST("/test",
		func(c *gin.Context) { c.Set("userId", uuid.New().String()); c.Next() },
		middleware.Idempotency(infra.RedisClient, true),
		func(c *gin.Context) {
			callCount++
			c.JSON(http.StatusCreated, gin.H{"call": callCount})
		},
	)

	w1 := testutil.DoRequest(t, router, "POST", "/test", `{}`, map[string]string{"Idempotency-Key": uuid.New().String()})
	w2 := testutil.DoRequest(t, router, "POST", "/test", `{}`, map[string]string{"Idempotency-Key": uuid.New().String()})

	if w1.Code != http.StatusCreated || w2.Code != http.StatusCreated {
		t.Fatalf("expected both 201, got %d and %d", w1.Code, w2.Code)
	}
	if callCount != 2 {
		t.Errorf("expected handler called twice, got %d", callCount)
	}

	var r1, r2 map[string]any
	json.Unmarshal(w1.Body.Bytes(), &r1)
	json.Unmarshal(w2.Body.Bytes(), &r2)
	if r1["call"] == r2["call"] {
		t.Error("expected different responses for different keys")
	}
}

func TestIdempotency_ExpiredKeyAllowsNewRequest(t *testing.T) {
	// This test verifies behavior conceptually — we can't easily wait 24h.
	// Instead we verify that a key set with short TTL expires.
	infra := setupInfra(t)
	ctx := infra.RedisClient.Context()

	key := "idempotency:testuser:" + uuid.New().String()
	infra.RedisClient.Set(ctx, key, `{"status":"done","status_code":201,"body":"{}"}`, 1*time.Second)

	time.Sleep(1100 * time.Millisecond)

	result, err := infra.RedisClient.Get(ctx, key).Result()
	if err == nil {
		t.Errorf("expected key to be expired, got: %s", result)
	}
}
```

- [ ] **Step 2: Write rate limit integration tests**

Create `go/ecommerce-service/internal/integration/ratelimit_test.go`:

```go
//go:build integration

package integration

import (
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/integration/testutil"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/middleware"
)

func TestRateLimiter_BlocksAfterThreshold(t *testing.T) {
	infra := setupInfra(t)

	// Use a small limit for testing
	limiter := middleware.NewRateLimiter(infra.RedisClient, "test:ratelimit:"+t.Name(), 5, time.Minute)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(limiter.Middleware())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// 5 requests should succeed
	for i := 0; i < 5; i++ {
		w := testutil.DoRequest(t, router, "GET", "/test", "", nil)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i+1, w.Code)
		}
	}

	// 6th request should be rate limited
	w := testutil.DoRequest(t, router, "GET", "/test", "", nil)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", w.Code)
	}

	retryAfter := w.Header().Get("Retry-After")
	if retryAfter == "" {
		t.Error("expected Retry-After header")
	}
}
```

- [ ] **Step 3: Run all integration tests**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/go/ecommerce-service && go test -tags=integration -race ./internal/integration/ -v -timeout 180s`
Expected: All PASS

- [ ] **Step 4: Commit**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer
git add go/ecommerce-service/internal/integration/idempotency_test.go go/ecommerce-service/internal/integration/ratelimit_test.go
git commit -m "test(ecommerce): add idempotency and rate limiter integration tests"
```

---

## Task 14: CI and Makefile Integration

**Files:**
- Modify: `Makefile` — add `preflight-go-integration` target

- [ ] **Step 1: Add Makefile target**

Add to `Makefile`:

```makefile
preflight-go-integration:
	cd go/ecommerce-service && go test -tags=integration -race -timeout 180s ./internal/integration/...
```

- [ ] **Step 2: Run the full preflight-go to verify no regressions**

Run: `make preflight-go`
Expected: All existing lint + unit tests PASS

- [ ] **Step 3: Run the new integration target**

Run: `make preflight-go-integration`
Expected: All integration tests PASS

- [ ] **Step 4: Commit**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer
git add Makefile
git commit -m "chore: add preflight-go-integration Makefile target"
```

---

## Task 15: Final Verification

- [ ] **Step 1: Run full preflight**

Run: `make preflight-go`
Expected: All lint + unit tests PASS

- [ ] **Step 2: Run integration tests**

Run: `make preflight-go-integration`
Expected: All integration tests PASS

- [ ] **Step 3: Verify no untracked files or uncommitted changes**

Run: `git status`
Expected: Clean working tree (all changes committed)
