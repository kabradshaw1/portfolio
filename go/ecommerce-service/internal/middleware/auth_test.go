package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/middleware"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
	"github.com/stretchr/testify/assert"
)

const testSecret = "test-secret-key-at-least-32-chars-long"

func setupRouter(secret string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(apperror.ErrorHandler())
	r.Use(middleware.Auth(secret))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"userId": c.GetString("userId")})
	})
	return r
}

func TestAuth_RejectsNoneAlgorithm(t *testing.T) {
	r := setupRouter(testSecret)
	token := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{"sub": "user-123"})
	tokenStr, _ := token.SignedString(jwt.UnsafeAllowNoneSignatureType)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestAuth_AcceptsValidHS256Token(t *testing.T) {
	r := setupRouter(testSecret)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": "user-123"})
	tokenStr, _ := token.SignedString([]byte(testSecret))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
