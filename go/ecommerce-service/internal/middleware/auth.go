package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

func Auth(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			_ = c.Error(apperror.Unauthorized("MISSING_AUTH", "missing authorization header"))
			c.Abort()
			return
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		claims := jwt.MapClaims{}
		_, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, apperror.Forbidden("INVALID_TOKEN", "unexpected signing method")
			}
			return []byte(jwtSecret), nil
		})
		if err != nil {
			_ = c.Error(apperror.Forbidden("INVALID_TOKEN", "invalid or expired token"))
			c.Abort()
			return
		}
		sub, ok := claims["sub"].(string)
		if !ok {
			_ = c.Error(apperror.Forbidden("INVALID_TOKEN", "invalid token claims"))
			c.Abort()
			return
		}
		c.Set("userId", sub)
		c.Next()
	}
}
