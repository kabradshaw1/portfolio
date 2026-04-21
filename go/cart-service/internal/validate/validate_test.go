package validate_test

import (
	"testing"

	"github.com/kabradshaw1/portfolio/go/cart-service/internal/validate"
)

func TestAddToCart_Valid(t *testing.T) {
	errs := validate.AddToCart("550e8400-e29b-41d4-a716-446655440000", 1)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestAddToCart_EmptyProductID(t *testing.T) {
	errs := validate.AddToCart("", 1)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if errs[0].Field != "productId" {
		t.Errorf("expected field productId, got %s", errs[0].Field)
	}
}

func TestAddToCart_InvalidUUID(t *testing.T) {
	errs := validate.AddToCart("not-a-uuid", 1)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
}

func TestAddToCart_QuantityTooLow(t *testing.T) {
	errs := validate.AddToCart("550e8400-e29b-41d4-a716-446655440000", 0)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if errs[0].Field != "quantity" {
		t.Errorf("expected field quantity, got %s", errs[0].Field)
	}
}

func TestAddToCart_QuantityTooHigh(t *testing.T) {
	errs := validate.AddToCart("550e8400-e29b-41d4-a716-446655440000", 100)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
}

func TestAddToCart_MultipleErrors(t *testing.T) {
	errs := validate.AddToCart("", 0)
	if len(errs) != 2 {
		t.Errorf("expected 2 errors, got %d", len(errs))
	}
}

func TestUpdateCart_Valid(t *testing.T) {
	errs := validate.UpdateCart(5)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestUpdateCart_QuantityTooLow(t *testing.T) {
	errs := validate.UpdateCart(0)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
}

func TestUpdateCart_QuantityTooHigh(t *testing.T) {
	errs := validate.UpdateCart(100)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
}
