package apperror

import (
	"net/http"
	"testing"
)

func TestValidation_HTTPStatus(t *testing.T) {
	ae := Validation([]FieldError{{Field: "quantity", Message: "must be between 1 and 99"}})
	if ae.HTTPStatus != http.StatusUnprocessableEntity {
		t.Errorf("HTTPStatus = %d, want 422", ae.HTTPStatus)
	}
}

func TestValidation_Code(t *testing.T) {
	ae := Validation([]FieldError{{Field: "quantity", Message: "must be between 1 and 99"}})
	if ae.Code != "VALIDATION_ERROR" {
		t.Errorf("Code = %q, want VALIDATION_ERROR", ae.Code)
	}
}

func TestValidation_Fields(t *testing.T) {
	fields := []FieldError{
		{Field: "quantity", Message: "must be between 1 and 99"},
		{Field: "price", Message: "must be positive"},
	}
	ae := Validation(fields)
	if len(ae.Fields) != len(fields) {
		t.Fatalf("len(Fields) = %d, want %d", len(ae.Fields), len(fields))
	}
	for i, f := range fields {
		if ae.Fields[i].Field != f.Field {
			t.Errorf("Fields[%d].Field = %q, want %q", i, ae.Fields[i].Field, f.Field)
		}
		if ae.Fields[i].Message != f.Message {
			t.Errorf("Fields[%d].Message = %q, want %q", i, ae.Fields[i].Message, f.Message)
		}
	}
}

func TestValidation_ErrorMessage(t *testing.T) {
	ae := Validation([]FieldError{{Field: "name", Message: "required"}})
	if ae.Error() != "validation failed" {
		t.Errorf("Error() = %q, want \"validation failed\"", ae.Error())
	}
}
