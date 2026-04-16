package model_test

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/kabradshaw1/portfolio/go/auth-service/internal/model"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestRegisterRequest_PasswordMinLength(t *testing.T) {
	t.Run("8-character password fails validation", func(t *testing.T) {
		req := model.RegisterRequest{
			Email:    "test@example.com",
			Password: "abcd1234", // exactly 8 chars — below new min of 12
			Name:     "Test User",
		}
		err := binding.Validator.ValidateStruct(req)
		if err == nil {
			t.Error("expected validation error for 8-character password, got nil")
		}
	})

	t.Run("14-character password passes validation", func(t *testing.T) {
		req := model.RegisterRequest{
			Email:    "test@example.com",
			Password: "abcd1234567890", // 14 chars — above min of 12
			Name:     "Test User",
		}
		err := binding.Validator.ValidateStruct(req)
		if err != nil {
			t.Errorf("expected no validation error for 14-character password, got: %v", err)
		}
	})
}
